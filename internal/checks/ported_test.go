package checks

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
)

// The cases of rlsbl's dependency and dead-code tests that the numbered
// lessons do not already pin, ported as regression tests.

// Every except clause catching ImportError or ModuleNotFoundError, alone,
// in a tuple, qualified, or bound with "as", guards the imports in its try
// body, at any depth of nesting under it; except Exception and a bare
// except do not.
func TestPortedGuardForms(t *testing.T) {
	cases := map[string]struct {
		source  string
		guarded bool
	}{
		"module not found":     {"try:\n    import b\nexcept ModuleNotFoundError:\n    b = None\n", true},
		"tuple":                {"try:\n    import b\nexcept (ImportError, AttributeError):\n    b = None\n", true},
		"as binding":           {"try:\n    import b\nexcept ImportError as exc:\n    b = None\n", true},
		"tuple with binding":   {"try:\n    import b\nexcept (ImportError, OSError) as exc:\n    b = None\n", true},
		"nested under a guard": {"try:\n    try:\n        import b\n    except KeyError:\n        pass\nexcept ImportError:\n    pass\n", true},
		"except Exception":     {"try:\n    import b\nexcept Exception:\n    b = None\n", false},
		"bare except":          {"try:\n    import b\nexcept:\n    b = None\n", false},
		"outside any try":      {"import b\n", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fs := analyze(t, twoMemberPy("", map[string]string{"a_pkg/mod.py": c.source}))
			undeclared := byRule(fs, "deps-undeclared")
			if c.guarded && len(undeclared) != 0 {
				t.Fatalf("a guarded import was reported undeclared: %+v", undeclared)
			}
			if !c.guarded && len(undeclared) != 1 {
				t.Fatalf("an unguarded import must be reported undeclared once: %+v", undeclared)
			}
		})
	}
}

func TestPortedDepsUnusedCases(t *testing.T) {
	// Every unused declaration is reported, one finding each.
	files := map[string]string{
		fixture.DeclarationsPath: fixture.Workspace("path = \"a\"\nname = \"a\"\n", "path = \"b\"\nname = \"b\"\n", "path = \"c\"\nname = \"c\"\n"),
		"a/pyproject.toml":       "[project]\nname = \"a\"\ndependencies = [\"b\", \"c\", \"requests\"]\n",
		"a/a_pkg/__init__.py":    "import requests\n",
		"b/pyproject.toml":       "[project]\nname = \"b\"\n",
		"b/b/__init__.py":        "",
		"c/pyproject.toml":       "[project]\nname = \"c\"\n",
		"c/c/__init__.py":        "",
	}
	unused := byRule(analyze(t, files), "deps-unused")
	if len(unused) != 2 || !messagesContain(unused, "'b'") || !messagesContain(unused, "'c'") {
		t.Fatalf("want b and c unused, and no finding for the external requests: %+v", unused)
	}

	// An import from test code marks the dependency used.
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{"tests/test_a.py": "import b\n"}))
	if got := byRule(fs, "deps-unused"); len(got) != 0 {
		t.Errorf("a test import must mark the dependency used: %+v", got)
	}

	// An optional dependency never imported is still unused.
	files = twoMemberPy("", nil)
	files["a/pyproject.toml"] = "[project]\nname = \"a\"\n\n[project.optional-dependencies]\nextra = [\"b\"]\n"
	if got := byRule(analyze(t, files), "deps-unused"); len(got) != 1 {
		t.Errorf("an optional dependency never imported must be unused: %+v", got)
	}

	// Imports of packages outside the workspace never count as undeclared.
	fs = analyze(t, twoMemberPy("", map[string]string{"a_pkg/mod.py": "import requests\nimport os\n"}))
	if got := byRule(fs, "deps-undeclared"); len(got) != 0 {
		t.Errorf("an external import was reported undeclared: %+v", got)
	}
}

