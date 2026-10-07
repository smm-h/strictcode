package checks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// constraintVerdict is what evaluating a version constraint against a
// version decides.
type constraintVerdict int

const (
	// constraintSatisfied: the version satisfies the constraint.
	constraintSatisfied constraintVerdict = iota
	// constraintOutdated: the version does not satisfy the constraint.
	constraintOutdated
	// constraintUnevaluated: the constraint or the version is outside the
	// simple forms evaluated here (several conditions, a wildcard, a
	// pre-release), so no verdict is given.
	constraintUnevaluated
)

// checkDepsStale: an intra-workspace dependency whose manifest constraint
// the dependency member's declared version does not satisfy. Only
// registry-sourced declarations carry a constraint to judge; path and
// workspace-protocol sources resolve to the member itself. A constraint or
// version in a form evaluateConstraint does not evaluate gives no finding,
// and so does a dependency member whose manifest declares no version.
func checkDepsStale(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, m := range ctx.View.WS.Members {
		for _, lang := range vocab.Langs {
			if langNA("deps-stale", lang) {
				continue
			}
			mf := m.Manifests[lang]
			if mf == nil {
				continue
			}
			for _, dep := range mf.Deps {
				if dep.Source != workspace.SourceRegistry || dep.Constraint == "" {
					continue
				}
				target := extract.DependencyMember(ctx.View.WS, lang, m, dep.Name)
				if target == nil {
					continue
				}
				tmf := target.Manifests[lang]
				if tmf == nil || tmf.Version == "" {
					continue
				}
				if evaluateConstraint(dep.Constraint, tmf.Version) != constraintOutdated {
					continue
				}
				if ctx.suppressedPair("deps-stale", m.Name, target.Name) {
					continue
				}
				site := ctx.declarationSite(lang, m.Name, target.Name, mf.Path)
				out = append(out, ctx.finding("deps-stale",
					memberTargetID(ctx, lang, m.Name), vocab.NodeKindWorkspaceMember,
					site.File, site.Span.Start,
					fmt.Sprintf("'%s' depends on '%s' %s but '%s' is now %s",
						m.Name, target.Name, dep.Constraint, target.Name, tmf.Version)))
			}
		}
	}
	return out
}

// declarationSite is the declares_dependency row of src on dst in lang, or
// a row naming only the manifest when there is none.
func (ctx *Context) declarationSite(lang vocab.Lang, src, dst, manifest string) relation.Row {
	if rows := ctx.View.DeclaredDeps[memberEdge{lang, src, dst}]; len(rows) > 0 {
		return rows[0]
	}
	return relation.Row{File: manifest}
}

// evaluateConstraint judges version against one simple constraint: an
// operator (>=, >, <=, <, ==, =, ~=, ^, or ~) or none (an exact version)
// followed by a dotted numeric version. Caret keeps the major version (the
// minor for a 0.x constraint); tilde and ~= keep the major and minor.
// Several conditions (a comma, "||", or a space between parts), "!=",
// wildcards, and non-numeric versions are not evaluated.
func evaluateConstraint(constraint, version string) constraintVerdict {
	current, ok := parseVersionTuple(version)
	if !ok {
		return constraintUnevaluated
	}
	c := strings.TrimSpace(constraint)
	if c == "" || strings.Contains(c, ",") || strings.Contains(c, "||") || strings.HasPrefix(c, "!=") {
		return constraintUnevaluated
	}
	op := "=="
	for _, candidate := range []string{">=", "<=", "==", "~=", ">", "<", "^", "~", "="} {
		if strings.HasPrefix(c, candidate) {
			op = candidate
			c = strings.TrimSpace(c[len(candidate):])
			break
		}
	}
	if op == "=" {
		op = "=="
	}
	want, ok := parseVersionTuple(c)
	if !ok {
		return constraintUnevaluated
	}
	verdict := func(satisfied bool) constraintVerdict {
		if satisfied {
			return constraintSatisfied
		}
		return constraintOutdated
	}
	cmp := compareTuples(current, want)
	switch op {
	case ">=":
		return verdict(cmp >= 0)
	case ">":
		return verdict(cmp > 0)
	case "<=":
		return verdict(cmp <= 0)
	case "<":
		return verdict(cmp < 0)
	case "==":
		return verdict(cmp == 0)
	case "^":
		if cmp < 0 {
			return constraintOutdated
		}
		if want[0] > 0 {
			return verdict(current[0] == want[0])
		}
		if len(want) >= 2 && len(current) >= 2 {
			return verdict(current[0] == 0 && current[1] == want[1])
		}
		return constraintUnevaluated
	default: // "~" and "~="
		if cmp < 0 {
			return constraintOutdated
		}
		if len(want) >= 2 && len(current) >= 2 {
			return verdict(current[0] == want[0] && current[1] == want[1])
		}
		return constraintUnevaluated
	}
}

// parseVersionTuple parses a dotted numeric version ("1.2.3"); any other
// form is not a tuple.
func parseVersionTuple(s string) ([]int, bool) {
	if s == "" {
		return nil, false
	}
	var out []int
	for _, part := range strings.Split(s, ".") {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return nil, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// compareTuples compares version tuples element by element, a shorter tuple
// that is a prefix of a longer one ranking below it.
func compareTuples(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}
