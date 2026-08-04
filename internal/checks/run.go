package checks

import (
	"sort"

	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// Context carries the shared inputs of one check run.
type Context struct {
	View *View
	Cfg  *config.Effective
	// CfgPath is the workspace-root-relative config file path (the site of
	// stale-suppression findings).
	CfgPath string
}

// checkFn implements one rule over the view.
type checkFn func(ctx *Context) []findings.Finding

// implemented maps rule IDs to their implementations — all fourteen minted
// rules as of round 3.
var implemented = map[string]checkFn{
	"deps-unused":               checkDepsUnused,
	"deps-hard-guarded-only":    checkDepsHardGuardedOnly,
	"deps-undeclared":           checkDepsUndeclared,
	"deps-runtime-test-only":    checkDepsRuntimeTestOnly,
	"deps-dev-in-production":    checkDepsDevInProduction,
	"dead-modules":              checkDeadModules,
	"dead-workspace-packages":   checkDeadWorkspacePackages,
	"import-cycles":             checkImportCycles,
	"stale-suppression":         checkStaleSuppression,
	"library-forbidden-imports": checkLibraryForbiddenImports,
	"library-entry-point":       checkLibraryEntryPoint,
	"library-stdout":            checkLibraryStdout,
	"library-direct-logging":    checkLibraryDirectLogging,
	"unreachable-code":          checkUnreachableCode,
}

// Run executes every enabled, implemented check over the one shared
// relation (lesson 30) and returns the sorted findings.
func Run(ws *workspace.Workspace, res *extract.Result, cfg *config.Effective, cfgPath string) []findings.Finding {
	ctx := &Context{
		View:    buildView(ws, res),
		Cfg:     cfg,
		CfgPath: cfgPath,
	}
	ids := make([]string, 0, len(implemented))
	for id := range implemented {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []findings.Finding
	for _, id := range ids {
		if !cfg.Setting(id).Enabled {
			continue
		}
		out = append(out, implemented[id](ctx)...)
	}
	findings.Sort(out)
	return out
}

// --- shared helpers -------------------------------------------------------

// finding constructs a finding with the rule's effective severity.
func (ctx *Context) finding(ruleID string, targetID string, kind vocab.NodeKind, file string, offset uint32, message string) findings.Finding {
	line := 1
	if file != "" {
		line = ctx.View.Res.Line(file, offset)
	}
	return findings.Finding{
		Rule:     ruleID,
		Severity: ctx.Cfg.Setting(ruleID).Severity,
		Message:  message,
		Target: findings.Target{
			ID:   targetID,
			Kind: kind,
			File: file,
			Line: line,
		},
	}
}

// langNA reports whether a rule's matrix cell is not-applicable for a
// language (e.g. import-cycles on Go — lesson 20).
func langNA(ruleID string, lang vocab.Lang) bool {
	r, ok := rules.ByID(ruleID)
	if !ok {
		return true
	}
	return rules.MatrixCell(r, lang).Status == rules.CellNotApplicable
}

// suppressedPair reports whether a (project, dep) suppression exists for
// the rule.
func (ctx *Context) suppressedPair(ruleID, project, dep string) bool {
	for _, s := range ctx.Cfg.Setting(ruleID).Suppressions {
		if s.Shape == rules.SuppressProjectDep && s.Project == project && s.Dep == dep {
			return true
		}
	}
	return false
}

// suppressedPaths returns the rule's path suppressions as a set of
// workspace-root-relative paths.
func (ctx *Context) suppressedPaths(ruleID string) map[string]bool {
	out := map[string]bool{}
	for _, s := range ctx.Cfg.Setting(ruleID).Suppressions {
		if s.Shape == rules.SuppressPath {
			out[s.Path] = true
		}
	}
	return out
}

// suppressedMember reports whether a member-shaped suppression names the
// member.
func (ctx *Context) suppressedMember(ruleID, member string) bool {
	for _, s := range ctx.Cfg.Setting(ruleID).Suppressions {
		if s.Shape == rules.SuppressMember && s.Member == member {
			return true
		}
	}
	return false
}

// suppressedModuleSet reports whether a member-set suppression matches the
// sorted module set exactly.
func (ctx *Context) suppressedModuleSet(ruleID string, sortedModules []string) bool {
	for _, s := range ctx.Cfg.Setting(ruleID).Suppressions {
		if s.Shape != rules.SuppressMemberSet || len(s.Modules) != len(sortedModules) {
			continue
		}
		match := true
		for i := range s.Modules {
			if s.Modules[i] != sortedModules[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// firstRow returns the canonical-first row (rows arrive in canonical order).
func firstRow(rows []relation.Row) relation.Row {
	return rows[0]
}