// npmPairWorkspace is two npm members: sdk (a library, registry name
// @x/sdk) and app depending on it with the given specifier.
func npmPairWorkspace(spec, appSource string) map[string]string {
	return map[string]string{
		fixture.DeclarationsPath: fixture.Workspace("path = \"sdk\"\nname = \"sdk\"\nlibrary = true\n", "path = \"app\"\nname = \"app\"\n"),
		"sdk/package.json":       `{"name": "@x/sdk", "version": "1.0.0", "main": "./index.js"}`,
		"sdk/index.js":           "export const s = 1;\n",
		"app/package.json":       `{"name": "app", "main": "./index.js", "dependencies": {"@x/sdk": "` + spec + `"}}`,
		"app/index.js":           appSource,
	}
}

func TestPortedNpmWorkspaceDependencies(t *testing.T) {
	fs := analyze(t, npmPairWorkspace("workspace:*", "import { s } from '@x/sdk/sub';\nexport const a = s;\n"))
	if got := byRule(fs, "deps-unused"); len(got) != 0 {
		t.Errorf("a scoped workspace dependency imported through a subpath was reported unused: %+v", got)
	}
	if got := byRule(fs, "dead-workspace-packages"); len(got) != 0 {
		t.Errorf("an imported library was reported dead: %+v", got)
	}
	fs = analyze(t, npmPairWorkspace("workspace:*", "export const a = 1;\n"))
	if got := byRule(fs, "deps-unused"); len(got) != 1 {
		t.Errorf("an unused workspace dependency must be reported: %+v", got)
	}
}

func TestPortedRuntimeTestOnlyAndDevInProduction(t *testing.T) {
	// A runtime dependency imported by production and test code is fine.
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py":    "import b\n",
		"tests/test_a.py": "import b\n",
	}))
	if got := byRule(fs, "deps-runtime-test-only"); len(got) != 0 {
		t.Errorf("a runtime dependency used in production was reported test-only: %+v", got)
	}

	// A runtime dependency imported nowhere is deps-unused, not test-only.
	fs = analyze(t, twoMemberPy(`"b"`, map[string]string{"a_pkg/mod.py": "x = 1\n"}))
	if got := byRule(fs, "deps-runtime-test-only"); len(got) != 0 {
		t.Errorf("an unimported dependency was reported test-only: %+v", got)
	}

	// A dev dependency imported only by tests is neither test-only nor dev
	// in production.
	files := twoMemberPy("", map[string]string{"tests/test_a.py": "import b\n"})
	files["a/pyproject.toml"] = "[project]\nname = \"a\"\n\n[dependency-groups]\ndev = [\"b\"]\n"
	fs = analyze(t, files)
	if got := append(byRule(fs, "deps-runtime-test-only"), byRule(fs, "deps-dev-in-production")...); len(got) != 0 {
		t.Errorf("a dev dependency used by tests was reported: %+v", got)
	}

	// A runtime dependency imported by production is never dev in
	// production.
	fs = analyze(t, twoMemberPy(`"b"`, map[string]string{"a_pkg/mod.py": "import b\n"}))
	if got := byRule(fs, "deps-dev-in-production"); len(got) != 0 {
		t.Errorf("a runtime dependency was reported as dev: %+v", got)
	}
}

// Lesson 15's other half: a module imported only by a scripts/ file is
// dead, and scripts/ files are never candidates.
func TestLesson15ScriptImportsDoNotKeepModulesAlive(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":     "[project]\nname = \"p\"\n",
		"pkg/__init__.py":    "from pkg import used\n",
		"pkg/used.py":        "",
		"pkg/scriptonly.py":  "",
		"scripts/release.py": "import pkg.scriptonly\n",
	})
	dead := byRule(fs, "dead-modules")
	if !messagesContain(dead, "pkg.scriptonly") {
		t.Errorf("a module imported only by a script must be dead: %+v", dead)
	}
	if messagesContain(dead, "scripts") {
		t.Errorf("a scripts/ file was a candidate: %+v", dead)
	}
}

