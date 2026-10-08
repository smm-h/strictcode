// Package treesitter is strictcode's single parsing path: a thin, disciplined
// layer over cgofree's pure-Go tree-sitter (the official C runtime and
// grammars translated to Go through WebAssembly; builds need no C compiler
// and work with CGO_ENABLED=0). It owns the three responsibilities the rest
// of the codebase must never re-implement:
//
//   - grammar selection for the language trio (Python, Go, TS/JS);
//   - LF normalization before parsing, so every byte span in the system is a
//     span over LF-normalized UTF-8 (stricttools/docs/graph-model.md);
//   - the Parsers: one per grammar for the whole process, because each
//     Parser owns a runtime instance and instances cost memory.
//
// Parse, and every Tree, Node, and Query result derived from it, must be used
// from one goroutine at a time (strictcode's extraction is sequential).
//
// There is no fallback parser and no regex path; a parse failure is an error.
package treesitter

import (
	"bytes"
	"errors"
	"fmt"

	ts "github.com/cgofree/tree-sitter"
	tsgo "github.com/cgofree/tree-sitter-go"
	tspython "github.com/cgofree/tree-sitter-python"
	"github.com/cgofree/tree-sitter-typescript/tsx"
	"github.com/cgofree/tree-sitter-typescript/typescript"
)

// Node is a syntax node: a comparable value that keeps its tree alive.
type Node = ts.Node

// Grammar identifies a concrete tree-sitter grammar. The TS/JS profile column
// ("ts") spans two grammar variants: the typescript grammar parses .ts and
// plain JavaScript; the tsx grammar parses .tsx/.jsx (JSX syntax conflicts
// with TS type assertions, so tree-sitter ships them as separate grammars).
type Grammar int

const (
	GrammarPython Grammar = iota
	GrammarGo
	GrammarTypeScript
	GrammarTSX
)

func (g Grammar) String() string {
	switch g {
	case GrammarPython:
		return "python"
	case GrammarGo:
		return "go"
	case GrammarTypeScript:
		return "typescript"
	case GrammarTSX:
		return "tsx"
	}
	return fmt.Sprintf("Grammar(%d)", int(g))
}

// language returns the grammar's Language. Panics on an undefined grammar
// value: grammar selection is a closed set fixed at compile time, and an
// out-of-range value is a programming error, not an input condition.
func (g Grammar) language() *ts.Language {
	switch g {
	case GrammarPython:
		return tspython.Language()
	case GrammarGo:
		return tsgo.Language()
	case GrammarTypeScript:
		return typescript.Language()
	case GrammarTSX:
		return tsx.Language()
	}
	panic(fmt.Sprintf("treesitter: undefined grammar %d", int(g)))
}

// parsers holds one Parser per grammar, created on first use and kept for
// the life of the process: a Parser owns a runtime instance, which it
// replaces on its own once it has grown past the runtime's memory budget.
var parsers [GrammarTSX + 1]*ts.Parser

// parser returns the grammar's Parser, creating it the first time.
func (g Grammar) parser() (*ts.Parser, error) {
	lang := g.language()
	if p := parsers[g]; p != nil {
		return p, nil
	}
	p := ts.NewParser()
	if err := p.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("treesitter: grammar %s rejected by runtime: %w", g, err)
	}
	parsers[g] = p
	return p, nil
}

// GrammarForFile maps a filename to its grammar. The boolean is false for
// files strictcode does not parse. Extension mapping follows stricttools/docs/check-semantics.md
// (TS/JS resolution probes .ts/.tsx/.js/.jsx/.mjs/.cjs).
func GrammarForFile(filename string) (Grammar, bool) {
	dot := bytes.LastIndexByte([]byte(filename), '.')
	if dot < 0 {
		return 0, false
	}
	switch filename[dot:] {
	case ".py":
		return GrammarPython, true
	case ".go":
		return GrammarGo, true
	case ".ts", ".mts", ".cts", ".js", ".mjs", ".cjs":
		return GrammarTypeScript, true
	case ".tsx", ".jsx":
		return GrammarTSX, true
	}
	return 0, false
}

