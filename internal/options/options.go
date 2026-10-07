// Package options is strictcode's options registry and the reader of a
// repository's strictcode:<rule id> entries (~/Projects/CONTEXT/strict.md:
// options are the only sanctioned way to change how a family tool behaves).
//
// Every rule is an option. A rule declared at error severity ranks
// "error > warn > off", one declared at warning severity "warn > off", and the
// option's default is the rule's severity, except for an adopted rule
// (lint, format, type-check, strictspec-certificate), whose option ranks
// "error > warn > off" and defaults to off. An adopted rule that runs per
// member takes a path scope naming one workspace member's directory; every
// other option takes no scope.
//
// The registry is built from the rule registry (internal/rules), which is
// its one source, and is validated through strictspec's options rules; a
// repository's entries are read from .strictmetadata/options/ and judged by
// strictspec's namespace validator, so a refused entry carries strictspec's
// catalogued diagnostic.
package options

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/stricttools/strictspec/go/strictspec"
)

// Namespace is the tool name strictcode's option IDs are prefixed with.
const Namespace = "strictcode"

// Value is an option value.
type Value string

// The values strictcode's options rank.
const (
	Error Value = "error"
	Warn  Value = "warn"
	Off   Value = "off"
)

// scopeNone and scopePath are the registry scope forms strictcode's options
// use.
const (
	scopeNone = "none"
	scopePath = "path"
)

// ID is the option ID of a rule: strictcode:<rule id>.
func ID(ruleID string) string { return Namespace + ":" + ruleID }

// EntryFile is the repository-relative subject document that holds ruleID's
// option entries, the file to edit to change the option's value.
func EntryFile(ruleID string) string {
	r, ok := rules.ByID(ruleID)
	if !ok {
		panic("options: no rule " + ruleID)
	}
	return strictspec.OptionsDir + "/" + r.OptionSubject + ".toml"
}

// SwitchOff is the instruction that switches ruleID's option off, for use at
// the end of a refusal.
func SwitchOff(ruleID string) string {
	return fmt.Sprintf("switch %s off: set current = \"off\" in its entry in %s (the entry scoped to the member path, or the unscoped entry)", ID(ruleID), EntryFile(ruleID))
}

// Ranking is the ranking string of a rule's option.
func Ranking(r rules.Rule) string {
	if r.Severity == rules.SeverityWarning && !r.Adoption {
		return "warn > off"
	}
	return "error > warn > off"
}

// Default is the default value of a rule's option.
func Default(r rules.Rule) Value {
	switch {
	case r.Adoption:
		return Off
	case r.Severity == rules.SeverityWarning:
		return Warn
	default:
		return Error
	}
}

// Scope is the scope form of a rule's option.
func Scope(r rules.Rule) string {
	if r.PathScoped {
		return scopePath
	}
	return scopeNone
}

// Severity is the severity of the findings a rule reports at value v. Off
// has none: a rule whose value is off does not run.
func Severity(v Value) rules.Severity {
	if v == Warn {
		return rules.SeverityWarning
	}
	return rules.SeverityError
}

// Registry is strictcode's options registry: one option per rule, in the
// rule registry's order.
func Registry() *strictspec.OptionsRegistry {
	reg := &strictspec.OptionsRegistry{Options: []strictspec.OptionDeclaration{}}
	for _, r := range rules.Rules {
		reg.Options = append(reg.Options, strictspec.OptionDeclaration{
			Name:        r.ID,
			Subject:     r.OptionSubject,
			Values:      Ranking(r),
			Default:     string(Default(r)),
			Scope:       Scope(r),
			Requires:    []string{},
			Description: r.Description,
		})
	}
	return reg
}

// Checked is the registry after strictspec's registry rules. A registry that
// fails them is a strictcode defect, so it panics; a test holds it.
func Checked() *strictspec.CheckedOptionsRegistry {
	checked, diags := strictspec.ValidateOptionsRegistry(Registry())
	if len(diags) != 0 {
		panic("options: strictcode's options registry fails strictspec's registry rules:\n" + renderDiagnostics(diags))
	}
	return checked
}

