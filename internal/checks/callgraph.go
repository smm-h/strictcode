package checks

// Round 3: the Python call-graph library rules (library-stdout,
// library-direct-logging) and unreachable-code, consuming the semantic
// extraction side tables.

import (
	"fmt"
	"strings"

	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/vocab"
)

// stdoutCallees are the Python standard-stream writers (DESIGN.md 6.5:
// print and sys.stdout/stderr writes are errors). Matched against the
// alias-expanded canonical callee.
var stdoutCallees = map[string]bool{
	"print":            true,
	"sys.stdout.write": true,
	"sys.stderr.write": true,
}

// loggingMethods are the root-logger methods whose direct use is the
// library-direct-logging diagnosis.
var loggingMethods = map[string]bool{
	"debug": true, "info": true, "warning": true, "warn": true,
	"error": true, "critical": true, "exception": true, "log": true,
}

// checkLibraryStdout: a library writing to standard streams. Library-only
// (lesson 22); test/example files excluded via the shared predicate
// (lesson 23); individual stream identifiers ignorable via the allow list.
func checkLibraryStdout(ctx *Context) []findings.Finding {
	setting := ctx.Cfg.Setting("library-stdout")
	allowed := map[string]bool{}
	for _, a := range setting.Allow[string(vocab.LangPy)] {
		allowed[a] = true
	}
	var out []findings.Finding
	for _, c := range ctx.View.Res.Calls {
		if c.TestContext || !stdoutCallees[c.Callee] || allowed[c.Callee] {
			continue
		}
		m := ctx.View.WS.MemberByName(c.Member)
		if m == nil || !m.Library {
			continue
		}
		out = append(out, ctx.finding("library-stdout",
			c.Container, ctx.containerKind(c.Container),
			c.File, c.Span.Start,
			fmt.Sprintf("library '%s' writes to a standard stream via %s", c.Member, c.Callee)))
	}
	return out
}

// checkLibraryDirectLogging: a Python library calling the root logger
// directly (logging.<method>) instead of taking a logger (lesson 27:
// warning severity, distinct from the stdout errors).
func checkLibraryDirectLogging(ctx *Context) []findings.Finding {
	setting := ctx.Cfg.Setting("library-direct-logging")
	allowed := map[string]bool{}
	for _, a := range setting.Allow[string(vocab.LangPy)] {
		allowed[a] = true
	}
	var out []findings.Finding
	for _, c := range ctx.View.Res.Calls {
		if c.TestContext || allowed[c.Callee] {
			continue
		}
		rest, ok := strings.CutPrefix(c.Callee, "logging.")
		if !ok || !loggingMethods[rest] {
			continue
		}
		m := ctx.View.WS.MemberByName(c.Member)
		if m == nil || !m.Library {
			continue
		}
		out = append(out, ctx.finding("library-direct-logging",
			c.Container, ctx.containerKind(c.Container),
			c.File, c.Span.Start,
			fmt.Sprintf("library '%s' calls the root logger directly (%s); take a logger instead", c.Member, c.Callee)))
	}
	return out
}

// checkUnreachableCode: statements following an unconditional terminator
// (all projects — the approved departure from the donor's library-only
// scoping). Comment-aware and nested-scope-independent by construction in
// the extractor (lessons 24-25). Path-suppressable; offers the tier-1
// removal fix.
func checkUnreachableCode(ctx *Context) []findings.Finding {
	suppressed := ctx.suppressedPaths("unreachable-code")
	var out []findings.Finding
	for _, u := range ctx.View.Res.Unreachable {
		if suppressed[u.File] {
			continue
		}
		f := ctx.finding("unreachable-code",
			u.Container, ctx.containerKind(u.Container),
			u.File, u.FirstLineSpan.Start,
			"unreachable code: statements after an unconditional terminator")
		f.Fix = unreachableFixDescriptor()
		out = append(out, f)
	}
	return out
}

// unreachableFixDescriptor is the tier-1 fix offer attached to
// unreachable-code findings; the transform lives in internal/fix.
func unreachableFixDescriptor() *findings.Fix {
	return &findings.Fix{
		Tier:        1,
		Description: "Remove the unreachable statements (whitelisted transform; verified by post-fix graph re-extraction).",
	}
}

// containerKindOf looks up the precise node kind of a serialized container
// ID in the relation's node table; unknown IDs (never emitted) label as
// module.
func (ctx *Context) containerKind(id string) vocab.NodeKind {
	if k, ok := ctx.View.NodeKinds[id]; ok {
		return k
	}
	return vocab.NodeKindModule
}