// NormalizeLF converts CRLF and lone CR line endings to LF. Canonical byte
// positions everywhere in strictcode are offsets into this normalized form
// (stricttools/docs/graph-model.md, spans and positions). The input slice is never modified; when no
// normalization is needed the input is returned as-is.
func NormalizeLF(src []byte) []byte {
	if !bytes.ContainsRune(src, '\r') {
		return src
	}
	out := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		if src[i] == '\r' {
			if i+1 < len(src) && src[i+1] == '\n' {
				continue // CRLF: drop the CR, keep the LF
			}
			out = append(out, '\n') // lone CR: becomes LF
			continue
		}
		out = append(out, src[i])
	}
	return out
}

// Tree is a parsed file: the LF-normalized source and the syntax tree over
// it. Node byte offsets index Source.
type Tree struct {
	// Source is the LF-normalized UTF-8 the tree was parsed from. All node
	// spans index into this slice, never into the raw file bytes. It must
	// not be modified: the syntax tree keeps it for Node.Text.
	Source  []byte
	Grammar Grammar

	tree *ts.Tree
}

// Parse normalizes src to LF and parses it with the grammar's Parser.
func Parse(g Grammar, src []byte) (*Tree, error) {
	normalized := NormalizeLF(src)
	parser, err := g.parser()
	if err != nil {
		return nil, err
	}
	t, err := parser.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("treesitter: grammar %s: %w", g, err)
	}
	return &Tree{Source: normalized, Grammar: g, tree: t}, nil
}

// Root returns the root node.
func (t *Tree) Root() Node {
	return t.tree.RootNode()
}

// HasParseErrors reports whether the tree contains ERROR or MISSING nodes.
// tree-sitter is error-tolerant; strictcode is honest about it — extractors
// consult this instead of silently analyzing a broken tree.
func (t *Tree) HasParseErrors() bool {
	return t.tree.RootNode().HasError()
}

// Query is a compiled tree-sitter query for one grammar. Queries are
// compiled once and reused across many trees.
type Query struct {
	Grammar Grammar

	query *ts.Query
	names []string
}

// CompileQuery compiles a query pattern against a grammar. Only the standard
// text filters (#eq?, #match?, #any-of?, ...) are accepted as predicates.
// Pattern errors are hard errors carrying the tree-sitter diagnostic.
func CompileQuery(g Grammar, pattern string) (*Query, error) {
	q, err := ts.NewQuery(g.language(), pattern)
	if err != nil {
		var qerr *ts.QueryError
		if errors.As(err, &qerr) {
			return nil, fmt.Errorf("treesitter: query for grammar %s: %s (row %d, column %d)", g, qerr.Message, qerr.Row, qerr.Column)
		}
		return nil, fmt.Errorf("treesitter: query for grammar %s: %w", g, err)
	}
	names := make([]string, q.CaptureCount())
	for i := range names {
		names[i] = q.CaptureNameForID(uint32(i))
	}
	return &Query{Grammar: g, query: q, names: names}, nil
}

// Capture is one captured node with its capture name.
type Capture struct {
	Name string
	Node Node
}

// Match is one query match: the pattern index within the query and its
// captures in capture order.
type Match struct {
	PatternIndex uint
	Captures     []Capture
}

// Matches runs the query over the tree and returns all matches with text
// predicates (#eq?, #match?, #any-of?, ...) applied.
// Matches panics if the query and tree grammars differ — that is a
// programming error in the caller, never an input condition.
func (q *Query) Matches(t *Tree) []Match {
	if q.Grammar != t.Grammar {
		panic(fmt.Sprintf("treesitter: query grammar %s run against tree grammar %s", q.Grammar, t.Grammar))
	}
	var out []Match
	for m := range q.query.Matches(t.Root()) {
		match := Match{PatternIndex: uint(m.PatternIndex)}
		for _, c := range m.Captures {
			match.Captures = append(match.Captures, Capture{Name: q.names[c.Index], Node: c.Node})
		}
		out = append(out, match)
	}
	return out
}
