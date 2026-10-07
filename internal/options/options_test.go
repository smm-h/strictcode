package options

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
	"github.com/smm-h/strictcode/internal/rules"
)

func TestRegistryPassesStrictspecRegistryRules(t *testing.T) {
	checked := Checked()
	if got := len(checked.Names()); got != len(rules.Rules) {
		t.Fatalf("registry declares %d options, the rule registry %d rules", got, len(rules.Rules))
	}
}

func TestEveryRuleIsAnOptionDefaultingToItsSeverity(t *testing.T) {
	checked := Checked()
	for _, r := range rules.Rules {
		opt, ok := checked.Option(r.ID)
		if !ok {
			t.Errorf("%s has no option", r.ID)
			continue
		}
		d := opt.Declaration
		switch {
		case r.Adoption:
			if d.Default != "off" || d.Values != "error > warn > off" {
				t.Errorf("%s: adopted rule's option is %q defaulting to %q, want error > warn > off defaulting to off", r.ID, d.Values, d.Default)
			}
		case r.Severity == rules.SeverityError:
			if d.Default != "error" || d.Values != "error > warn > off" {
				t.Errorf("%s: error rule's option is %q defaulting to %q", r.ID, d.Values, d.Default)
			}
		default:
			if d.Default != "warn" || d.Values != "warn > off" {
				t.Errorf("%s: warning rule's option is %q defaulting to %q", r.ID, d.Values, d.Default)
			}
		}
		wantScope := "none"
		if r.PathScoped {
			wantScope = "path"
		}
		if d.Scope != wantScope {
			t.Errorf("%s: scope %q, want %q", r.ID, d.Scope, wantScope)
		}
	}
}

func TestToolAndCertificateOptionsDefaultToOff(t *testing.T) {
	res := Defaults()
	for _, id := range []string{"lint", "format", "type-check", "strictspec-certificate"} {
		if v := res.Value(id); v != Off {
			t.Errorf("strictcode:%s defaults to %q, want off", id, v)
		}
		if res.Runs(id) {
			t.Errorf("strictcode:%s runs without an entry", id)
		}
	}
	for _, id := range []string{"lint", "format", "type-check"} {
		r, _ := rules.ByID(id)
		if Scope(r) != "path" {
			t.Errorf("strictcode:%s takes scope %q, want path", id, Scope(r))
		}
	}
}

const optionsManifest = "owner = \"strictspec\"\n"

func TestLoadAppliesEntries(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		".strictmetadata/options/manifest.toml": optionsManifest,
		".strictmetadata/options/code.toml": `format_version = 1

[[entry]]
id = "strictcode:dead-modules"
current = "off"
ideal = "warn"
reason = "the generated sources are not walked yet"

[[entry]]
id = "strictcode:lint"
scope = "core"
current = "error"
ideal = "error"
reason = "core adopts ruff"

[[entry]]
id = "rlsbl:scaffold-unreplaced-vars"
current = "off"
ideal = "error"
reason = "another tool's namespace, which strictcode does not judge"
`,
		".strictmetadata/options/dependencies.toml": `format_version = 1

[[entry]]
id = "strictcode:deps-unused"
current = "warn"
ideal = "error"
reason = "the workspace is mid-migration"
`,
	})
	res, err := Load(root, []string{".", "core"})
	if err != nil {
		t.Fatal(err)
	}
	if v := res.Value("dead-modules"); v != Off {
		t.Errorf("dead-modules = %q, want off", v)
	}
	if v := res.Value("deps-unused"); v != Warn {
		t.Errorf("deps-unused = %q, want warn", v)
	}
	if Severity(res.Value("deps-unused")) != rules.SeverityWarning {
		t.Error("warn must report at warning severity")
	}
	if v := res.Value("deps-undeclared"); v != Error {
		t.Errorf("deps-undeclared = %q, want its default error", v)
	}
	if v := res.ValueFor("lint", "core"); v != Error {
		t.Errorf("lint at core = %q, want error", v)
	}
	if v := res.ValueFor("lint", "."); v != Off {
		t.Errorf("lint at the root = %q, want off", v)
	}
	if got := res.OnPaths("lint", []string{".", "core"}); len(got) != 1 || got[0] != "core" {
		t.Errorf("lint runs at %v, want [core]", got)
	}
}

func TestLoadRefusesEntries(t *testing.T) {
	cases := map[string]struct {
		file, body, want string
	}{
		"unknown option": {"code.toml", `[[entry]]
id = "strictcode:no-such-rule"
current = "off"
ideal = "off"
reason = "r"
`, "STRICTSPEC_OPTIONS_UNKNOWN_OPTION"},
		"undeclared value": {"code.toml", `[[entry]]
id = "strictcode:dead-modules"
current = "error"
ideal = "error"
reason = "r"
`, "STRICTSPEC_OPTIONS_UNDECLARED_CURRENT"},
		"wrong subject": {"code.toml", `[[entry]]
id = "strictcode:deps-unused"
current = "off"
ideal = "error"
reason = "r"
`, "STRICTSPEC_OPTIONS_WRONG_SUBJECT"},
		"scope on an unscoped option": {"code.toml", `[[entry]]
id = "strictcode:dead-modules"
scope = "core"
current = "off"
ideal = "warn"
reason = "r"
`, "STRICTSPEC_OPTIONS_SCOPE_NOT_ACCEPTED"},
		"scope naming no member": {"code.toml", `[[entry]]
id = "strictcode:lint"
scope = "nowhere"
current = "error"
ideal = "error"
reason = "r"
`, "nowhere"},
		"entry equal to the default": {"code.toml", `[[entry]]
id = "strictcode:lint"
current = "off"
ideal = "off"
reason = "r"
`, "STRICTSPEC_OPTIONS_REDUNDANT"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := fixture.Write(t, map[string]string{
				".strictmetadata/options/manifest.toml": optionsManifest,
				".strictmetadata/options/" + c.file:     "format_version = 1\n\n" + c.body,
			})
			_, err := Load(root, []string{".", "core"})
			if err == nil {
				t.Fatal("entry accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("refusal does not name %q: %v", c.want, err)
			}
		})
	}
}

func TestLoadWithoutOptionsDirectoryIsTheDefaults(t *testing.T) {
	root := fixture.Write(t, map[string]string{"pyproject.toml": "[project]\nname = \"solo\"\n"})
	res, err := Load(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules.Rules {
		if res.Value(r.ID) != Default(r) {
			t.Errorf("%s = %q, want its default %q", r.ID, res.Value(r.ID), Default(r))
		}
	}
}