// Resolved holds the value of every rule's option in one repository.
type Resolved struct {
	// repository is each rule's unscoped value: its unscoped entry's current
	// value, or its default.
	repository map[string]Value
	// scoped is each path-scoped rule's values by member path, from its
	// scoped entries.
	scoped map[string]map[string]Value
}

// Defaults resolves every option to its default, as a repository without
// entries does.
func Defaults() *Resolved {
	res := &Resolved{repository: map[string]Value{}, scoped: map[string]map[string]Value{}}
	for _, r := range rules.Rules {
		res.repository[r.ID] = Default(r)
	}
	return res
}

// Value is a rule's repository-wide value. Panics on an unknown rule ID:
// callers pass registry IDs.
func (r *Resolved) Value(ruleID string) Value {
	v, ok := r.repository[ruleID]
	if !ok {
		panic("options: no value for rule " + ruleID)
	}
	return v
}

// ValueFor is a path-scoped rule's value for the member at memberPath: its
// entry scoped to that path, else the rule's repository-wide value.
func (r *Resolved) ValueFor(ruleID, memberPath string) Value {
	if v, ok := r.scoped[ruleID][memberPath]; ok {
		return v
	}
	return r.Value(ruleID)
}

// Runs reports whether a rule runs anywhere: its repository-wide value, or
// any value scoped to a member, is not off.
func (r *Resolved) Runs(ruleID string) bool {
	if r.Value(ruleID) != Off {
		return true
	}
	for _, v := range r.scoped[ruleID] {
		if v != Off {
			return true
		}
	}
	return false
}

// OnPaths lists the member paths at which a path-scoped rule is not off,
// sorted, from memberPaths (every member path of the workspace).
func (r *Resolved) OnPaths(ruleID string, memberPaths []string) []string {
	var out []string
	for _, p := range memberPaths {
		if r.ValueFor(ruleID, p) != Off {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// setScoped records a path-scoped rule's value for one member path.
func (r *Resolved) setScoped(ruleID, scope string, v Value) {
	if r.scoped[ruleID] == nil {
		r.scoped[ruleID] = map[string]Value{}
	}
	r.scoped[ruleID][scope] = v
}

// Load reads repoRoot's options entries and resolves strictcode's namespace.
// memberPaths are the workspace's member paths, the only scopes a path-scoped
// option accepts. A document that fails its shape, an entry strictspec's
// namespace validator refuses, and a scope that names no member are hard
// errors, each reported.
func Load(repoRoot string, memberPaths []string) (*Resolved, error) {
	load, err := strictspec.LoadOptionsEntries(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("options: %w", err)
	}
	var problems []string
	for _, inv := range load.Invalid {
		problems = append(problems, fmt.Sprintf("%s/%s is not a valid options document:\n%s",
			strictspec.OptionsDir, inv.File, renderDiagnostics(inv.Diagnostics)))
	}
	accepted, diags := strictspec.ValidateOptionsNamespace(Namespace, Checked(), load.Entries)
	if len(diags) != 0 {
		problems = append(problems, renderDiagnostics(diags))
	}
	known := map[string]bool{}
	for _, p := range memberPaths {
		known[p] = true
	}
	res := Defaults()
	for _, c := range accepted {
		e := c.Entry
		ruleID := strings.TrimPrefix(e.ID, Namespace+":")
		v := Value(e.Current)
		if !e.HasScope {
			res.repository[ruleID] = v
			continue
		}
		if !known[e.Scope] {
			problems = append(problems, fmt.Sprintf(
				"%s/%s: the entry for %s is scoped to %q, which is no workspace member's path; a scope names one of: %s",
				strictspec.OptionsDir, e.File, e.ID, e.Scope, strings.Join(sortedCopy(memberPaths), ", ")))
			continue
		}
		res.setScoped(ruleID, e.Scope, v)
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("options: strictcode's options entries are refused:\n%s", strings.Join(problems, "\n"))
	}
	return res, nil
}

func renderDiagnostics(diags []strictspec.Diagnostic) string {
	lines := make([]string, 0, len(diags))
	for _, d := range diags {
		lines = append(lines, fmt.Sprintf("  %s at %s: %s", d.Code, d.Path, d.Message))
	}
	return strings.Join(lines, "\n")
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
