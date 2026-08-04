package checks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/vocab"
)

// checkDeadModules dispatches the per-language dead-module algorithms
// pinned in DESIGN.md 6.2: union-of-imports for Python and Go, BFS from
// entry points for TS/JS.
func checkDeadModules(ctx *Context) []findings.Finding {
	var out []findings.Finding
	suppressed := ctx.suppressedPaths("dead-modules")
	for _, m := range ctx.View.WS.Members {
		out = append(out, deadPy(ctx, m.Name, suppressed)...)
		out = append(out, deadGo(ctx, m.Name, suppressed)...)
		out = append(out, deadTS(ctx, m.Name, suppressed)...)
	}
	return out
}

// pyScriptFile reports whether a module is a file directly under a root
// scripts/ directory (lesson 15: excluded from candidates AND from the
// reference union).
func pyScriptFile(mi *ModuleInfo) bool {
	parts := strings.Split(mi.MemberRel, "/")
	return len(parts) == 2 && parts[0] == "scripts"
}

// deadPy: union-of-imports. A module is dead when (a) no other eligible
// production module's imports reference it by dotted-name prefix and (b)
// its leaf is not exported by any __init__.py (lesson 16). Suppressed and
// scripts/ modules leave both the candidate set and the reference union
// (lessons 14, 15).
func deadPy(ctx *Context, member string, suppressed map[string]bool) []findings.Finding {
	lm := langMember{vocab.LangPy, member}
	modules := ctx.View.Modules[lm]
	if len(modules) == 0 {
		return nil
	}
	eligible := func(mi *ModuleInfo) bool {
		return !mi.Test && !pyScriptFile(mi) && !suppressed[mi.Path]
	}
	// pkg/__main__.py is the `python -m pkg` entry point: an implicit
	// entry, never a candidate (its imports still count — it is production
	// code that keeps its imports alive).
	implicitEntry := func(mi *ModuleInfo) bool {
		return strings.HasSuffix(mi.MemberRel, "/__main__.py") || mi.MemberRel == "__main__.py"
	}

	// aliveRefs: logical -> set of referencing source logicals.
	aliveRefs := map[string]map[string]bool{}
	mark := func(target, src string) {
		if aliveRefs[target] == nil {
			aliveRefs[target] = map[string]bool{}
		}
		aliveRefs[target][src] = true
	}
	for srcLogical, edges := range ctx.View.ModuleImports[lm] {
		src := modules[srcLogical]
		if src == nil || !eligible(src) {
			continue
		}
		for _, e := range edges {
			// An import of pkg.sub.mod references pkg.sub and pkg too
			// (dotted-name prefix semantics).
			target := e.Dst
			for {
				mark(target, srcLogical)
				i := strings.LastIndexByte(target, '.')
				if i < 0 {
					break
				}
				target = target[:i]
			}
		}
	}
	exported := map[string]bool{}
	for _, e := range ctx.View.Exports[lm] {
		src := modules[e.Src]
		if src != nil && eligible(src) {
			exported[e.Dst] = true
		}
	}

	var out []findings.Finding
	for _, logical := range sortedKeys(modules) {
		mi := modules[logical]
		if !eligible(mi) || implicitEntry(mi) {
			continue
		}
		alive := exported[logical]
		for src := range aliveRefs[logical] {
			if src != logical {
				alive = true
				break
			}
		}
		if alive {
			continue
		}
		out = append(out, ctx.finding("dead-modules",
			mi.ID.String(), vocab.NodeKindModule, mi.Path, 0,
			fmt.Sprintf("module %s is not imported by any production module and not exported by any __init__.py", logical)))
	}
	return out
}

