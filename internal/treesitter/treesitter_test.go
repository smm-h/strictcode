package treesitter

import (
	"bytes"
	"testing"
)

func TestNormalizeLF(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no-cr-unchanged", "a\nb\n", "a\nb\n"},
		{"crlf", "a\r\nb\r\n", "a\nb\n"},
		{"lone-cr", "a\rb\r", "a\nb\n"},
		{"mixed", "a\r\nb\rc\n", "a\nb\nc\n"},
		{"cr-at-end-of-crlf-run", "x\r\r\ny", "x\n\ny"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeLF([]byte(c.in))
			if string(got) != c.want {
				t.Fatalf("NormalizeLF(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeLFReturnsInputWhenClean(t *testing.T) {
	in := []byte("clean\nsource\n")
	out := NormalizeLF(in)
	if &in[0] != &out[0] {
		t.Fatal("NormalizeLF copied a clean input; it must return it as-is")
	}
}

func TestGrammarForFile(t *testing.T) {
	cases := []struct {
		file string
		want Grammar
		ok   bool
	}{
		{"pkg/mod.py", GrammarPython, true},
		{"main.go", GrammarGo, true},
		{"src/index.ts", GrammarTypeScript, true},
		{"src/util.js", GrammarTypeScript, true},
		{"src/a.mjs", GrammarTypeScript, true},
		{"src/a.cjs", GrammarTypeScript, true},
		{"src/a.mts", GrammarTypeScript, true},
		{"src/a.cts", GrammarTypeScript, true},
		{"src/App.tsx", GrammarTSX, true},
		{"src/App.jsx", GrammarTSX, true},
		{"README.md", 0, false},
		{"Makefile", 0, false},
		{"noext", 0, false},
	}
	for _, c := range cases {
		g, ok := GrammarForFile(c.file)
		if ok != c.ok || (ok && g != c.want) {
			t.Errorf("GrammarForFile(%q) = (%v, %v), want (%v, %v)", c.file, g, ok, c.want, c.ok)
		}
	}
}

func TestParseAllGrammars(t *testing.T) {
	cases := []struct {
		grammar  Grammar
		src      string
		rootKind string
	}{
		{GrammarPython, "import os\n\ndef f():\n    return 1\n", "module"},
		{GrammarGo, "package main\n\nfunc main() {}\n", "source_file"},
		{GrammarTypeScript, "import { x } from './x';\nexport function f(): number { return x; }\n", "program"},
		{GrammarTSX, "export const C = () => <div/>;\n", "program"},
	}
	for _, c := range cases {
		t.Run(c.grammar.String(), func(t *testing.T) {
			tree, err := Parse(c.grammar, []byte(c.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			defer tree.Close()
			if kind := tree.Root().Kind(); kind != c.rootKind {
				t.Fatalf("root kind = %q, want %q", kind, c.rootKind)
			}
			if tree.HasParseErrors() {
				t.Fatal("unexpected parse errors in valid source")
			}
		})
	}
}

func TestParseNormalizesBeforeParsing(t *testing.T) {
	// CRLF source: spans must index the normalized bytes, not the raw file.
	raw := []byte("import os\r\nimport sys\r\n")
	tree, err := Parse(GrammarPython, raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()
	if bytes.ContainsRune(tree.Source, '\r') {
		t.Fatal("tree.Source still contains CR")
	}
	root := tree.Root()
	if got, want := root.ChildCount(), uint(2); got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	second := root.Child(1)
	// "import sys" starts at byte 10 of the LF-normalized source.
	if second.StartByte() != 10 {
		t.Fatalf("second import starts at %d in normalized source, want 10", second.StartByte())
	}
	if text := string(tree.Source[second.StartByte():second.EndByte()]); text != "import sys" {
		t.Fatalf("span text = %q, want %q", text, "import sys")
	}
}

func TestParseErrorsAreReported(t *testing.T) {
	tree, err := Parse(GrammarPython, []byte("def broken(:\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()
	if !tree.HasParseErrors() {
		t.Fatal("HasParseErrors = false for syntactically broken source")
	}
}

func TestQueryMatches(t *testing.T) {
	src := []byte("import os\nfrom collections import abc\n\ndef alpha():\n    pass\n\ndef beta():\n    pass\n")
	tree, err := Parse(GrammarPython, src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()

	q, err := CompileQuery(GrammarPython, `(function_definition name: (identifier) @name)`)
	if err != nil {
		t.Fatalf("CompileQuery: %v", err)
	}
	defer q.Close()

	matches := q.Matches(tree)
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(matches))
	}
	var names []string
	for _, m := range matches {
		for _, c := range m.Captures {
			if c.Name != "name" {
				t.Fatalf("capture name = %q, want %q", c.Name, "name")
			}
			names = append(names, string(src[c.Node.StartByte():c.Node.EndByte()]))
		}
	}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("captured names = %v, want [alpha beta]", names)
	}
}

func TestQueryTextPredicatesApplied(t *testing.T) {
	src := []byte("__all__ = ['a']\nother = 2\n")
	tree, err := Parse(GrammarPython, src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()

	q, err := CompileQuery(GrammarPython, `(assignment left: (identifier) @lhs (#eq? @lhs "__all__"))`)
	if err != nil {
		t.Fatalf("CompileQuery: %v", err)
	}
	defer q.Close()

	matches := q.Matches(tree)
	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1 (predicate must filter)", len(matches))
	}
	c := matches[0].Captures[0]
	if text := string(src[c.Node.StartByte():c.Node.EndByte()]); text != "__all__" {
		t.Fatalf("capture text = %q, want __all__", text)
	}
}

func TestQueryCompileErrorIsHardError(t *testing.T) {
	_, err := CompileQuery(GrammarPython, `(nonexistent_node_kind) @x`)
	if err == nil {
		t.Fatal("CompileQuery accepted a query over an unknown node kind")
	}
}

func TestQueryGrammarMismatchPanics(t *testing.T) {
	tree, err := Parse(GrammarGo, []byte("package p\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Close()
	q, err := CompileQuery(GrammarPython, `(module) @m`)
	if err != nil {
		t.Fatalf("CompileQuery: %v", err)
	}
	defer q.Close()
	defer func() {
		if recover() == nil {
			t.Fatal("no panic on query/tree grammar mismatch")
		}
	}()
	q.Matches(tree)
}

func TestCloseIdempotent(t *testing.T) {
	tree, err := Parse(GrammarPython, []byte("x = 1\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tree.Close()
	tree.Close() // must not panic or double-free

	q, err := CompileQuery(GrammarPython, `(module) @m`)
	if err != nil {
		t.Fatalf("CompileQuery: %v", err)
	}
	q.Close()
	q.Close()
}
