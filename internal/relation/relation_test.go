package relation

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/vocab"
)

// --- test fixtures --------------------------------------------------------

func moduleID(module string) NodeID {
	return NodeID{Lang: "py", Member: "core", Module: module}
}

func moduleNode(module string, testContext bool) Node {
	return Node{
		Kind: vocab.NodeKindModule,
		ID:   moduleID(module),
		Attrs: map[string]Value{
			"logical_name": StringValue(module),
			"path":         StringValue("src/" + module + ".py"),
			"test_context": BoolValue(testContext),
		},
	}
}

func functionNode(module, name string) Node {
	return Node{
		Kind: vocab.NodeKindFunction,
		ID: NodeID{Lang: "py", Member: "core", Module: module,
			Chain: []Segment{{Name: name}}},
		Attrs: map[string]Value{
			"visibility": StringValue("public"),
			"is_method":  BoolValue(false),
			"is_async":   BoolValue(false),
			"is_test":    BoolValue(false),
		},
	}
}

func importsRow(src, dst string, span Span) Row {
	return Row{
		Kind: vocab.RowKindImports,
		Src:  moduleID(src),
		Dst:  moduleID(dst),
		File: "src/" + src + ".py",
		Span: span,
		Attrs: map[string]Value{
			"test_context":  BoolValue(false),
			"guarded":       BoolValue(false),
			"type_checking": BoolValue(false),
		},
	}
}

// --- builder validation ---------------------------------------------------

