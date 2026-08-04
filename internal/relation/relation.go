// Package relation implements the interaction relation — the primary
// artifact of extraction (schema/SPEC.md). A flat typed relation of rows
// plus a companion node table; the algorithm graph and the site feed are
// pure deterministic projections; canonical form (sorted rows, SHA-256) is
// defined once, here.
//
// The builder is the schema-discipline chokepoint: unknown kinds, undeclared
// or missing attributes, illegal enum values, ID collisions, case-only ID
// clashes, and rows referencing absent nodes are all hard errors. Nothing is
// coerced, defaulted, or skipped.
package relation

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/vocab"
)

// Span is a byte range over the file's LF-normalized UTF-8 bytes
// (schema/SPEC.md section 3). Line/column are derived at output time, never
// stored as truth.
type Span struct {
	Start uint32
	End   uint32
}

// Node is one node-table entry: (node_kind, id, attrs).
type Node struct {
	Kind  vocab.NodeKind
	ID    NodeID
	Attrs map[string]Value
}

// Row is one interaction-relation row:
// (row_kind, src_node, dst_node, file, span, attrs).
type Row struct {
	Kind  vocab.RowKind
	Src   NodeID
	Dst   NodeID
	File  string
	Span  Span
	Attrs map[string]Value
}

// Builder accumulates nodes and rows, validating each addition against the
// vocabulary. Insertion order is irrelevant: Build sorts everything into
// canonical order, and rows may be added before the nodes they reference
// (referential integrity is checked at Build).
type Builder struct {
	nodes     map[string]Node   // serialized ID -> node
	caseIndex map[string]string // lowercased serialized ID -> serialized ID
	rows      []Row
}

// NewBuilder returns an empty builder.
func NewBuilder() *Builder {
	return &Builder{
		nodes:     map[string]Node{},
		caseIndex: map[string]string{},
	}
}

// AddNode validates and inserts a node. Hard errors: unknown kind, invalid
// ID, attribute violations, ID collision, case-only ID clash.
func (b *Builder) AddNode(n Node) error {
	decl, ok := vocab.NodeKindInfo[n.Kind]
	if !ok {
		return fmt.Errorf("relation: unknown node kind %q", n.Kind)
	}
	if err := n.ID.Validate(); err != nil {
		return err
	}
	if err := checkAttrs(fmt.Sprintf("node %s", n.ID), decl.Attrs, n.Attrs); err != nil {
		return err
	}
	id := n.ID.String()
	if _, exists := b.nodes[id]; exists {
		return fmt.Errorf("relation: node ID collision: two distinct nodes serialize to %q", id)
	}
	lower := strings.ToLower(id)
	if prior, exists := b.caseIndex[lower]; exists {
		return fmt.Errorf("relation: case-only ID clash between %q and %q (case-insensitive filesystem safety)", prior, id)
	}
	b.nodes[id] = n
	b.caseIndex[lower] = id
	return nil
}

// AddRow validates and appends a row. Hard errors: unknown kind, invalid
// IDs, empty file, inverted span, attribute violations. Referential
// integrity and src/dst kind constraints are checked at Build.
func (b *Builder) AddRow(r Row) error {
	decl, ok := vocab.RowKindInfo[r.Kind]
	if !ok {
		return fmt.Errorf("relation: unknown row kind %q", r.Kind)
	}
	if err := r.Src.Validate(); err != nil {
		return fmt.Errorf("relation: row %s src: %w", r.Kind, err)
	}
	if err := r.Dst.Validate(); err != nil {
		return fmt.Errorf("relation: row %s dst: %w", r.Kind, err)
	}
	if r.File == "" {
		return fmt.Errorf("relation: row %s has empty file", r.Kind)
	}
	if r.Span.Start > r.Span.End {
		return fmt.Errorf("relation: row %s has inverted span %d..%d", r.Kind, r.Span.Start, r.Span.End)
	}
	if err := checkAttrs(fmt.Sprintf("row %s %s->%s", r.Kind, r.Src, r.Dst), decl.Attrs, r.Attrs); err != nil {
		return err
	}
	b.rows = append(b.rows, r)
	return nil
}

