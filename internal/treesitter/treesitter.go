// Package treesitter is strictcode's single parsing path: a thin, disciplined
// layer over the official tree-sitter CGo bindings (the binding-benchmark
// winner; see BUILDLOG.md). It owns the three responsibilities the rest of
// the codebase must never re-implement:
//
//   - grammar selection for the language trio (Python, Go, TS/JS);
//   - LF normalization before parsing, so every byte span in the system is a
//     span over LF-normalized UTF-8 (schema/SPEC.md section 3);
//   - CGo resource lifecycle (Close on parsers, trees, queries, cursors),
//     so extractors cannot leak C memory.
//
// There is no fallback parser and no regex path; a parse failure is an error.
package treesitter

import (
	"bytes"
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

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

// language returns the sitter.Language for a grammar. Panics on an undefined
// grammar value: grammar selection is a closed set fixed at compile time, and
// an out-of-range value is a programming error, not an input condition.
func (g Grammar) language() *sitter.Language {
	switch g {
	case GrammarPython:
		return sitter.NewLanguage(tspython.Language())
	case GrammarGo:
		return sitter.NewLanguage(tsgo.Language())
	case GrammarTypeScript:
		return sitter.NewLanguage(tsts.LanguageTypescript())
	case GrammarTSX:
		return sitter.NewLanguage(tsts.LanguageTSX())
	}
	panic(fmt.Sprintf("treesitter: undefined grammar %d", int(g)))
}

// GrammarForFile maps a filename to its grammar. The boolean is false for
// files strictcode does not parse. Extension mapping follows DESIGN.md
// section 6.2 (TS/JS resolution probes .ts/.tsx/.js/.jsx/.mjs/.cjs).
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
// (schema/SPEC.md section 3). The input slice is never modified; when no
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
// it. Node byte offsets index Source. Callers must Close it.
type Tree struct {
	// Source is the LF-normalized UTF-8 the tree was parsed from. All node
	// spans index into this slice, never into the raw file bytes.
	Source  []byte
	Grammar Grammar

	tree *sitter.Tree
}

// Parse normalizes src to LF and parses it with the given grammar. The
// returned tree must be Closed. A nil tree from the runtime (the only
// failure mode of ts_parser_parse without timeouts/cancellation, e.g. a
// grammar/runtime version mismatch) is a hard error.
func Parse(g Grammar, src []byte) (*Tree, error) {
	normalized := NormalizeLF(src)
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(g.language()); err != nil {
		return nil, fmt.Errorf("treesitter: grammar %s rejected by runtime: %w", g, err)
	}
	t := parser.Parse(normalized, nil)
	if t == nil {
		return nil, fmt.Errorf("treesitter: parser returned no tree for grammar %s", g)
	}
	return &Tree{Source: normalized, Grammar: g, tree: t}, nil
}

// Root returns the root node. Valid only until Close.
func (t *Tree) Root() *sitter.Node {
	return t.tree.RootNode()
}

// HasParseErrors reports whether the tree contains ERROR or MISSING nodes.
// tree-sitter is error-tolerant; strictcode is honest about it — extractors
// consult this instead of silently analyzing a broken tree.
func (t *Tree) HasParseErrors() bool {
	return t.tree.RootNode().HasError()
}

// Close releases the underlying C tree. Idempotent.
func (t *Tree) Close() {
	if t.tree != nil {
		t.tree.Close()
		t.tree = nil
	}
}

// Query is a compiled tree-sitter query for one grammar. Callers must Close
// it. Queries are compiled once and reused across many trees.
type Query struct {
	Grammar Grammar

	query *sitter.Query
	names []string
}

// CompileQuery compiles a query pattern against a grammar. Pattern errors are
// hard errors carrying the tree-sitter diagnostic.
func CompileQuery(g Grammar, pattern string) (*Query, error) {
	q, qerr := sitter.NewQuery(g.language(), pattern)
	if qerr != nil {
		return nil, fmt.Errorf("treesitter: query for grammar %s: %s (row %d, column %d)", g, qerr.Message, qerr.Row, qerr.Column)
	}
	return &Query{Grammar: g, query: q, names: q.CaptureNames()}, nil
}

// Close releases the underlying C query. Idempotent.
func (q *Query) Close() {
	if q.query != nil {
		q.query.Close()
		q.query = nil
	}
}

// Capture is one captured node with its capture name.
type Capture struct {
	Name string
	Node sitter.Node
}

// Match is one query match: the pattern index within the query and its
// captures in capture order.
type Match struct {
	PatternIndex uint
	Captures     []Capture
}

// Matches runs the query over the tree and returns all matches with text
// predicates (#eq?, #match?, #any-of?, ...) applied. The cursor is created
// and closed internally; returned nodes are valid until the tree is Closed.
// Matches panics if the query and tree grammars differ — that is a
// programming error in the caller, never an input condition.
func (q *Query) Matches(t *Tree) []Match {
	if q.Grammar != t.Grammar {
		panic(fmt.Sprintf("treesitter: query grammar %s run against tree grammar %s", q.Grammar, t.Grammar))
	}
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	var out []Match
	matches := cursor.Matches(q.query, t.Root(), t.Source)
	for m := matches.Next(); m != nil; m = matches.Next() {
		match := Match{PatternIndex: m.PatternIndex}
		for _, c := range m.Captures {
			match.Captures = append(match.Captures, Capture{Name: q.names[c.Index], Node: c.Node})
		}
		out = append(out, match)
	}
	return out
}
