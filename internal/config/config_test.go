package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
	"github.com/smm-h/strictcode/internal/rules"
)

func TestMissingFileYieldsDefaults(t *testing.T) {
	root := fixture.Write(t, map[string]string{})
	eff, err := Load(filepath.Join(root, "strictcode.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(eff.Rules) != len(rules.Rules) {
		t.Fatalf("defaults cover %d rules, registry has %d", len(eff.Rules), len(rules.Rules))
	}
	for _, r := range rules.Rules {
		s := eff.Setting(r.ID)
		if !s.Enabled || s.Severity != r.Severity || len(s.Suppressions) != 0 {
			t.Errorf("%s default = %+v, want enabled at %s", r.ID, s, r.Severity)
		}
	}
	if eff.Analysis.PythonTypeChecker != "" {
		t.Fatal("default analysis must be syntactic-only")
	}
}

func TestRuleAndGroupResolution(t *testing.T) {
	eff, err := Parse([]byte(`
format_version = 1

[groups.library]
enabled = false

[rules.library-stdout]
enabled = true
severity = "warning"

[rules.deps-unused]
severity = "warning"

[[rules.dead-modules.suppressions]]
path = "src/keep.py"
reason = "referenced from templated imports"
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	// Group toggle disables all four library rules...
	if eff.Setting("library-forbidden-imports").Enabled || eff.Setting("library-entry-point").Enabled {
		t.Fatal("group:library disable not applied")
	}
	// ...but the per-rule override wins for library-stdout.
	if s := eff.Setting("library-stdout"); !s.Enabled || s.Severity != rules.SeverityWarning {
		t.Fatalf("per-rule override lost to group toggle: %+v", s)
	}
	if s := eff.Setting("deps-unused"); s.Severity != rules.SeverityWarning || !s.Enabled {
		t.Fatalf("deps-unused: %+v", s)
	}
	sups := eff.Setting("dead-modules").Suppressions
	if len(sups) != 1 || sups[0].Shape != rules.SuppressPath || sups[0].Path != "src/keep.py" {
		t.Fatalf("suppressions: %+v", sups)
	}
	if sups[0].Reason == "" {
		t.Fatal("reason lost")
	}
}

func TestAnalysisModes(t *testing.T) {
	eff, err := Parse([]byte("format_version = 1\n[analysis]\npython_call_resolution = \"type-checker\"\npython_type_checker = \"ty\"\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if eff.Analysis.PythonTypeChecker != "ty" {
		t.Fatalf("analysis: %+v", eff.Analysis)
	}
	eff, err = Parse([]byte("format_version = 1\n[analysis]\npython_call_resolution = \"syntactic\"\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if eff.Analysis.PythonTypeChecker != "" {
		t.Fatal("syntactic mode must not set a checker")
	}
}

func TestUnknownRuleIsHardError(t *testing.T) {
	_, err := Parse([]byte("format_version = 1\n[rules.no-such-rule]\nenabled = false\n"), "test")
	if err == nil || !strings.Contains(err.Error(), `unknown rule "no-such-rule"`) {
		t.Fatalf("got %v", err)
	}
}

func TestUnknownGroupIsHardError(t *testing.T) {
	_, err := Parse([]byte("format_version = 1\n[groups.nope]\nenabled = false\n"), "test")
	if err == nil || !strings.Contains(err.Error(), `unknown group "nope"`) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "library") {
		t.Fatalf("error must list known groups: %v", err)
	}
}

func TestTombstoneRendering(t *testing.T) {
	// Inject a tombstone; restore after. The registry has none yet, but the
	// lifecycle demands the rendering exist before the first tombstone does.
	saved := rules.Tombstones
	rules.Tombstones = []rules.Tombstone{{
		ID:         "old-rule",
		RetiredIn:  "0.9.0",
		Reason:     "the diagnosis split",
		ReplacedBy: []string{"deps-unused", "deps-hard-guarded-only"},
		Migration:  "move suppressions to the successor rules",
	}}
	defer func() { rules.Tombstones = saved }()

	_, err := Parse([]byte("format_version = 1\n[rules.old-rule]\nenabled = true\n"), "test")
	if err == nil {
		t.Fatal("tombstoned rule accepted")
	}
	msg := err.Error()
	for _, want := range []string{
		`rule "old-rule" was retired in 0.9.0`,
		"the diagnosis split",
		"deps-unused, deps-hard-guarded-only",
		"move suppressions to the successor rules",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("tombstone error missing %q:\n%s", want, msg)
		}
	}
}

func TestSuppressionShapeMismatchIsHardError(t *testing.T) {
	// dead-modules takes path-shaped suppressions; a (project, dep) pair is
	// a shape mismatch even though it is schema-valid in isolation.
	_, err := Parse([]byte(`
format_version = 1
[[rules.dead-modules.suppressions]]
project = "core"
dep = "transport"
reason = "wrong shape"
`), "test")
	if err == nil || !strings.Contains(err.Error(), "takes path-shaped suppressions") {
		t.Fatalf("got %v", err)
	}
}

func TestSuppressNoneRejectsAllSuppressions(t *testing.T) {
	_, err := Parse([]byte(`
format_version = 1
[[rules.stale-suppression.suppressions]]
path = "x"
reason = "meta"
`), "test")
	if err == nil || !strings.Contains(err.Error(), "accepts no suppressions") {
		t.Fatalf("got %v", err)
	}
}

func TestMalformedConfigIsHardError(t *testing.T) {
	// Lesson 31: wrong types, unknown keys — fail loudly, never coerce.
	cases := []string{
		"format_version = 1\nunknown_key = 1\n",
		"format_version = 1\n[rules.deps-unused]\nenabled = \"yes\"\n",
		"not toml at all [",
	}
	for _, doc := range cases {
		if _, err := Parse([]byte(doc), "test"); err == nil {
			t.Errorf("malformed config accepted: %q", doc)
		}
	}
}

func TestAllSuppressionsSortedByRule(t *testing.T) {
	eff, err := Parse([]byte(`
format_version = 1
[[rules.import-cycles.suppressions]]
modules = ["b", "a"]
reason = "cycle"

[[rules.dead-modules.suppressions]]
path = "x.py"
reason = "keep"
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	all := eff.AllSuppressions()
	if len(all) != 2 || all[0].Rule != "dead-modules" || all[1].Rule != "import-cycles" {
		t.Fatalf("AllSuppressions: %+v", all)
	}
	if all[1].Modules[0] != "a" || all[1].Modules[1] != "b" {
		t.Fatal("module sets must be sorted for deterministic matching")
	}
}
