// Package config loads strictcode.toml through the strictspec-generated
// reader and implements the consumer-native checks the schema cannot
// express (DESIGN.md section 12.3):
//
//   - rule-ID validity against the registry, with tombstone rendering — a
//     config referencing a retired ID hard-errors with the tombstone's
//     retired_in, reason, replaced_by successors, and migration hint;
//   - group-name validity;
//   - suppression-shape-vs-rule matching (a rule accepts only its declared
//     natural target shape; shape "none" accepts no suppressions at all).
//
// Disk/registry staleness of suppression targets is NOT a load error — it is
// the stale-suppression rule (CATALOG.md), evaluated during analysis with
// the workspace in hand.
//
// A missing config file yields the registry defaults: every rule enabled at
// its default severity, no suppressions, syntactic-only analysis. A present
// but malformed config is a hard error (lesson 31): nothing is coerced,
// defaulted, or skipped.
package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/spec/configspec"
	"github.com/smm-h/strictspec/go/strictspec"
)

// Suppression is one configured suppression in its rule's natural shape.
type Suppression struct {
	Rule   string
	Shape  rules.SuppressionShape
	Reason string

	Path    string   // shape path
	Project string   // shape project-dep
	Dep     string   // shape project-dep
	Modules []string // shape member-set
	Member  string   // shape member
}

// RuleSetting is the effective per-rule configuration after group toggles
// and per-rule overrides.
type RuleSetting struct {
	Enabled      bool
	Severity     rules.Severity
	Thresholds   map[string]int64
	Suppressions []Suppression
}

// Analysis is the effective analysis-mode selection.
type Analysis struct {
	// PythonTypeChecker is empty for the always-on syntactic layer, or the
	// chosen checker ("pyright" | "ty") when type-checker mode is selected.
	PythonTypeChecker string
}

// Effective is the fully resolved configuration.
type Effective struct {
	Analysis Analysis
	// Rules has an entry for every live registry rule.
	Rules map[string]RuleSetting
}

// Setting returns the effective setting for a live rule ID. Panics on an
// unknown ID: callers pass registry IDs, and an unknown one is a bug.
func (e *Effective) Setting(id string) RuleSetting {
	s, ok := e.Rules[id]
	if !ok {
		panic("config: no setting for rule " + id)
	}
	return s
}

// AllSuppressions returns every configured suppression (input to the
// stale-suppression rule), sorted by rule ID.
func (e *Effective) AllSuppressions() []Suppression {
	var out []Suppression
	ids := make([]string, 0, len(e.Rules))
	for id := range e.Rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, e.Rules[id].Suppressions...)
	}
	return out
}

