package extract

import (
	"strings"

	"github.com/smm-h/strictcode/internal/treesitter"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// Standard-stream writes in Go and TypeScript/JavaScript are recorded as
// call sites by their syntactic callee, the qualified name the call is
// written with, for the library-stdout rule. Python's call sites come from
// the full semantic extraction instead, with aliases expanded.

// goStreamCallees are the Go calls that write to a standard stream by
// themselves.
var goStreamCallees = map[string]bool{
	"fmt.Print":             true,
	"fmt.Printf":            true,
	"fmt.Println":           true,
	"os.Stdout.Write":       true,
	"os.Stdout.WriteString": true,
	"os.Stderr.Write":       true,
	"os.Stderr.WriteString": true,
	"print":                 true,
	"println":               true,
}

// goFprintCallees write to their first argument, a standard stream when
// that argument is os.Stdout or os.Stderr; the call site is recorded as
// "fmt.Fprintln(os.Stdout)".
var goFprintCallees = map[string]bool{
	"fmt.Fprint":   true,
	"fmt.Fprintf":  true,
	"fmt.Fprintln": true,
}

// tsStreamCallees are the TypeScript/JavaScript calls that write to a
// standard stream.
var tsStreamCallees = map[string]bool{
	"console.log":          true,
	"console.info":         true,
	"console.warn":         true,
	"console.error":        true,
	"console.debug":        true,
	"console.trace":        true,
	"process.stdout.write": true,
	"process.stderr.write": true,
}

var streamQueries = struct {
	goCalls  *treesitter.Query
	tsCalls  *treesitter.Query
	tsxCalls *treesitter.Query
}{}

func initStreamQueries() {
	if streamQueries.goCalls != nil {
		return
	}
	mustCompile := func(g treesitter.Grammar, pattern string) *treesitter.Query {
		q, err := treesitter.CompileQuery(g, pattern)
		if err != nil {
			panic(err)
		}
		return q
	}
	streamQueries.goCalls = mustCompile(treesitter.GrammarGo,
		`(call_expression function: [(selector_expression) (identifier)] @callee arguments: (argument_list) @args)`)
	const tsPattern = `(call_expression function: (member_expression) @callee)`
	streamQueries.tsCalls = mustCompile(treesitter.GrammarTypeScript, tsPattern)
	streamQueries.tsxCalls = mustCompile(treesitter.GrammarTSX, tsPattern)
}

// compactCallee is a callee's text without whitespace, so a call written
// across lines ("fmt.\n\tPrintln") reads as one name.
func compactCallee(text string) string {
	return strings.Join(strings.Fields(text), "")
}

// recordGoStreamWrites adds a call site for every standard-stream write in
// one parsed Go file of package pkgDir.
func (ex *extraction) recordGoStreamWrites(m *workspace.Member, tree *treesitter.Tree, pkgDir, wsPath string, isTest bool) {
	initStreamQueries()
	container := moduleNodeID(vocab.LangGo, m.Name, pkgDir).String()
	for _, match := range streamQueries.goCalls.Matches(tree) {
		var callee, args treesitter.Node
		for _, c := range match.Captures {
			switch c.Name {
			case "callee":
				callee = c.Node
			case "args":
				args = c.Node
			}
		}
		name := compactCallee(callee.Text())
		switch {
		case goStreamCallees[name]:
		case goFprintCallees[name] && args.NamedChildCount() > 0:
			first := compactCallee(namedChildAt(args, 0).Text())
			if first != "os.Stdout" && first != "os.Stderr" {
				continue
			}
			name += "(" + first + ")"
		default:
			continue
		}
		ex.callSites = append(ex.callSites, CallSite{
			Lang: vocab.LangGo, Member: m.Name, Module: pkgDir, Container: container,
			Callee: name, Resolution: CallUnresolved,
			File: wsPath, Span: spanOf(callee), TestContext: isTest,
		})
	}
}

// recordTSStreamWrites adds a call site for every standard-stream write in
// one parsed TypeScript/JavaScript file of module logical.
func (ex *extraction) recordTSStreamWrites(m *workspace.Member, tree *treesitter.Tree, grammar treesitter.Grammar, logical, wsPath string, isTest bool) {
	initStreamQueries()
	query := streamQueries.tsCalls
	if grammar == treesitter.GrammarTSX {
		query = streamQueries.tsxCalls
	}
	container := moduleNodeID(vocab.LangTS, m.Name, logical).String()
	for _, match := range query.Matches(tree) {
		for _, c := range match.Captures {
			name := compactCallee(c.Node.Text())
			if !tsStreamCallees[name] {
				continue
			}
			ex.callSites = append(ex.callSites, CallSite{
				Lang: vocab.LangTS, Member: m.Name, Module: logical, Container: container,
				Callee: name, Resolution: CallUnresolved,
				File: wsPath, Span: spanOf(c.Node), TestContext: isTest,
			})
		}
	}
}
