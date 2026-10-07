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
				if evaluateConstraint(lang, dep.Constraint, tmf.Version) != constraintOutdated {
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

// evaluateConstraint judges version against one simple constraint of the
// language's ecosystem: an operator followed by a dotted numeric version.
//
//   - Python (PEP 440) has >=, >, <=, <, ==, and ~=, and compares releases
//     padded with zeros (==1.2 matches 1.2.0). ~=X.Y keeps every component
//     but the last (~=1.4 is >=1.4, ==1.*) and needs at least two.
//   - npm has >=, >, <=, <, =, none (an exact version), ^, and ~, and reads a
//     partial version as a range over its missing components (1.2 is 1.2.x,
//     >1.2 is >=1.3.0), so the version is compared on as many components as
//     the constraint gives. Caret keeps the left-most non-zero component (all
//     given components when they are zero: ^0.0.3 is exactly 0.0.3); tilde
//     keeps the major and minor when a minor is given, the major otherwise.
//
// Several conditions (a comma, "||", or a space between parts), "!=",
// wildcards, non-numeric versions, and an operator the ecosystem does not
// have are not evaluated.
func evaluateConstraint(lang vocab.Lang, constraint, version string) constraintVerdict {
	current, ok := parseVersionTuple(version)
	if !ok {
		return constraintUnevaluated
	}
	c := strings.TrimSpace(constraint)
	if c == "" || strings.Contains(c, ",") || strings.Contains(c, "||") || strings.HasPrefix(c, "!=") {
		return constraintUnevaluated
	}
	op := ""
	for _, candidate := range []string{">=", "<=", "==", "~=", ">", "<", "^", "~", "="} {
		if strings.HasPrefix(c, candidate) {
			op = candidate
			c = strings.TrimSpace(c[len(candidate):])
			break
		}
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
	switch lang {
	case vocab.LangPy:
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
		case "~=":
			if len(want) < 2 {
				return constraintUnevaluated
			}
			return verdict(cmp >= 0 && samePrefix(current, want, len(want)-1))
		}
		return constraintUnevaluated
	case vocab.LangTS:
		// The version compared on the components the constraint gives.
		cmp := compareTuples(truncate(current, len(want)), want)
		switch op {
		case ">=":
			return verdict(cmp >= 0)
		case ">":
			return verdict(cmp > 0)
		case "<=":
			return verdict(cmp <= 0)
		case "<":
			return verdict(cmp < 0)
		case "=", "":
			return verdict(cmp == 0)
		case "^":
			keep := len(want)
			for i, n := range want {
				if n != 0 {
					keep = i + 1
					break
				}
			}
			return verdict(compareTuples(current, want) >= 0 && samePrefix(current, want, keep))
		case "~":
			keep := 1
			if len(want) >= 2 {
				keep = 2
			}
			return verdict(compareTuples(current, want) >= 0 && samePrefix(current, want, keep))
		}
		return constraintUnevaluated
	}
	return constraintUnevaluated
}

// samePrefix reports whether a and b agree on their first n components, a
// missing component counting as zero.
func samePrefix(a, b []int, n int) bool {
	for i := 0; i < n; i++ {
		if component(a, i) != component(b, i) {
			return false
		}
	}
	return true
}

// truncate is the first n components of v, padded with zeros.
func truncate(v []int, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = component(v, i)
	}
	return out
}

func component(v []int, i int) int {
	if i < len(v) {
		return v[i]
	}
	return 0
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

// compareTuples compares version tuples component by component, the shorter
// padded with zeros (1.2 equals 1.2.0).
func compareTuples(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		x, y := component(a, i), component(b, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