// Load reads the config file at path. A missing file returns the defaults;
// any other read, parse, schema, or registry failure is a hard error.
func Load(path string) (*Effective, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Defaults(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return Parse(raw, path)
}

// Defaults returns the registry-default configuration.
func Defaults() *Effective {
	eff := &Effective{Rules: map[string]RuleSetting{}}
	for _, r := range rules.Rules {
		eff.Rules[r.ID] = RuleSetting{Enabled: true, Severity: r.Severity}
	}
	return eff
}

// Parse validates and resolves a config document. name is used in errors.
func Parse(raw []byte, name string) (*Effective, error) {
	doc, diags := configspec.ValidateBytes(raw, "toml")
	if doc == nil {
		return nil, fmt.Errorf("config: %s is invalid:\n%s", name, renderDiags(diags))
	}

	eff := Defaults()

	if doc.Analysis != nil {
		if doc.Analysis.PythonCallResolution == "type-checker" {
			eff.Analysis.PythonTypeChecker = doc.Analysis.PythonTypeChecker
		}
	}

	// Group toggles first (per-rule settings override them).
	for _, kv := range doc.Groups.Entries() {
		members, ok := rules.Groups[kv.Key]
		if !ok {
			return nil, fmt.Errorf("config: %s: unknown group %q (known groups: %s)",
				name, kv.Key, strings.Join(groupNames(), ", "))
		}
		enabled, hasEnabled := fieldBool(kv.Value, "enabled")
		severity, hasSeverity := fieldString(kv.Value, "severity")
		for _, id := range members {
			s := eff.Rules[id]
			if hasEnabled {
				s.Enabled = enabled
			}
			if hasSeverity {
				s.Severity = rules.Severity(severity)
			}
			eff.Rules[id] = s
		}
	}

	// Per-rule settings.
	for _, kv := range doc.Rules.Entries() {
		rule, live := rules.ByID(kv.Key)
		if !live {
			if tomb, retired := rules.TombstoneByID(kv.Key); retired {
				return nil, fmt.Errorf("config: %s: rule %q was retired in %s: %s; use %s; %s",
					name, kv.Key, tomb.RetiredIn, tomb.Reason,
					renderSuccessors(tomb.ReplacedBy), tomb.Migration)
			}
			return nil, fmt.Errorf("config: %s: unknown rule %q", name, kv.Key)
		}
		s := eff.Rules[rule.ID]
		if enabled, ok := fieldBool(kv.Value, "enabled"); ok {
			s.Enabled = enabled
		}
		if severity, ok := fieldString(kv.Value, "severity"); ok {
			s.Severity = rules.Severity(severity)
		}
		if thresholds, ok := kv.Value.Field("thresholds"); ok {
			s.Thresholds = map[string]int64{}
			for _, tkv := range thresholds.Entries() {
				n, _ := tkv.Value.Int()
				s.Thresholds[tkv.Key] = n
			}
		}
		if sups, ok := kv.Value.Field("suppressions"); ok {
			for i, item := range sups.Items() {
				sup, err := bindSuppression(rule, item)
				if err != nil {
					return nil, fmt.Errorf("config: %s: rules.%s.suppressions[%d]: %w", name, rule.ID, i, err)
				}
				s.Suppressions = append(s.Suppressions, sup)
			}
		}
		eff.Rules[rule.ID] = s
	}
	return eff, nil
}

// bindSuppression converts one schema-valid suppression entry and enforces
// shape-vs-rule matching against the registry.
func bindSuppression(rule rules.Rule, v strictspec.Value) (Suppression, error) {
	sup := Suppression{Rule: rule.ID}
	sup.Reason, _ = fieldString(v, "reason")

	// The schema guarantees exactly one shape discriminant.
	switch {
	case has(v, "path"):
		sup.Shape = rules.SuppressPath
		sup.Path, _ = fieldString(v, "path")
	case has(v, "dep"):
		sup.Shape = rules.SuppressProjectDep
		sup.Project, _ = fieldString(v, "project")
		sup.Dep, _ = fieldString(v, "dep")
	case has(v, "modules"):
		sup.Shape = rules.SuppressMemberSet
		mods, _ := v.Field("modules")
		for _, m := range mods.Items() {
			s, _ := m.AsString()
			sup.Modules = append(sup.Modules, s)
		}
		sort.Strings(sup.Modules)
	case has(v, "member"):
		sup.Shape = rules.SuppressMember
		sup.Member, _ = fieldString(v, "member")
	default:
		return sup, fmt.Errorf("no suppression shape (schema should have rejected this)")
	}

	if rule.Suppression == rules.SuppressNone {
		return sup, fmt.Errorf("rule %q accepts no suppressions", rule.ID)
	}
	if sup.Shape != rule.Suppression {
		return sup, fmt.Errorf("rule %q takes %s-shaped suppressions, got %s",
			rule.ID, rule.Suppression, sup.Shape)
	}
	return sup, nil
}

func renderSuccessors(ids []string) string {
	if len(ids) == 0 {
		return "no successor"
	}
	return strings.Join(ids, ", ")
}

func renderDiags(diags []strictspec.Diagnostic) string {
	var lines []string
	for _, d := range diags {
		lines = append(lines, fmt.Sprintf("  %s at %s: %s", d.Code, d.Path, d.Message))
	}
	return strings.Join(lines, "\n")
}

func groupNames() []string {
	var names []string
	for name := range rules.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func fieldString(v strictspec.Value, name string) (string, bool) {
	f, ok := v.Field(name)
	if !ok {
		return "", false
	}
	return f.AsString()
}

func fieldBool(v strictspec.Value, name string) (bool, bool) {
	f, ok := v.Field(name)
	if !ok {
		return false, false
	}
	return f.Bool()
}

func has(v strictspec.Value, name string) bool {
	_, ok := v.Field(name)
	return ok
}