func TestPortedPythonPrefixImportsKeepParentsAlive(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":       "[project]\nname = \"p\"\n",
		"pkg/__init__.py":      "import pkg.sub.leaf\n",
		"pkg/sub/__init__.py":  "",
		"pkg/sub/leaf.py":      "",
		"pkg/sub/neighbour.py": "",
	})
	dead := byRule(fs, "dead-modules")
	if messagesContain(dead, "pkg.sub ") || messagesContain(dead, "pkg.sub.leaf") {
		t.Errorf("an import of pkg.sub.leaf must keep pkg.sub and the leaf alive: %+v", dead)
	}
	if !messagesContain(dead, "pkg.sub.neighbour") {
		t.Errorf("an unimported sibling must stay dead: %+v", dead)
	}
}

func TestPortedGoDeadPackageCases(t *testing.T) {
	files := goModule(map[string]string{
		"main.go":                     "package main\n\nimport \"example.com/g/internal/used\"\n\nfunc main() { used.U() }\n",
		"internal/used/u.go":          "package used\n\nfunc U() {}\n",
		"internal/unused/u.go":        "package unused\n\nimport \"example.com/g/internal/selfref\"\n\nfunc N() { selfref.R() }\n",
		"internal/selfref/r.go":       "package selfref\n\nfunc R() {}\n",
		"internal/selfref/other.go":   "package selfref\n\nimport _ \"example.com/g/internal/selfref/inner\"\n",
		"internal/selfref/inner/i.go": "package inner\n",
		"cmd/tool/main.go":            "package main\n\nfunc main() {}\n",
	})
	fs := analyze(t, files)
	if deadNamed(fs, "internal/used") {
		t.Error("an imported package was reported dead")
	}
	if !deadNamed(fs, "internal/unused") {
		t.Error("an unimported internal package must be dead")
	}
	if deadNamed(fs, "internal/selfref") {
		t.Error("a package imported by another package was reported dead")
	}
	for _, f := range byRule(fs, "dead-modules") {
		if strings.Contains(f.Message, "cmd/tool") {
			t.Errorf("a package outside internal/ was a candidate: %+v", f)
		}
	}

	// Suppressing internal/unused removes it from the candidates and from
	// the reference union, so internal/selfref, which only internal/unused
	// imports, becomes dead (lesson 14).
	files["strictcode.toml"] = "format_version = 1\n[[rules.dead-modules.suppressions]]\npath = \"internal/unused\"\nreason = \"loaded by the plugin table\"\n"
	fs = analyze(t, files)
	if deadNamed(fs, "internal/unused") {
		t.Error("a suppressed package was reported")
	}
	if !deadNamed(fs, "internal/selfref") {
		t.Error("a suppressed package's imports kept another package alive (lesson 14)")
	}
}

func TestPortedTypeScriptReachability(t *testing.T) {
	fs := analyze(t, map[string]string{
		"package.json":           `{"name": "app", "bin": "./bin/cli.js", "exports": {".": {"import": "./src/index.js", "require": "./src/index.cjs"}, "./extra": "./src/extra/index.js"}}`,
		"bin/cli.js":             "import '../src/util/a.js';\n",
		"src/index.js":           "export const i = 1;\n",
		"src/index.cjs":          "module.exports = require('./shared.js');\n",
		"src/shared.js":          "module.exports = 1;\n",
		"src/extra/index.js":     "export * from './deep.js';\n",
		"src/extra/deep.js":      "export const d = 1;\n",
		"src/util/a.js":          "import './b.js';\n",
		"src/util/b.js":          "export const b = 1;\n",
		"src/util/a.test.js":     "import './only-tests.js';\n",
		"src/util/only-tests.js": "export const t = 1;\n",
		"src/lonely.js":          "export const l = 1;\n",
		"py/pkg/__init__.py":     "",
		"py/pkg/static/app.js":   "export const embedded = 1;\n",
	})
	dead := byRule(fs, "dead-modules")
	got := map[string]bool{}
	for _, f := range dead {
		got[f.Target.File] = true
	}
	for _, want := range []string{"src/lonely.js", "src/util/only-tests.js"} {
		if !got[want] {
			t.Errorf("%s must be dead: %+v", want, dead)
		}
	}
	for _, alive := range []string{"bin/cli.js", "src/index.js", "src/index.cjs", "src/shared.js", "src/extra/index.js", "src/extra/deep.js", "src/util/a.js", "src/util/b.js", "src/util/a.test.js", "py/pkg/static/app.js"} {
		if got[alive] {
			t.Errorf("%s was reported dead: %+v", alive, dead)
		}
	}
}

