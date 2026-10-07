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
		if s := eff.Setting(r.ID); len(s.Suppressions) != 0 {
			t.Errorf("%s default = %+v, want no suppressions", r.ID, s)
		}
	}
	if len(eff.PythonTools) != 0 || eff.Certificate != nil {
		t.Fatalf("defaults declare tools or a certificate: %+v", eff)
	}
	if eff.Analysis.PythonTypeChecker != "" {
		t.Fatal("default analysis must be syntactic-only")
	}
}

func TestRuleDeclarations(t *testing.T) {
	eff, err := Parse([]byte(`
format_version = 1

[rules.library-forbidden-imports.allow]
py = ["click"]

[[rules.dead-modules.suppressions]]
path = "src/keep.py"
reason = "referenced from templated imports"
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	if allow := eff.Setting("library-forbidden-imports").Allow["py"]; len(allow) != 1 || allow[0] != "click" {
		t.Fatalf("allow list: %+v", allow)
	}
	sups := eff.Setting("dead-modules").Suppressions
	if len(sups) != 1 || sups[0].Shape != rules.SuppressPath || sups[0].Path != "src/keep.py" {
		t.Fatalf("suppressions: %+v", sups)
	}
	if sups[0].Reason == "" {
		t.Fatal("reason lost")
	}
}

// The per-rule and per-group switches are gone: a rule's behavior changes
// only through its strictcode:<rule id> option, so strictcode.toml refuses
// every spelling of a switch.
func TestSwitchesAreRefused(t *testing.T) {
	for _, doc := range []string{
		"format_version = 1\n[rules.deps-unused]\nenabled = false\n",
		"format_version = 1\n[rules.deps-unused]\nseverity = \"warning\"\n",
		"format_version = 1\n[groups.library]\nenabled = false\n",
		"format_version = 1\n[groups.library]\nseverity = \"warning\"\n",
	} {
		if _, err := Parse([]byte(doc), "test"); err == nil {
			t.Errorf("switch accepted: %q", doc)
		}
	}
}

func TestPythonToolDeclarations(t *testing.T) {
	eff, err := Parse([]byte(`
format_version = 1

[python_tools.lint]
paths = ["core", "tools/gen"]

[python_tools.type-check]
cwd = "core"
paths = ["src", "tests"]
`), "test")
	if err != nil {
		t.Fatal(err)
	}
	lint, ok := eff.PythonTools["lint"]
	if !ok || lint.Cwd != "." || len(lint.Paths) != 2 {
		t.Fatalf("lint declaration: %+v", lint)
	}
	tc := eff.PythonTools["type-check"]
	if got := tc.RootPaths(); len(got) != 2 || got[0] != "core/src" || got[1] != "core/tests" {
		t.Fatalf("type-check root paths: %v", got)
	}
	if _, ok := eff.PythonTools["format"]; ok {
		t.Fatal("an undeclared tool appeared")
	}
}

func TestPythonToolDeclarationsAreRefused(t *testing.T) {
	cases := map[string]string{
		"no paths":          "[python_tools.lint]\ncwd = \"core\"\n",
		"empty paths":       "[python_tools.lint]\npaths = []\n",
		"unknown tool":      "[python_tools.pyright]\npaths = [\"x\"]\n",
		"unknown key":       "[python_tools.lint]\npaths = [\"x\"]\nargs = [\"--fix\"]\n",
		"absolute path":     "[python_tools.lint]\npaths = [\"/x\"]\n",
		"climbing path":     "[python_tools.lint]\npaths = [\"../x\"]\n",
		"non-canonical cwd": "[python_tools.lint]\ncwd = \"core/\"\npaths = [\"x\"]\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte("format_version = 1\n"+body), "test"); err == nil {
				t.Fatalf("accepted: %q", body)
			}
		})
	}
}

func TestCertificateDeclaration(t *testing.T) {
	eff, err := Parse([]byte("format_version = 1\n[strictspec_certificate]\ncertificate = \"migrations/cert.json\"\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if eff.Certificate == nil || eff.Certificate.Certificate != "migrations/cert.json" || eff.Certificate.Adjudication != "" {
		t.Fatalf("certificate: %+v", eff.Certificate)
	}
	for _, body := range []string{
		"[strictspec_certificate]\nadjudication = \"a.toml\"\n",
		"[strictspec_certificate]\ncertificate = \"/abs/cert.json\"\n",
		"[strictspec_certificate]\ncertificate = \"c.json\"\nenabled = true\n",
	} {
		if _, err := Parse([]byte("format_version = 1\n"+body), "test"); err == nil {
			t.Errorf("accepted: %q", body)
		}
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
	_, err := Parse([]byte("format_version = 1\n[rules.no-such-rule.allow]\npy = [\"click\"]\n"), "test")
	if err == nil || !strings.Contains(err.Error(), `unknown rule "no-such-rule"`) {
		t.Fatalf("got %v", err)
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

	_, err := Parse([]byte("format_version = 1\n[[rules.old-rule.suppressions]]\npath = \"x\"\nreason = \"kept\"\n"), "test")
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
		"format_version = 1\n[rules.deps-unused]\nthresholds = \"yes\"\n",
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