// deadGo: union-of-imports, package-granular. Only packages under an
// internal/ path component are candidates; test-context packages never are
// (lesson 9). A package is dead when no non-test file outside it imports
// its path; suppressed packages leave the reference union too (lesson 14).
func deadGo(ctx *Context, member string, suppressed map[string]bool) []findings.Finding {
	lm := langMember{vocab.LangGo, member}
	modules := ctx.View.Modules[lm]
	if len(modules) == 0 {
		return nil
	}
	underInternal := func(mi *ModuleInfo) bool {
		for _, part := range strings.Split(mi.MemberRel, "/") {
			if part == "internal" {
				return true
			}
		}
		return false
	}

	// alive: package logical -> imported by a non-test file of another,
	// non-suppressed package.
	alive := map[string]bool{}
	for srcLogical, edges := range ctx.View.ModuleImports[lm] {
		src := modules[srcLogical]
		if src == nil || suppressed[src.Path] {
			continue
		}
		for _, e := range edges {
			if e.Dst == srcLogical || rowBool(e.Row, "test_context") {
				continue
			}
			alive[e.Dst] = true
		}
	}

	var out []findings.Finding
	for _, logical := range sortedKeys(modules) {
		mi := modules[logical]
		if mi.Test || !underInternal(mi) || suppressed[mi.Path] || alive[logical] {
			continue
		}
		out = append(out, ctx.finding("dead-modules",
			mi.ID.String(), vocab.NodeKindModule, mi.Path, 0,
			fmt.Sprintf("package %s is not imported by any non-test file outside it", logical)))
	}
	return out
}

// deadTS: BFS from entry points over the resolved import graph. A
// suppressed unit's edges are never traversed (lesson 14, BFS form).
func deadTS(ctx *Context, member string, suppressed map[string]bool) []findings.Finding {
	lm := langMember{vocab.LangTS, member}
	modules := ctx.View.Modules[lm]
	if len(modules) == 0 {
		return nil
	}
	// Donor safeguard: no resolved entry points (e.g. exports pointing at
	// built dist/ output) means reachability cannot be determined — abstain
	// rather than report the whole tree dead.
	if len(ctx.View.EntryTargets[lm]) == 0 {
		return nil
	}

	reachable := map[string]bool{}
	var queue []string
	for _, target := range ctx.View.EntryTargets[lm] {
		if !reachable[target] {
			reachable[target] = true
			queue = append(queue, target)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		mi := modules[cur]
		if mi == nil || suppressed[mi.Path] {
			continue // suppressed units' edges are never traversed
		}
		for _, e := range ctx.View.ModuleImports[lm][cur] {
			if !reachable[e.Dst] {
				reachable[e.Dst] = true
				queue = append(queue, e.Dst)
			}
		}
	}

	var out []findings.Finding
	for _, logical := range sortedKeys(modules) {
		mi := modules[logical]
		if mi.Test || suppressed[mi.Path] || reachable[logical] {
			continue
		}
		out = append(out, ctx.finding("dead-modules",
			mi.ID.String(), vocab.NodeKindModule, mi.Path, 0,
			fmt.Sprintf("file %s is unreachable from every entry point", logical)))
	}
	return out
}

// checkDeadWorkspacePackages: a library member no sibling imports. Dev-only,
// non-library, and published releasable members are exempt; self-imports
// never count; test-only importers get a distinct message (lesson 28).
func checkDeadWorkspacePackages(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, m := range ctx.View.WS.Members {
		if !m.Library || m.DevOnly || m.Releasable {
			continue
		}
		if ctx.suppressedMember("dead-workspace-packages", m.Name) {
			continue
		}
		production, test := 0, 0
		var targetLang vocab.Lang
		found := false
		for _, lang := range vocab.Langs {
			lm := langMember{lang, m.Name}
			if _, ok := ctx.View.memberNodeIDs[lm]; !ok {
				continue
			}
			if !found {
				targetLang, found = lang, true
			}
			for e, rows := range ctx.View.MemberImports {
				if e.Lang != lang || e.Dst != m.Name || e.Src == m.Name {
					continue
				}
				for _, r := range rows {
					if rowBool(r, "test_context") {
						test++
					} else {
						production++
					}
				}
			}
		}
		if !found || production > 0 {
			continue
		}
		file := m.Path
		if mf := m.Manifests[targetLang]; mf != nil {
			file = mf.Path
		}
		msg := fmt.Sprintf("library member '%s' is imported by no sibling member", m.Name)
		if test > 0 {
			msg = fmt.Sprintf("library member '%s' is imported only by sibling test code", m.Name)
		}
		out = append(out, ctx.finding("dead-workspace-packages",
			memberTargetID(ctx, targetLang, m.Name), vocab.NodeKindWorkspaceMember,
			file, 0, msg))
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