func TestPortedDeadWorkspacePackagesCases(t *testing.T) {
	fs := analyze(t, map[string]string{
		fixture.DeclarationsPath: fixture.Workspace(
			"path = \"both\"\nname = \"both\"\nlibrary = true\n",
			"path = \"lonely1\"\nname = \"lonely1\"\nlibrary = true\n",
			"path = \"lonely2\"\nname = \"lonely2\"\nlibrary = true\n",
			"path = \"plain\"\nname = \"plain\"\n",
			"path = \"app\"\nname = \"app\"\n"),
		"both/pyproject.toml":         "[project]\nname = \"both\"\n",
		"both/both/__init__.py":       "",
		"lonely1/pyproject.toml":      "[project]\nname = \"lonely1\"\n",
		"lonely1/lonely1/__init__.py": "import lonely1.x\n",
		"lonely1/lonely1/x.py":        "",
		"lonely2/pyproject.toml":      "[project]\nname = \"lonely2\"\n",
		"lonely2/lonely2/__init__.py": "",
		"plain/pyproject.toml":        "[project]\nname = \"plain\"\n",
		"plain/plain/__init__.py":     "",
		"app/pyproject.toml":          "[project]\nname = \"app\"\ndependencies = [\"both\"]\n",
		"app/app/__init__.py":         "import both\n",
		"app/tests/test_app.py":       "import both\n",
	})
	dead := byRule(fs, "dead-workspace-packages")
	if len(dead) != 2 || !messagesContain(dead, "'lonely1'") || !messagesContain(dead, "'lonely2'") {
		t.Fatalf("want lonely1 and lonely2 reported, both alive, plain exempt: %+v", dead)
	}
}

// Lesson 46: a member whose import name differs from its distribution name
// (core installs portal_core; cloudflare installs cf) is matched by the
// package directory it ships, so its dependents' imports are neither unused
// nor undeclared, and the library is not dead.
func TestLesson46ImportNameDiffersFromDistributionName(t *testing.T) {
	fs := analyze(t, map[string]string{
		fixture.DeclarationsPath: fixture.Workspace(
			"path = \"core\"\nname = \"core\"\nlibrary = true\n",
			"path = \"cloudflare\"\nname = \"cloudflare\"\nlibrary = true\n",
			"path = \"cli\"\nname = \"cli\"\n"),
		"core/pyproject.toml":           "[project]\nname = \"core\"\n\n[tool.hatch.build.targets.wheel]\npackages = [\"portal_core\"]\n",
		"core/portal_core/__init__.py":  "",
		"cloudflare/pyproject.toml":     "[project]\nname = \"cloudflare\"\n\n[tool.hatch.build.targets.wheel]\npackages = [\"src/cf\"]\n",
		"cloudflare/src/cf/__init__.py": "",
		"cli/pyproject.toml":            "[project]\nname = \"cli\"\ndependencies = [\"core\", \"cloudflare\"]\n",
		"cli/cli/__init__.py":           "from portal_core import x\nimport cf\n",
	})
	for _, rule := range []string{"deps-unused", "deps-undeclared", "dead-workspace-packages"} {
		if got := byRule(fs, rule); len(got) != 0 {
			t.Errorf("%s: an import under a different import name was misread: %+v", rule, got)
		}
	}
}