// checkAttrs enforces the vocabulary attribute declaration exactly: every
// declared attribute present with the declared type (enum values legal),
// and no undeclared attributes.
func checkAttrs(subject string, decls []vocab.AttrDecl, attrs map[string]Value) error {
	declared := map[string]vocab.AttrDecl{}
	for _, d := range decls {
		declared[d.Name] = d
	}
	for name := range attrs {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("relation: %s carries undeclared attribute %q", subject, name)
		}
	}
	for _, d := range decls {
		v, ok := attrs[d.Name]
		if !ok {
			return fmt.Errorf("relation: %s missing attribute %q (all declared attributes are mandatory)", subject, d.Name)
		}
		switch d.Type {
		case vocab.AttrString:
			if v.Kind() != ValueString {
				return fmt.Errorf("relation: %s attribute %q must be a string", subject, d.Name)
			}
		case vocab.AttrBool:
			if v.Kind() != ValueBool {
				return fmt.Errorf("relation: %s attribute %q must be a boolean", subject, d.Name)
			}
		case vocab.AttrInt:
			if v.Kind() != ValueInt {
				return fmt.Errorf("relation: %s attribute %q must be an integer", subject, d.Name)
			}
		case vocab.AttrEnum:
			s, isStr := v.AsString()
			if !isStr {
				return fmt.Errorf("relation: %s attribute %q must be an enum value (string)", subject, d.Name)
			}
			legal := false
			for _, val := range vocab.Enums[d.Enum] {
				if s == val {
					legal = true
					break
				}
			}
			if !legal {
				return fmt.Errorf("relation: %s attribute %q has illegal enum value %q (enum %s allows %v)",
					subject, d.Name, s, d.Enum, vocab.Enums[d.Enum])
			}
		}
	}
	return nil
}

// Relation is the frozen, canonically ordered relation: the node table
// sorted by (node_kind, id) and the rows sorted by the total key
// (row_kind, src_id, dst_id, file, span_start, span_end, attr-tuple).
type Relation struct {
	Nodes []Node
	Rows  []Row
}

// Build freezes the builder into a Relation. Hard errors: a row whose src or
// dst is not in the node table, or whose src/dst node kind violates the row
// kind's declared src_kinds/dst_kinds.
func (b *Builder) Build() (*Relation, error) {
	// Referential integrity + endpoint kind constraints.
	for _, r := range b.rows {
		decl := vocab.RowKindInfo[r.Kind]
		src, ok := b.nodes[r.Src.String()]
		if !ok {
			return nil, fmt.Errorf("relation: row %s references absent src node %s", r.Kind, r.Src)
		}
		dst, ok := b.nodes[r.Dst.String()]
		if !ok {
			return nil, fmt.Errorf("relation: row %s references absent dst node %s", r.Kind, r.Dst)
		}
		if !kindAllowed(src.Kind, decl.SrcKinds) {
			return nil, fmt.Errorf("relation: row %s src %s has kind %s; allowed src kinds are %v",
				r.Kind, r.Src, src.Kind, decl.SrcKinds)
		}
		if !kindAllowed(dst.Kind, decl.DstKinds) {
			return nil, fmt.Errorf("relation: row %s dst %s has kind %s; allowed dst kinds are %v",
				r.Kind, r.Dst, dst.Kind, decl.DstKinds)
		}
	}

	rel := &Relation{
		Nodes: make([]Node, 0, len(b.nodes)),
		Rows:  make([]Row, len(b.rows)),
	}
	for _, n := range b.nodes {
		rel.Nodes = append(rel.Nodes, n)
	}
	sort.Slice(rel.Nodes, func(i, j int) bool {
		if rel.Nodes[i].Kind != rel.Nodes[j].Kind {
			return rel.Nodes[i].Kind < rel.Nodes[j].Kind
		}
		return rel.Nodes[i].ID.String() < rel.Nodes[j].ID.String()
	})
	copy(rel.Rows, b.rows)
	sort.Slice(rel.Rows, func(i, j int) bool {
		return rowKey(rel.Rows[i]) < rowKey(rel.Rows[j])
	})
	// The relation is an ordered SET of rows (SPEC.md section 1): identical
	// rows collapse to one element. Extraction can legitimately produce
	// duplicates (e.g. `import os, os` yields two identical rows).
	deduped := rel.Rows[:0]
	var prevKey string
	for i, r := range rel.Rows {
		key := rowKey(r)
		if i == 0 || key != prevKey {
			deduped = append(deduped, r)
		}
		prevKey = key
	}
	rel.Rows = deduped
	return rel, nil
}

