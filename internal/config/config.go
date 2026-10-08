// Package config loads strictcode.toml, the declarations file, through the
// strictspec-generated reader and implements the consumer-native checks the
// schema cannot express (.strictmetadata/docs/config.md):
//
//   - rule-ID validity against the registry, with tombstone rendering — a
//     config referencing a retired ID hard-errors with the tombstone's
//     retired_in, reason, replaced_by successors, and migration hint;
//   - suppression-shape-vs-rule matching (a rule accepts only its declared
//     natural target shape; shape "none" accepts no suppressions at all);
//   - canonical relative paths in the Python tool and certificate
//     declarations.
//
// The file only declares: suppressions with their reasons, allow lists, the
// Python tool declarations, and the strictspec certificate. It switches
// nothing; whether a rule runs, and at which severity, is the rule's
// strictcode:<rule id> option (internal/options).
//
// Disk/registry staleness of suppression targets is NOT a load error — it is
// the stale-suppression rule (.strictmetadata/docs/rules/stale-suppression.md), evaluated during analysis with
// the workspace in hand.
//
// A missing config file yields no declarations: no suppressions, no tool or
// certificate declarations, syntactic-only analysis. A present but malformed
// config is a hard error (lesson 31): nothing is coerced, defaulted, or
// skipped.
package config

