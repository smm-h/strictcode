package spec_test

import (
	"testing"

	"github.com/smm-h/strictcode/internal/spec/configspec"
	"github.com/smm-h/strictspec/go/strictspec"
)

// validConfig exercises every configuration surface the pinned decisions
// define: analysis modes, group toggles, rule toggles/severities, and all
// four suppression shapes.
const validConfig = `
format_version = 1

[analysis]
python_call_resolution = "type-checker"
python_type_checker = "pyright"

[groups.library]
enabled = true
severity = "error"

[rules.deps-unused]
enabled = true

[[rules.deps-unused.suppressions]]
project = "core"
dep = "transport"
reason = "loaded dynamically via the plugin registry"

[rules.dead-modules]
severity = "error"

[[rules.dead-modules.suppressions]]
path = "src/legacy_shim.py"
reason = "kept until the migration window closes"

[[rules.import-cycles.suppressions]]
modules = ["pkg.a", "pkg.b"]
reason = "known cycle, scheduled for the split in the next refactor"

[[rules.dead-workspace-packages.suppressions]]
member = "experimental"
reason = "incubating package, not yet consumed"
`

func TestValidConfigBinds(t *testing.T) {
	cfg, diags := configspec.ValidateBytes([]byte(validConfig), "toml")
	if len(diags) != 0 {
		t.Fatalf("valid config has diagnostics: %v", diags)
	}
	if cfg == nil {
		t.Fatal("nil binding for valid config")
	}
	if cfg.Analysis == nil {
		t.Fatal("analysis section not bound")
	}
	if cfg.Analysis.PythonCallResolution != "type-checker" || cfg.Analysis.PythonTypeChecker != "pyright" {
		t.Fatalf("analysis binding wrong: %+v", cfg.Analysis)
	}
	if len(cfg.Rules.Entries()) != 4 {
		t.Fatalf("bound %d rule entries, want 4", len(cfg.Rules.Entries()))
	}
}

func TestInvalidConfigsAreRejected(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{
			"suppression-without-reason",
			"format_version = 1\n[[rules.dead-modules.suppressions]]\npath = \"src/x.py\"\n",
		},
		{
			"suppression-empty-reason",
			"format_version = 1\n[[rules.dead-modules.suppressions]]\npath = \"src/x.py\"\nreason = \"\"\n",
		},
		{
			"suppression-no-shape",
			"format_version = 1\n[[rules.dead-modules.suppressions]]\nreason = \"why\"\n",
		},
		{
			"suppression-two-shapes",
			"format_version = 1\n[[rules.dead-modules.suppressions]]\npath = \"src/x.py\"\nmember = \"m\"\nreason = \"why\"\n",
		},
		{
			"project-without-dep",
			"format_version = 1\n[[rules.deps-unused.suppressions]]\nproject = \"core\"\npath = \"x\"\nreason = \"why\"\n",
		},
		{
			"single-module-cycle",
			"format_version = 1\n[[rules.import-cycles.suppressions]]\nmodules = [\"only-one\"]\nreason = \"why\"\n",
		},
		{
			"type-checker-mode-without-checker",
			"format_version = 1\n[analysis]\npython_call_resolution = \"type-checker\"\n",
		},
		{
			"checker-with-syntactic-mode",
			"format_version = 1\n[analysis]\npython_call_resolution = \"syntactic\"\npython_type_checker = \"ty\"\n",
		},
		{
			"empty-group-toggle",
			"format_version = 1\n[groups.library]\n",
		},
		{
			"unknown-key",
			"format_version = 1\nsurprise = true\n",
		},
		{
			"bad-severity",
			"format_version = 1\n[rules.deps-unused]\nseverity = \"fatal\"\n",
		},
		{
			"bad-rule-key-shape",
			"format_version = 1\n[rules.NotARule]\nenabled = false\n",
		},
		{
			"non-integer-threshold",
			"format_version = 1\n[rules.deps-unused.thresholds]\nmax = \"five\"\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, diags := configspec.ValidateBytes([]byte(c.doc), "toml")
			if cfg != nil || len(diags) == 0 {
				t.Fatalf("invalid config accepted (diags: %v)", diags)
			}
		})
	}
}

func TestConfigSuppressionShapesBind(t *testing.T) {
	cfg, diags := configspec.ValidateBytes([]byte(validConfig), "toml")
	if cfg == nil {
		t.Fatalf("valid config rejected: %v", diags)
	}
	shapes := map[string]string{}
	for _, kv := range cfg.Rules.Entries() {
		sup, ok := kv.Value.Field("suppressions")
		if !ok {
			continue
		}
		for _, item := range sup.Items() {
			switch {
			case fieldPresent(item, "path"):
				shapes[kv.Key] = "path"
			case fieldPresent(item, "dep"):
				shapes[kv.Key] = "project-dep"
			case fieldPresent(item, "modules"):
				shapes[kv.Key] = "member-set"
			case fieldPresent(item, "member"):
				shapes[kv.Key] = "member"
			}
		}
	}
	want := map[string]string{
		"deps-unused":             "project-dep",
		"dead-modules":            "path",
		"import-cycles":           "member-set",
		"dead-workspace-packages": "member",
	}
	for rule, shape := range want {
		if shapes[rule] != shape {
			t.Errorf("rule %s bound shape %q, want %q", rule, shapes[rule], shape)
		}
	}
}

func fieldPresent(v strictspec.Value, name string) bool {
	_, ok := v.Field(name)
	return ok
}