func kindAllowed(k vocab.NodeKind, allowed []vocab.NodeKind) bool {
	for _, a := range allowed {
		if k == a {
			return true
		}
	}
	return false
}

// rowKey renders the total sort key of a row. Fields are joined with the
// canonical-format separator (space), each field escaped, so the key order
// equals the canonical-line order.
func rowKey(r Row) string {
	return fmt.Sprintf("%s %s %s %s %010d %010d %s",
		r.Kind,
		escapeCanonical(r.Src.String()),
		escapeCanonical(r.Dst.String()),
		escapeCanonical(r.File),
		r.Span.Start, r.Span.End,
		attrTuple(r.Kind, r.Attrs))
}

// attrTuple renders a row or node attribute set in vocabulary declaration
// order (deterministic; validated complete at Add time).
func attrTuple(kind vocab.RowKind, attrs map[string]Value) string {
	decls := vocab.RowKindInfo[kind].Attrs
	parts := make([]string, 0, len(decls))
	for _, d := range decls {
		parts = append(parts, d.Name+"="+attrs[d.Name].canonical())
	}
	return strings.Join(parts, " ")
}

func nodeAttrTuple(kind vocab.NodeKind, attrs map[string]Value) string {
	decls := vocab.NodeKindInfo[kind].Attrs
	parts := make([]string, 0, len(decls))
	for _, d := range decls {
		parts = append(parts, d.Name+"="+attrs[d.Name].canonical())
	}
	return strings.Join(parts, " ")
}

// canonicalVersion is the canonical-form format version line. Any change to
// the serialization below is a format change and must bump this.
const canonicalVersion = "strictcode-relation-canonical v1"

// CanonicalForm renders the relation deterministically: the version line,
// the sorted node table, then the sorted rows, one record per line, every
// free-form field percent-escaped. Equal relations produce equal bytes.
func (rel *Relation) CanonicalForm() []byte {
	var b strings.Builder
	b.WriteString(canonicalVersion)
	b.WriteByte('\n')
	for _, n := range rel.Nodes {
		b.WriteString("node ")
		b.WriteString(string(n.Kind))
		b.WriteByte(' ')
		b.WriteString(escapeCanonical(n.ID.String()))
		if tuple := nodeAttrTuple(n.Kind, n.Attrs); tuple != "" {
			b.WriteByte(' ')
			b.WriteString(tuple)
		}
		b.WriteByte('\n')
	}
	for _, r := range rel.Rows {
		b.WriteString("row ")
		b.WriteString(rowKey(r))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// Hash is the SHA-256 of the canonical form — the one hash every projection
// inherits determinism from.
func (rel *Relation) Hash() [32]byte {
	return sha256.Sum256(rel.CanonicalForm())
}

// Edge is one algorithm-graph edge: a distinct (src, dst) pair, as
// serialized IDs.
type Edge struct {
	Src string
	Dst string
}

// AlgorithmGraph projects the distinct (src, dst) pairs of one row kind, in
// canonical (sorted) order. Algorithms (Tarjan SCC, reachability) consume
// this; they never see spans or attributes.
func (rel *Relation) AlgorithmGraph(kind vocab.RowKind) []Edge {
	seen := map[Edge]bool{}
	var out []Edge
	for _, r := range rel.Rows {
		if r.Kind != kind {
			continue
		}
		e := Edge{Src: r.Src.String(), Dst: r.Dst.String()}
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	// Rows are already sorted by (kind, src, dst, ...), so first-seen order
	// is sorted (src, dst) order.
	return out
}

// SiteFeed projects the rows of one row kind with their spans and
// attributes, in canonical order — the input for findings and fixes.
func (rel *Relation) SiteFeed(kind vocab.RowKind) []Row {
	var out []Row
	for _, r := range rel.Rows {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}