import (
	"fmt"
	"os"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/spec/configspec"
	"github.com/stricttools/strictspec/go/strictspec"
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

// RuleSetting is one rule's declarations.
type RuleSetting struct {
	Thresholds   map[string]int64
	Suppressions []Suppression
	// Allow holds per-language allow lists (library-forbidden-imports:
	// subtracted from the effective forbidden set; lesson 26).
	Allow map[string][]string
	// Forbidden holds per-language replacements of the default
	// forbidden-imports list.
	Forbidden map[string][]string
}

// PythonTool is one [python_tools.<rule>] declaration: the tool runs in Cwd
// over Paths.
type PythonTool struct {
	// Cwd is the canonical workspace-root-relative directory the tool runs
	// in; "." when the declaration states none.
	Cwd string
	// Paths are canonical and relative to Cwd.
	Paths []string
}

// RootPaths are the declared paths relative to the workspace root.
func (p PythonTool) RootPaths() []string {
	out := make([]string, 0, len(p.Paths))
	for _, path := range p.Paths {
		out = append(out, joinRelative(p.Cwd, path))
	}
	return out
}

// Certificate is the [strictspec_certificate] declaration: workspace-root
// relative paths, Adjudication empty when none is declared.
type Certificate struct {
	Certificate  string
	Adjudication string
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
	// PythonTools holds the [python_tools.<rule>] declarations, keyed by
	// rule ID.
	PythonTools map[string]PythonTool
	// Certificate is the [strictspec_certificate] declaration, nil when the
	// file declares none.
	Certificate *Certificate
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

// Load reads the config file at path. A missing file returns no
// declarations; any other read, parse, schema, or registry failure is a hard
// error.
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

// Defaults returns the configuration of a repository without a config file.
func Defaults() *Effective {
	eff := &Effective{Rules: map[string]RuleSetting{}, PythonTools: map[string]PythonTool{}}
	for _, r := range rules.Rules {
		eff.Rules[r.ID] = RuleSetting{}
	}
	return eff
}

// Parse validates and resolves a config document. name is used in errors.
func Parse(raw []byte, name string) (*Effective, error) {
	if valid, diags := configspec.ValidateBytes(raw, "toml"); valid == nil {
		return nil, fmt.Errorf("config: %s is invalid:\n%s", name, renderDiags(diags))
	}
	// The generated reader validated the document; the fields are read from
	// the document itself, which keeps this loader independent of the typed
	// binding's shape.
	doc, err := strictspec.LoadValue(raw, "toml")
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", name, err)
	}

	eff := Defaults()

	if analysis, ok := doc.Field("analysis"); ok {
		if mode, _ := fieldString(analysis, "python_call_resolution"); mode == "type-checker" {
			eff.Analysis.PythonTypeChecker, _ = fieldString(analysis, "python_type_checker")
		}
	}

	if ruleTables, ok := doc.Field("rules"); ok {
		for _, kv := range ruleTables.Entries() {
			if err := parseRule(eff, kv, name); err != nil {
				return nil, err
			}
		}
	}

	if tools, ok := doc.Field("python_tools"); ok {
		for _, kv := range tools.Entries() {
			tool, err := parsePythonTool(kv.Value)
			if err != nil {
				return nil, fmt.Errorf("config: %s: python_tools.%s: %w", name, kv.Key, err)
			}
			eff.PythonTools[kv.Key] = tool
		}
	}

	if cert, ok := doc.Field("strictspec_certificate"); ok {
		c := &Certificate{}
		c.Certificate, _ = fieldString(cert, "certificate")
		c.Adjudication, _ = fieldString(cert, "adjudication")
		for field, path := range map[string]string{"certificate": c.Certificate, "adjudication": c.Adjudication} {
			if path == "" && field == "adjudication" {
				continue
			}
			if problem := pathProblem(path); problem != "" {
				return nil, fmt.Errorf("config: %s: strictspec_certificate.%s: %s", name, field, problem)
			}
		}
		eff.Certificate = c
	}
	return eff, nil
}

// parseRule reads one [rules.<id>] table.
func parseRule(eff *Effective, kv strictspec.KV, name string) error {
	rule, live := rules.ByID(kv.Key)
	if !live {
		if tomb, retired := rules.TombstoneByID(kv.Key); retired {
			return fmt.Errorf("config: %s: rule %q was retired in %s: %s; use %s; %s",
				name, kv.Key, tomb.RetiredIn, tomb.Reason,
				renderSuccessors(tomb.ReplacedBy), tomb.Migration)
		}
		return fmt.Errorf("config: %s: unknown rule %q", name, kv.Key)
	}
	s := eff.Rules[rule.ID]
	if thresholds, ok := kv.Value.Field("thresholds"); ok {
		s.Thresholds = map[string]int64{}
		for _, tkv := range thresholds.Entries() {
			n, _ := tkv.Value.Int()
			s.Thresholds[tkv.Key] = n
		}
	}
	for _, listField := range []string{"allow", "forbidden"} {
		lists, ok := kv.Value.Field(listField)
		if !ok {
			continue
		}
		m := map[string][]string{}
		for _, lkv := range lists.Entries() {
			var vals []string
			for _, item := range lkv.Value.Items() {
				v, _ := item.AsString()
				vals = append(vals, v)
			}
			m[lkv.Key] = vals
		}
		if listField == "allow" {
			s.Allow = m
		} else {
			s.Forbidden = m
		}
	}
	if sups, ok := kv.Value.Field("suppressions"); ok {
		for i, item := range sups.Items() {
			sup, err := bindSuppression(rule, item)
			if err != nil {
				return fmt.Errorf("config: %s: rules.%s.suppressions[%d]: %w", name, rule.ID, i, err)
			}
			s.Suppressions = append(s.Suppressions, sup)
		}
	}
	eff.Rules[rule.ID] = s
	return nil
}

// parsePythonTool reads one [python_tools.<rule>] table, refusing a path
// that is not canonical.
func parsePythonTool(v strictspec.Value) (PythonTool, error) {
	tool := PythonTool{Cwd: "."}
	if cwd, ok := fieldString(v, "cwd"); ok {
		if problem := pathProblem(cwd); problem != "" {
			return tool, fmt.Errorf("cwd: %s", problem)
		}
		tool.Cwd = cwd
	}
	paths, _ := v.Field("paths")
	for i, item := range paths.Items() {
		p, _ := item.AsString()
		if problem := pathProblem(p); problem != "" {
			return tool, fmt.Errorf("paths[%d]: %s", i, problem)
		}
		tool.Paths = append(tool.Paths, p)
	}
	return tool, nil
}

// pathProblem says why p is not a canonical relative path, and is empty
// when it is one: relative, '/'-separated, with no '.', '..', or empty
// segment, the directory itself written ".".
func pathProblem(p string) string {
	const rule = "a path here is relative, '/'-separated, with no '.', '..', or empty segment, and the directory itself is written \".\""
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.TrimSpace(p) != p {
		return fmt.Sprintf("%q is not canonical: %s", p, rule)
	}
	clean := pathpkg.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Sprintf("%q leaves the directory it is relative to: %s", p, rule)
	}
	if clean != p {
		return fmt.Sprintf("%q is not canonical: write it as %q", p, clean)
	}
	return ""
}

// joinRelative joins canonical relative paths, "." disappearing.
func joinRelative(dir, rel string) string {
	switch {
	case dir == ".":
		return rel
	case rel == ".":
		return dir
	default:
		return dir + "/" + rel
	}
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

func fieldString(v strictspec.Value, name string) (string, bool) {
	f, ok := v.Field(name)
	if !ok {
		return "", false
	}
	return f.AsString()
}

func has(v strictspec.Value, name string) bool {
	_, ok := v.Field(name)
	return ok
}