func TestBuildSmallRelation(t *testing.T) {
	b := NewBuilder()
	if err := b.AddNode(moduleNode("a", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddNode(moduleNode("b", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddRow(importsRow("a", "b", Span{0, 8})); err != nil {
		t.Fatal(err)
	}
	rel, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(rel.Nodes) != 2 || len(rel.Rows) != 1 {
		t.Fatalf("built %d nodes, %d rows; want 2, 1", len(rel.Nodes), len(rel.Rows))
	}
}

func TestRowsBeforeNodesIsFine(t *testing.T) {
	b := NewBuilder()
	if err := b.AddRow(importsRow("a", "b", Span{0, 8})); err != nil {
		t.Fatal(err)
	}
	if err := b.AddNode(moduleNode("a", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddNode(moduleNode("b", false)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Build(); err != nil {
		t.Fatal(err)
	}
}

func TestNodeIDCollisionIsHardError(t *testing.T) {
	b := NewBuilder()
	if err := b.AddNode(moduleNode("a", false)); err != nil {
		t.Fatal(err)
	}
	err := b.AddNode(moduleNode("a", true)) // same ID, different attrs
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("want collision error, got %v", err)
	}
}

func TestCaseOnlyClashIsHardError(t *testing.T) {
	b := NewBuilder()
	if err := b.AddNode(moduleNode("Utils", false)); err != nil {
		t.Fatal(err)
	}
	err := b.AddNode(moduleNode("utils", false))
	if err == nil || !strings.Contains(err.Error(), "case-only") {
		t.Fatalf("want case-only clash error, got %v", err)
	}
}

func TestUnknownNodeKindIsHardError(t *testing.T) {
	b := NewBuilder()
	n := moduleNode("a", false)
	n.Kind = "gadget"
	if err := b.AddNode(n); err == nil || !strings.Contains(err.Error(), "unknown node kind") {
		t.Fatalf("want unknown-kind error, got %v", err)
	}
}

func TestAttributeDiscipline(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Node)
		wantSub string
	}{
		{"missing", func(n *Node) { delete(n.Attrs, "path") }, "missing attribute"},
		{"undeclared", func(n *Node) { n.Attrs["extra"] = StringValue("x") }, "undeclared attribute"},
		{"wrong-type", func(n *Node) { n.Attrs["test_context"] = StringValue("false") }, "must be a boolean"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := moduleNode("a", false)
			c.mutate(&n)
			err := NewBuilder().AddNode(n)
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("want %q error, got %v", c.wantSub, err)
			}
		})
	}
}

func TestIllegalEnumValueIsHardError(t *testing.T) {
	n := functionNode("m", "f")
	n.Attrs["visibility"] = StringValue("secret")
	err := NewBuilder().AddNode(n)
	if err == nil || !strings.Contains(err.Error(), "illegal enum value") {
		t.Fatalf("want illegal-enum error, got %v", err)
	}
}

func TestRowValidation(t *testing.T) {
	valid := func() Row { return importsRow("a", "b", Span{0, 8}) }

	t.Run("unknown-kind", func(t *testing.T) {
		r := valid()
		r.Kind = "links"
		if err := NewBuilder().AddRow(r); err == nil || !strings.Contains(err.Error(), "unknown row kind") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("empty-file", func(t *testing.T) {
		r := valid()
		r.File = ""
		if err := NewBuilder().AddRow(r); err == nil || !strings.Contains(err.Error(), "empty file") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("inverted-span", func(t *testing.T) {
		r := valid()
		r.Span = Span{9, 3}
		if err := NewBuilder().AddRow(r); err == nil || !strings.Contains(err.Error(), "inverted span") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing-attr", func(t *testing.T) {
		r := valid()
		delete(r.Attrs, "guarded")
		if err := NewBuilder().AddRow(r); err == nil || !strings.Contains(err.Error(), "missing attribute") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCallsResolutionIsMandatory(t *testing.T) {
	// SPEC.md section 4.2: calls.resolution is mandatory, no default.
	b := NewBuilder()
	r := Row{
		Kind:  vocab.RowKindCalls,
		Src:   functionNode("m", "f").ID,
		Dst:   functionNode("m", "g").ID,
		File:  "src/m.py",
		Span:  Span{5, 10},
		Attrs: map[string]Value{},
	}
	err := b.AddRow(r)
	if err == nil || !strings.Contains(err.Error(), `missing attribute "resolution"`) {
		t.Fatalf("got %v", err)
	}
}

func TestBuildRejectsDanglingRows(t *testing.T) {
	b := NewBuilder()
	if err := b.AddNode(moduleNode("a", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddRow(importsRow("a", "ghost", Span{0, 5})); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Build(); err == nil || !strings.Contains(err.Error(), "absent dst node") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildRejectsEndpointKindViolation(t *testing.T) {
	// imports rows must have module src; a function src is a hard error.
	b := NewBuilder()
	fn := functionNode("m", "f")
	if err := b.AddNode(fn); err != nil {
		t.Fatal(err)
	}
	if err := b.AddNode(moduleNode("b", false)); err != nil {
		t.Fatal(err)
	}
	r := importsRow("a", "b", Span{0, 5})
	r.Src = fn.ID
	if err := b.AddRow(r); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Build(); err == nil || !strings.Contains(err.Error(), "allowed src kinds") {
		t.Fatalf("got %v", err)
	}
}

// --- determinism ----------------------------------------------------------

func TestCanonicalFormIsInsertionOrderIndependent(t *testing.T) {
	build := func(order []int) *Relation {
		nodes := []Node{moduleNode("a", false), moduleNode("b", false), moduleNode("c", true)}
		rows := []Row{
			importsRow("a", "b", Span{0, 8}),
			importsRow("b", "c", Span{4, 12}),
			importsRow("a", "c", Span{20, 28}),
		}
		b := NewBuilder()
		for _, i := range order {
			if err := b.AddNode(nodes[i]); err != nil {
				t.Fatal(err)
			}
		}
		for _, i := range order {
			if err := b.AddRow(rows[i]); err != nil {
				t.Fatal(err)
			}
		}
		rel, err := b.Build()
		if err != nil {
			t.Fatal(err)
		}
		return rel
	}
	r1 := build([]int{0, 1, 2})
	r2 := build([]int{2, 0, 1})
	if string(r1.CanonicalForm()) != string(r2.CanonicalForm()) {
		t.Fatal("canonical forms differ across insertion orders")
	}
	if r1.Hash() != r2.Hash() {
		t.Fatal("hashes differ across insertion orders")
	}
}

func TestHashChangesWithContent(t *testing.T) {
	base := func(guarded bool) [32]byte {
		b := NewBuilder()
		if err := b.AddNode(moduleNode("a", false)); err != nil {
			t.Fatal(err)
		}
		if err := b.AddNode(moduleNode("b", false)); err != nil {
			t.Fatal(err)
		}
		r := importsRow("a", "b", Span{0, 8})
		r.Attrs["guarded"] = BoolValue(guarded)
		if err := b.AddRow(r); err != nil {
			t.Fatal(err)
		}
		rel, err := b.Build()
		if err != nil {
			t.Fatal(err)
		}
		return rel.Hash()
	}
	if base(false) == base(true) {
		t.Fatal("hash did not change when a row attribute changed")
	}
}

// --- projections ----------------------------------------------------------

func TestAlgorithmGraphProjectsDistinctPairs(t *testing.T) {
	b := NewBuilder()
	for _, m := range []string{"a", "b"} {
		if err := b.AddNode(moduleNode(m, false)); err != nil {
			t.Fatal(err)
		}
	}
	// Two imports rows for the same (a, b) pair at different sites.
	if err := b.AddRow(importsRow("a", "b", Span{0, 8})); err != nil {
		t.Fatal(err)
	}
	if err := b.AddRow(importsRow("a", "b", Span{30, 38})); err != nil {
		t.Fatal(err)
	}
	rel, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	edges := rel.AlgorithmGraph(vocab.RowKindImports)
	if len(edges) != 1 {
		t.Fatalf("algorithm graph has %d edges, want 1 (distinct pairs)", len(edges))
	}
	feed := rel.SiteFeed(vocab.RowKindImports)
	if len(feed) != 2 {
		t.Fatalf("site feed has %d rows, want 2 (every site)", len(feed))
	}
	if feed[0].Span.Start > feed[1].Span.Start {
		t.Fatal("site feed not in canonical order")
	}
}

func TestProjectionsFilterByKind(t *testing.T) {
	b := NewBuilder()
	if err := b.AddNode(moduleNode("a", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddNode(moduleNode("b", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddRow(importsRow("a", "b", Span{0, 8})); err != nil {
		t.Fatal(err)
	}
	rel, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if got := rel.AlgorithmGraph(vocab.RowKindCalls); len(got) != 0 {
		t.Fatalf("calls projection has %d edges, want 0", len(got))
	}
	if got := rel.SiteFeed(vocab.RowKindCalls); len(got) != 0 {
		t.Fatalf("calls site feed has %d rows, want 0", len(got))
	}
}

// --- canonical form shape -------------------------------------------------

func TestCanonicalFormShape(t *testing.T) {
	b := NewBuilder()
	if err := b.AddNode(moduleNode("a", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddNode(moduleNode("b", false)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddRow(importsRow("a", "b", Span{0, 8})); err != nil {
		t.Fatal(err)
	}
	rel, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(rel.CanonicalForm()), "\n"), "\n")
	if lines[0] != "strictcode-relation-canonical v1" {
		t.Fatalf("first line = %q, want the version line", lines[0])
	}
	if len(lines) != 4 {
		t.Fatalf("canonical form has %d lines, want 4 (version + 2 nodes + 1 row)", len(lines))
	}
	if !strings.HasPrefix(lines[1], "node module ") || !strings.HasPrefix(lines[3], "row imports ") {
		t.Fatalf("unexpected canonical layout:\n%s", strings.Join(lines, "\n"))
	}
}
