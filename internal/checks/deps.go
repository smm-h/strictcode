package checks

import (
	"fmt"
	"sort"

	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// declaredEdge is one member's declared dependency on a sibling, with the
// matching imports rows.
type declaredEdge struct {
	lang    vocab.Lang
	src     string
	dst     string
	scope   workspace.DepScope
	declRow relation.Row
	// imports: the src member's import rows targeting dst, excluding
	// type-checking rows (lesson 5: excluded from both dep directions).
	imports []relation.Row
}

// declaredEdges enumerates declared edges in deterministic order.
func (ctx *Context) declaredEdges() []declaredEdge {
	keys := make([]memberEdge, 0, len(ctx.View.DeclaredDeps))
	for e := range ctx.View.DeclaredDeps {
		keys = append(keys, e)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.Lang != b.Lang {
			return a.Lang < b.Lang
		}
		if a.Src != b.Src {
			return a.Src < b.Src
		}
		return a.Dst < b.Dst
	})
	var out []declaredEdge
	for _, e := range keys {
		imports := nonTypeChecking(ctx.View.MemberImports[e])
		for _, declRow := range ctx.View.DeclaredDeps[e] {
			out = append(out, declaredEdge{
				lang: e.Lang, src: e.Src, dst: e.Dst,
				scope: rowScope(declRow), declRow: declRow, imports: imports,
			})
		}
	}
	return out
}

func nonTypeChecking(rows []relation.Row) []relation.Row {
	var out []relation.Row
	for _, r := range rows {
		if !rowBool(r, "type_checking") {
			out = append(out, r)
		}
	}
	return out
}

// checkDepsUnused: a declared workspace-internal dependency no source file
// imports. ANY import — test-context or guarded included — marks the dep
// used (lessons 1, 8); only type-checking imports never count (lesson 5).
// The contradictory hard-dep-guarded-only case is deps-hard-guarded-only's
// diagnosis, not a deps-unused one.
func checkDepsUnused(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, e := range ctx.declaredEdges() {
		if len(e.imports) > 0 {
			continue
		}
		if ctx.suppressedPair("deps-unused", e.src, e.dst) {
			continue
		}
		out = append(out, ctx.finding("deps-unused",
			memberTargetID(ctx, e.lang, e.src), vocab.NodeKindWorkspaceMember,
			e.declRow.File, e.declRow.Span.Start,
			fmt.Sprintf("declared dependency '%s' is never imported by '%s'", e.dst, e.src)))
	}
	return out
}

// checkDepsHardGuardedOnly: a hard dependency (runtime/explicit) imported
// only under optional-import guards (lesson 2).
func checkDepsHardGuardedOnly(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, e := range ctx.declaredEdges() {
		if e.scope.Optional() || len(e.imports) == 0 {
			continue
		}
		allGuarded := true
		for _, r := range e.imports {
			if !rowBool(r, "guarded") {
				allGuarded = false
				break
			}
		}
		if !allGuarded {
			continue
		}
		if ctx.suppressedPair("deps-hard-guarded-only", e.src, e.dst) {
			continue
		}
		site := firstRow(e.imports)
		out = append(out, ctx.finding("deps-hard-guarded-only",
			memberTargetID(ctx, e.lang, e.src), vocab.NodeKindWorkspaceMember,
			site.File, site.Span.Start,
			fmt.Sprintf("hard dependency '%s' (scope %s) is imported only under optional-import guards: declare it optional or import it unconditionally", e.dst, e.scope)))
	}
	return out
}

// checkDepsUndeclared: production source importing a workspace member the
// manifest does not declare. Exempt: test-context, guarded, type-checking,
// and self-imports (lessons 4, 5).
func checkDepsUndeclared(ctx *Context) []findings.Finding {
	keys := make([]memberEdge, 0, len(ctx.View.MemberImports))
	for e := range ctx.View.MemberImports {
		keys = append(keys, e)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.Lang != b.Lang {
			return a.Lang < b.Lang
		}
		if a.Src != b.Src {
			return a.Src < b.Src
		}
		return a.Dst < b.Dst
	})
	var out []findings.Finding
	for _, e := range keys {
		if e.Src == e.Dst {
			continue // self-imports exempt
		}
		if len(ctx.View.DeclaredDeps[e]) > 0 {
			continue
		}
		var qualifying []relation.Row
		for _, r := range ctx.View.MemberImports[e] {
			if rowBool(r, "test_context") || rowBool(r, "guarded") || rowBool(r, "type_checking") {
				continue
			}
			qualifying = append(qualifying, r)
		}
		if len(qualifying) == 0 {
			continue
		}
		if ctx.suppressedPair("deps-undeclared", e.Src, e.Dst) {
			continue
		}
		site := firstRow(qualifying)
		out = append(out, ctx.finding("deps-undeclared",
			site.Src.String(), vocab.NodeKindModule,
			site.File, site.Span.Start,
			fmt.Sprintf("production code imports workspace member '%s' but '%s' does not declare it", e.Dst, e.Src)))
	}
	return out
}

// checkDepsRuntimeTestOnly: a runtime-scoped dependency imported only by
// test code (root-relative test classification — lesson 6).
func checkDepsRuntimeTestOnly(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, e := range ctx.declaredEdges() {
		if e.scope != workspace.ScopeRuntime || len(e.imports) == 0 {
			continue
		}
		allTest := true
		for _, r := range e.imports {
			if !rowBool(r, "test_context") {
				allTest = false
				break
			}
		}
		if !allTest {
			continue
		}
		if ctx.suppressedPair("deps-runtime-test-only", e.src, e.dst) {
			continue
		}
		out = append(out, ctx.finding("deps-runtime-test-only",
			memberTargetID(ctx, e.lang, e.src), vocab.NodeKindWorkspaceMember,
			e.declRow.File, e.declRow.Span.Start,
			fmt.Sprintf("runtime dependency '%s' is imported only by test code: rescope it to dev", e.dst)))
	}
	return out
}

// checkDepsDevInProduction: a dev-scoped dependency imported by production
// code. Guarded production imports are the legitimate optional-dependency
// pattern and are exempt (the rule's uses of import-attr-guarded).
func checkDepsDevInProduction(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, e := range ctx.declaredEdges() {
		if e.scope != workspace.ScopeDev {
			continue
		}
		var production []relation.Row
		for _, r := range e.imports {
			if !rowBool(r, "test_context") && !rowBool(r, "guarded") {
				production = append(production, r)
			}
		}
		if len(production) == 0 {
			continue
		}
		if ctx.suppressedPair("deps-dev-in-production", e.src, e.dst) {
			continue
		}
		site := firstRow(production)
		out = append(out, ctx.finding("deps-dev-in-production",
			site.Src.String(), vocab.NodeKindModule,
			site.File, site.Span.Start,
			fmt.Sprintf("dev dependency '%s' is imported by production code in '%s'", e.dst, e.src)))
	}
	return out
}

// memberTargetID returns the serialized member node ID for a finding
// target.
func memberTargetID(ctx *Context, lang vocab.Lang, member string) string {
	if id, ok := ctx.View.memberNodeIDs[langMember{lang, member}]; ok {
		return id
	}
	return relation.NodeID{Lang: string(lang), Member: member, Module: "_"}.String()
}
