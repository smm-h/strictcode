// The lessons register (DESIGN.md section 6.7) as the acceptance suite:
// each applicable item is a regression test, written before the checks were
// implemented (red-green). Lesson numbers are cited inline. Lessons 6-8 and
// 17-19 are additionally covered at their home packages (testctx, extract);
// lessons 24-25 and 27 concern rules outside the import-graph round.
package checks

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/fixture"
	"github.com/smm-h/strictcode/internal/workspace"
)

// analyze runs the full check pipeline over a fixture. The fixture may
// contain a strictcode.toml; it is loaded as the config.
func analyze(t *testing.T, files map[string]string) []findings.Finding {
	t.Helper()
	root := fixture.Write(t, files)
	ws, err := workspace.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root + "/strictcode.toml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Extract(ws)
	if err != nil {
		t.Fatal(err)
	}
	return Run(ws, res, cfg, "strictcode.toml")
}

func byRule(fs []findings.Finding, rule string) []findings.Finding {
	var out []findings.Finding
	for _, f := range fs {
		if f.Rule == rule {
			out = append(out, f)
		}
	}
	return out
}

func messagesContain(fs []findings.Finding, sub string) bool {
	for _, f := range fs {
		if strings.Contains(f.Message, sub) {
			return true
		}
	}
	return false
}

// twoMemberPy builds a Python workspace: member a (importing) and b
// (imported), with a's pyproject declaring deps and a's source provided.
func twoMemberPy(aDeps string, aSources map[string]string) map[string]string {
	files := map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"a\"\nname = \"a\"\n\n[[projects]]\npath = \"b\"\nname = \"b\"\n",
		"a/pyproject.toml":               "[project]\nname = \"a\"\ndependencies = [" + aDeps + "]\n",
		"a/a_pkg/__init__.py":            "",
		"b/pyproject.toml":               "[project]\nname = \"b\"\n",
		"b/b/__init__.py":                "",
	}
	for path, content := range aSources {
		files["a/"+path] = content
	}
	return files
}

// --- Lessons 1-5: guards and TYPE_CHECKING --------------------------------

func TestLesson1GuardedImportCountsAsUsedForDepsUnused(t *testing.T) {
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "try:\n    import b\nexcept ImportError:\n    b = None\n",
	}))
	if len(byRule(fs, "deps-unused")) != 0 {
		t.Fatalf("guarded import must count as used for deps-unused: %+v", fs)
	}
}

func TestLesson2HardDepGuardedOnlyIsFlagged(t *testing.T) {
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "try:\n    import b\nexcept ImportError:\n    b = None\n",
	}))
	got := byRule(fs, "deps-hard-guarded-only")
	if len(got) != 1 {
		t.Fatalf("hard dep imported only under guards must be flagged: %+v", fs)
	}
	if !strings.Contains(got[0].Message, "optional") {
		t.Fatalf("message must tell the author to declare it optional or import unconditionally: %q", got[0].Message)
	}
}

func TestLesson2GuardsSatisfyOptionalScopes(t *testing.T) {
	// The same guarded-only import of a dev/peer-scoped dep is fine.
	files := twoMemberPy("", map[string]string{
		"a_pkg/mod.py": "try:\n    import b\nexcept ImportError:\n    b = None\n",
	})
	files["a/pyproject.toml"] = "[project]\nname = \"a\"\n\n[project.optional-dependencies]\nextra = [\"b\"]\n"
	fs := analyze(t, files)
	if len(byRule(fs, "deps-hard-guarded-only")) != 0 || len(byRule(fs, "deps-unused")) != 0 {
		t.Fatalf("guarded import of an optional dep is legitimate: %+v", fs)
	}
}

func TestLesson3FallbackImportIsNotGuarded(t *testing.T) {
	// The import of b sits in the EXCEPT body: not guarded, so the hard dep
	// is genuinely (unconditionally-on-fallback) imported — no findings.
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "try:\n    import fastb\nexcept ImportError:\n    import b\n",
	}))
	if len(byRule(fs, "deps-hard-guarded-only")) != 0 {
		t.Fatalf("except-body import must not be treated as guarded: %+v", fs)
	}
	if len(byRule(fs, "deps-unused")) != 0 {
		t.Fatalf("except-body import still marks the dep used: %+v", fs)
	}
}

func TestLesson4DepsUndeclaredIgnoresGuardedImports(t *testing.T) {
	fs := analyze(t, twoMemberPy("", map[string]string{
		"a_pkg/mod.py": "try:\n    import b\nexcept ImportError:\n    b = None\n",
	}))
	if len(byRule(fs, "deps-undeclared")) != 0 {
		t.Fatalf("guarded optional import need not be declared: %+v", fs)
	}
}

func TestLesson5TypeCheckingExcludedFromBoth(t *testing.T) {
	// Declared dep imported only under TYPE_CHECKING: unused.
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n    import b\n",
	}))
	if len(byRule(fs, "deps-unused")) != 1 {
		t.Fatalf("TYPE_CHECKING import must not count as used: %+v", fs)
	}
	// Undeclared dep imported only under TYPE_CHECKING: not flagged.
	fs = analyze(t, twoMemberPy("", map[string]string{
		"a_pkg/mod.py": "from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n    import b\n",
	}))
	if len(byRule(fs, "deps-undeclared")) != 0 {
		t.Fatalf("TYPE_CHECKING import must not need declaration: %+v", fs)
	}
}

// --- Lessons 6-8: test-context classification in checks -------------------

func TestLesson6ProductionSrcTestDirIsProduction(t *testing.T) {
	// A runtime dep imported only from src/test/handler.py (production —
	// root-relative matching) must NOT be deps-runtime-test-only.
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{
		"src/test/handler.py": "import b\n",
	}))
	if len(byRule(fs, "deps-runtime-test-only")) != 0 {
		t.Fatalf("src/test/ is production code (lesson 6): %+v", fs)
	}
}

func TestLesson8RuntimeDepImportedOnlyByTestsIsFlagged(t *testing.T) {
	fs := analyze(t, twoMemberPy(`"b"`, map[string]string{
		"tests/test_x.py": "import b\n",
	}))
	if len(byRule(fs, "deps-runtime-test-only")) != 1 {
		t.Fatalf("runtime dep imported only by tests must be flagged: %+v", fs)
	}
	// And the test import still counts as "used" for deps-unused.
	if len(byRule(fs, "deps-unused")) != 0 {
		t.Fatalf("test import counts as used: %+v", fs)
	}
}

// --- Lesson 9: Go testdata / test-only packages ---------------------------

func TestLesson9GoTestPackagesNeverDead(t *testing.T) {
	fs := analyze(t, map[string]string{
		"go.mod":                     "module example.com/solo\n\ngo 1.22\n",
		"main.go":                    "package main\n\nfunc main() {}\n",
		"internal/testdata/x/gen.go": "package gen\n",
		"internal/only/only_test.go": "package only\n",
		"internal/genuine/dead.go":   "package genuine\n",
	})
	dead := byRule(fs, "dead-modules")
	if len(dead) != 1 {
		t.Fatalf("exactly the genuine package must be dead: %+v", dead)
	}
	if !strings.Contains(dead[0].Message, "internal/genuine") {
		t.Fatalf("wrong dead package: %q", dead[0].Message)
	}
}

// --- Lessons 10-12: member resolution in dep checks -----------------------

func TestLesson10RegistryNameMismatchNoFalseUndeclared(t *testing.T) {
	fs := analyze(t, map[string]string{
		".rlsbl-monorepo/workspace.toml":         "[[projects]]\npath = \"app\"\nname = \"app\"\n\n[[projects]]\npath = \"transport\"\nname = \"transport\"\n",
		"app/pyproject.toml":                     "[project]\nname = \"app\"\ndependencies = [\"orxtra-transport\"]\n",
		"app/app/__init__.py":                    "import orxtra_transport\n",
		"transport/pyproject.toml":               "[project]\nname = \"orxtra-transport\"\n",
		"transport/orxtra_transport/__init__.py": "",
	})
	if len(byRule(fs, "deps-undeclared")) != 0 {
		t.Fatalf("registry-name import of a declared dep flagged (lesson 10): %+v", fs)
	}
	if len(byRule(fs, "deps-unused")) != 0 {
		t.Fatalf("registry-name import must mark the dep used: %+v", fs)
	}
}

func TestLesson11NamespaceImportResolves(t *testing.T) {
	fs := analyze(t, map[string]string{
		".rlsbl-monorepo/workspace.toml":           "[[projects]]\npath = \"app\"\nname = \"app\"\n\n[[projects]]\npath = \"transport\"\nname = \"transport\"\n",
		"app/pyproject.toml":                       "[project]\nname = \"app\"\ndependencies = [\"transport\"]\n",
		"app/app/__init__.py":                      "from orxt.transport import client\n",
		"transport/pyproject.toml":                 "[project]\nname = \"transport\"\n",
		"transport/src/orxt/transport/__init__.py": "",
		"transport/src/orxt/transport/client.py":   "",
	})
	if len(byRule(fs, "deps-undeclared")) != 0 || len(byRule(fs, "deps-unused")) != 0 {
		t.Fatalf("namespace import must resolve to the member (lesson 11): %+v", fs)
	}
}

func TestLesson12ImportNameOverrideHonored(t *testing.T) {
	fs := analyze(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"app\"\nname = \"app\"\n\n[[projects]]\npath = \"weird\"\nname = \"weird\"\nimport_name = \"totally_custom\"\n",
		"app/pyproject.toml":             "[project]\nname = \"app\"\ndependencies = [\"weird\"]\n",
		"app/app/__init__.py":            "import totally_custom\n",
		"weird/pyproject.toml":           "[project]\nname = \"weird\"\n",
		"weird/lib/__init__.py":          "",
	})
	if len(byRule(fs, "deps-undeclared")) != 0 || len(byRule(fs, "deps-unused")) != 0 {
		t.Fatalf("import_name override must be honored (lesson 12): %+v", fs)
	}
}

// --- Lesson 13: sibling pruning -------------------------------------------

func TestLesson13SiblingSourceNeverTriggersUndeclared(t *testing.T) {
	fs := analyze(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \".\"\nname = \"root\"\n\n[[projects]]\npath = \"sub\"\nname = \"sub\"\n\n[[projects]]\npath = \"other\"\nname = \"other\"\n",
		"pyproject.toml":                 "[project]\nname = \"root\"\n",
		"rootpkg/__init__.py":            "",
		"sub/pyproject.toml":             "[project]\nname = \"sub\"\ndependencies = [\"other\"]\n",
		"sub/subpkg/__init__.py":         "import other\n",
		"other/pyproject.toml":           "[project]\nname = \"other\"\n",
		"other/other/__init__.py":        "",
	})
	// sub's import of other is declared by sub; root must not report it.
	for _, f := range byRule(fs, "deps-undeclared") {
		t.Errorf("sibling source produced deps-undeclared: %+v", f)
	}
}

// --- Lessons 14-16: dead-modules (Python) ---------------------------------

func deadPyWorkspace(cfg string) map[string]string {
	files := map[string]string{
		"pyproject.toml":    "[project]\nname = \"solo\"\n",
		"pkg/__init__.py":   "from . import entry\n",
		"pkg/entry.py":      "import pkg.mid\n",
		"pkg/mid.py":        "import pkg.deep\n",
		"pkg/deep.py":       "",
		"pkg/orphan.py":     "",
		"scripts/build.py":  "import pkg.scriptonly\n",
		"pkg/scriptonly.py": "",
	}
	if cfg != "" {
		files["strictcode.toml"] = cfg
	}
	return files
}

func TestPyDeadModulesBaseline(t *testing.T) {
	fs := analyze(t, deadPyWorkspace(""))
	dead := byRule(fs, "dead-modules")
	// orphan (nobody imports) and scriptonly (only scripts/ imports it —
	// lesson 15) are dead; the pkg chain is alive; scripts/build.py itself
	// is not a candidate (lesson 15).
	if len(dead) != 2 {
		t.Fatalf("want orphan+scriptonly dead: %+v", dead)
	}
	if !messagesContain(dead, "pkg.orphan") || !messagesContain(dead, "pkg.scriptonly") {
		t.Fatalf("wrong dead set: %+v", dead)
	}
}

func TestLesson14NoEntryPointLaundering(t *testing.T) {
	// Suppress pkg/mid.py: it leaves the candidate set AND the reference
	// union — pkg.deep, kept alive only by mid, must now be reported.
	fs := analyze(t, deadPyWorkspace(`
format_version = 1
[[rules.dead-modules.suppressions]]
path = "pkg/mid.py"
reason = "kept while the migration completes"
`))
	dead := byRule(fs, "dead-modules")
	if messagesContain(dead, "pkg.mid") {
		t.Fatalf("suppressed module reported: %+v", dead)
	}
	if !messagesContain(dead, "pkg.deep") {
		t.Fatalf("suppressed module's imports kept pkg.deep alive (laundering, lesson 14): %+v", dead)
	}
}

func TestLesson16InitExportExemption(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "__all__ = [\"api\"]\n",
		"pkg/api.py":      "",
		"pkg/hidden.py":   "",
	})
	dead := byRule(fs, "dead-modules")
	if messagesContain(dead, "pkg.api") {
		t.Fatalf("__all__-exported module flagged dead (lesson 16): %+v", dead)
	}
	if !messagesContain(dead, "pkg.hidden") {
		t.Fatalf("unexported module must still be dead: %+v", dead)
	}
}

// --- Lesson 18: TS dead modules via BFS -----------------------------------

func TestLesson18TSReachabilityThroughMappedImports(t *testing.T) {
	fs := analyze(t, map[string]string{
		"package.json":     "{\n  \"name\": \"app\",\n  \"main\": \"./src/index.ts\"\n}\n",
		"src/index.ts":     "export { x } from './helper.js';\nimport './sub';\n",
		"src/helper.ts":    "export const x = 1;\n",
		"src/sub/index.ts": "import { orphaned } from '../orphan';\n",
		"src/orphan.ts":    "export const orphaned = 1;\n",
		"src/unreached.ts": "export const u = 1;\n",
	})
	dead := byRule(fs, "dead-modules")
	// helper reached via .js->.ts mapping; sub via directory index; orphan
	// via sub. Only unreached is dead.
	if len(dead) != 1 || !strings.Contains(dead[0].Message, "src/unreached") {
		t.Fatalf("BFS reachability wrong (lesson 18): %+v", dead)
	}
}

func TestTSSuppressedModuleEdgesNotTraversed(t *testing.T) {
	// Lesson 14, BFS variant: a suppressed unit's edges are never traversed.
	fs := analyze(t, map[string]string{
		"package.json":   "{\n  \"name\": \"app\",\n  \"main\": \"./src/index.ts\"\n}\n",
		"src/index.ts":   "import './carrier';\n",
		"src/carrier.ts": "import './cargo';\n",
		"src/cargo.ts":   "export const c = 1;\n",
		"strictcode.toml": `
format_version = 1
[[rules.dead-modules.suppressions]]
path = "src/carrier.ts"
reason = "generated import hub"
`,
	})
	dead := byRule(fs, "dead-modules")
	if !messagesContain(dead, "src/cargo") {
		t.Fatalf("suppressed module's edges were traversed (lesson 14 BFS): %+v", dead)
	}
}

// --- Lessons 20-21: cycles ------------------------------------------------

func TestLesson20NoCycleCheckOnGo(t *testing.T) {
	fs := analyze(t, map[string]string{
		"go.mod":  "module example.com/x\n\ngo 1.22\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})
	if len(byRule(fs, "import-cycles")) != 0 {
		t.Fatalf("import-cycles ran on Go (lesson 20): %+v", fs)
	}
}

func TestLesson21CyclesAreSCCsOfTwoPlus(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "from . import a\n",
		"pkg/a.py":        "import pkg.b\nimport pkg.a\n", // self-loop ignored
		"pkg/b.py":        "import pkg.a\n",
	})
	cycles := byRule(fs, "import-cycles")
	if len(cycles) != 1 {
		t.Fatalf("want exactly one SCC finding: %+v", cycles)
	}
	if !strings.Contains(cycles[0].Message, "pkg.a") || !strings.Contains(cycles[0].Message, "pkg.b") {
		t.Fatalf("cycle members missing from message: %q", cycles[0].Message)
	}
}

func TestCycleSuppressionByMemberSet(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "from . import a\n",
		"pkg/a.py":        "import pkg.b\n",
		"pkg/b.py":        "import pkg.a\n",
		"strictcode.toml": `
format_version = 1
[[rules.import-cycles.suppressions]]
modules = ["pkg.b", "pkg.a"]
reason = "known cycle, split scheduled"
`,
	})
	if got := byRule(fs, "import-cycles"); len(got) != 0 {
		t.Fatalf("member-set suppression not honored: %+v", got)
	}
}

// --- Lessons 22-23, 26: library boundary ----------------------------------

func libWorkspace(library bool, extra map[string]string) map[string]string {
	lib := ""
	if library {
		lib = "library = true\n"
	}
	files := map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"m\"\nname = \"m\"\n" + lib,
		"m/pyproject.toml":               "[project]\nname = \"m\"\n",
		"m/pkg/__init__.py":              "import flask\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

func TestLesson22LibraryLintOnlyOnLibraries(t *testing.T) {
	if got := byRule(analyze(t, libWorkspace(false, nil)), "library-forbidden-imports"); len(got) != 0 {
		t.Fatalf("library rule ran on a non-library member (lesson 22): %+v", got)
	}
	got := byRule(analyze(t, libWorkspace(true, nil)), "library-forbidden-imports")
	if len(got) != 1 || !strings.Contains(got[0].Message, "flask") {
		t.Fatalf("library flask import must be flagged: %+v", got)
	}
}

func TestLesson23DefaultTestExcludesApply(t *testing.T) {
	files := libWorkspace(true, map[string]string{
		"m/pkg/__init__.py":   "",
		"m/tests/test_app.py": "import flask\n",
		"m/examples/demo.py":  "import flask\n",
	})
	if got := byRule(analyze(t, files), "library-forbidden-imports"); len(got) != 0 {
		t.Fatalf("test/example files must be excluded (lesson 23): %+v", got)
	}
}

func TestLesson26BothAllowListsSubtracted(t *testing.T) {
	// flask allowed via workspace lint_allow; click allowed via the
	// per-language config allow list; django stays forbidden.
	files := map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"m\"\nname = \"m\"\nlibrary = true\nlint_allow = [\"flask\"]\n",
		"m/pyproject.toml":               "[project]\nname = \"m\"\n",
		"m/pkg/__init__.py":              "import flask\nimport click\nimport django\n",
		"strictcode.toml": `
format_version = 1
[rules.library-forbidden-imports.allow]
py = ["click"]
`,
	}
	got := byRule(analyze(t, files), "library-forbidden-imports")
	if len(got) != 1 || !strings.Contains(got[0].Message, "django") {
		t.Fatalf("both allow lists must be subtracted (lesson 26): %+v", got)
	}
}

func TestLibraryEntryPoint(t *testing.T) {
	files := map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"m\"\nname = \"m\"\nlibrary = true\n",
		"m/pyproject.toml":               "[project]\nname = \"m\"\n\n[project.scripts]\nm-cli = \"pkg.main:run\"\n",
		"m/pkg/__init__.py":              "",
		"m/pkg/main.py":                  "def run(): pass\n",
	}
	got := byRule(analyze(t, files), "library-entry-point")
	if len(got) != 1 || !strings.Contains(got[0].Message, "m-cli") {
		t.Fatalf("library CLI entry point must be flagged: %+v", got)
	}
	// npm "export" form entry points are the normal library surface — never
	// flagged.
	tsFiles := map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"m\"\nname = \"m\"\nlibrary = true\n",
		"m/package.json":                 "{\n  \"name\": \"m\",\n  \"main\": \"./index.ts\"\n}\n",
		"m/index.ts":                     "export const x = 1;\n",
	}
	if got := byRule(analyze(t, tsFiles), "library-entry-point"); len(got) != 0 {
		t.Fatalf("npm export/main entry points are not CLI entry points: %+v", got)
	}
}

// --- Lesson 28: dead workspace packages -----------------------------------

func TestLesson28DeadWorkspacePackages(t *testing.T) {
	fs := analyze(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": `
[[releasables]]
name = "pub"

[[projects]]
path = "used"
name = "used"
library = true

[[projects]]
path = "unused"
name = "unused"
library = true

[[projects]]
path = "testonly"
name = "testonly"
library = true

[[projects]]
path = "published"
name = "published"
library = true
releasable = "pub"

[[projects]]
path = "devtool"
name = "devtool"
library = true
dev_only = true

[[projects]]
path = "app"
name = "app"
`,
		"used/pyproject.toml":             "[project]\nname = \"used\"\n",
		"used/used/__init__.py":           "import used.sub\n",
		"used/used/sub.py":                "",
		"unused/pyproject.toml":           "[project]\nname = \"unused\"\n",
		"unused/unused/__init__.py":       "",
		"testonly/pyproject.toml":         "[project]\nname = \"testonly\"\n",
		"testonly/testonly/__init__.py":   "",
		"published/pyproject.toml":        "[project]\nname = \"published\"\n",
		"published/published/__init__.py": "",
		"devtool/pyproject.toml":          "[project]\nname = \"devtool\"\n",
		"devtool/devtool/__init__.py":     "",
		"app/pyproject.toml":              "[project]\nname = \"app\"\ndependencies = [\"used\", \"testonly\"]\n",
		"app/app/__init__.py":             "import used\n",
		"app/tests/test_t.py":             "import testonly\n",
	})
	dead := byRule(fs, "dead-workspace-packages")
	msgs := map[string]string{}
	for _, f := range dead {
		for _, name := range []string{"unused", "testonly", "published", "devtool", "used", "app"} {
			if strings.Contains(f.Message, "'"+name+"'") {
				msgs[name] = f.Message
			}
		}
	}
	if _, ok := msgs["unused"]; !ok {
		t.Fatalf("unused library member must be reported: %+v", dead)
	}
	if m, ok := msgs["testonly"]; !ok || !strings.Contains(m, "test") {
		t.Fatalf("test-only importers must produce a distinct message: %+v", dead)
	}
	// Exemptions: published (releasable), devtool (dev-only), app
	// (non-library), used (has production importer; self-import of used.sub
	// never counts for itself).
	for _, exempt := range []string{"published", "devtool", "app", "used"} {
		if m, ok := msgs[exempt]; ok {
			t.Errorf("member %q must be exempt (lesson 28): %s", exempt, m)
		}
	}
}

// --- Lesson 29: excluded directories --------------------------------------

func TestLesson29AssetDirsExcluded(t *testing.T) {
	fs := analyze(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"m\"\nname = \"m\"\nlibrary = true\n",
		"m/pyproject.toml":               "[project]\nname = \"m\"\n",
		"m/pkg/__init__.py":              "",
		"m/.venv/site/flask_user.py":     "import flask\n",
		"m/node_modules/x/index.js":      "import 'express';\n",
		"m/build/gen.py":                 "import django\n",
	})
	if got := byRule(fs, "library-forbidden-imports"); len(got) != 0 {
		t.Fatalf("excluded dirs were scanned (lesson 29): %+v", got)
	}
	if got := byRule(fs, "dead-modules"); messagesContain(got, "flask_user") {
		t.Fatalf(".venv modules in dead-modules output: %+v", got)
	}
}

// --- Lesson 32 + suppression staleness ------------------------------------

func TestLesson32StaleSuppressionPathIsError(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "",
		"strictcode.toml": `
format_version = 1
[[rules.dead-modules.suppressions]]
path = "pkg/vanished.py"
reason = "was kept for reference"
`,
	})
	stale := byRule(fs, "stale-suppression")
	if len(stale) != 1 || stale[0].Severity != "error" {
		t.Fatalf("nonexistent suppression path must be an error finding (lesson 32): %+v", fs)
	}
	if !strings.Contains(stale[0].Message, "pkg/vanished.py") {
		t.Fatalf("message must name the stale path: %q", stale[0].Message)
	}
}

func TestStaleProjectDepSuppression(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "",
		"strictcode.toml": `
format_version = 1
[[rules.deps-unused.suppressions]]
project = "ghost"
dep = "phantom"
reason = "these members are gone"
`,
	})
	if got := byRule(fs, "stale-suppression"); len(got) != 1 {
		t.Fatalf("suppression naming nonexistent members must be stale: %+v", fs)
	}
}

// --- Suppressions and config integration ----------------------------------

func TestProjectDepSuppressionHonored(t *testing.T) {
	files := twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "x = 1\n", // b never imported
	})
	files["strictcode.toml"] = `
format_version = 1
[[rules.deps-unused.suppressions]]
project = "a"
dep = "b"
reason = "b is loaded via plugin discovery"
`
	fs := analyze(t, files)
	if got := byRule(fs, "deps-unused"); len(got) != 0 {
		t.Fatalf("(project, dep) suppression not honored: %+v", got)
	}
}

func TestDisabledRuleDoesNotRun(t *testing.T) {
	files := twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "x = 1\n",
	})
	files["strictcode.toml"] = "format_version = 1\n[rules.deps-unused]\nenabled = false\n"
	if got := byRule(analyze(t, files), "deps-unused"); len(got) != 0 {
		t.Fatalf("disabled rule produced findings: %+v", got)
	}
}

func TestSeverityOverrideAppliedToFindings(t *testing.T) {
	files := twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "x = 1\n",
	})
	files["strictcode.toml"] = "format_version = 1\n[rules.deps-unused]\nseverity = \"warning\"\n"
	got := byRule(analyze(t, files), "deps-unused")
	if len(got) != 1 || got[0].Severity != "warning" {
		t.Fatalf("severity override not applied: %+v", got)
	}
}

func TestDepsUnusedAndUndeclaredBaseline(t *testing.T) {
	// Undeclared production import.
	fs := analyze(t, twoMemberPy("", map[string]string{
		"a_pkg/mod.py": "import b\n",
	}))
	if got := byRule(fs, "deps-undeclared"); len(got) != 1 {
		t.Fatalf("undeclared import must be flagged: %+v", fs)
	}
	// Declared, never imported.
	fs = analyze(t, twoMemberPy(`"b"`, map[string]string{
		"a_pkg/mod.py": "x = 1\n",
	}))
	if got := byRule(fs, "deps-unused"); len(got) != 1 {
		t.Fatalf("unused declared dep must be flagged: %+v", fs)
	}
}

func TestDepsDevInProduction(t *testing.T) {
	files := twoMemberPy("", map[string]string{
		"a_pkg/mod.py": "import b\n",
	})
	files["a/pyproject.toml"] = "[project]\nname = \"a\"\n\n[dependency-groups]\ndev = [\"b\"]\n"
	got := byRule(analyze(t, files), "deps-dev-in-production")
	if len(got) != 1 {
		t.Fatalf("dev dep imported by production code must be flagged: %+v", got)
	}
	// A guarded production import of a dev dep is the legitimate
	// optional-dependency pattern — not flagged.
	files["a/a_pkg/mod.py"] = "try:\n    import b\nexcept ImportError:\n    b = None\n"
	if got := byRule(analyze(t, files), "deps-dev-in-production"); len(got) != 0 {
		t.Fatalf("guarded optional use of a dev dep flagged: %+v", got)
	}
}

func TestSelfImportsAreExempt(t *testing.T) {
	// A package importing its own submodules is never deps-undeclared.
	fs := analyze(t, map[string]string{
		"pyproject.toml":   "[project]\nname = \"solo\"\n",
		"solo/__init__.py": "import solo.core\n",
		"solo/core.py":     "",
	})
	if got := byRule(fs, "deps-undeclared"); len(got) != 0 {
		t.Fatalf("self-import flagged as undeclared: %+v", got)
	}
}

func TestPyDunderMainIsImplicitEntryPoint(t *testing.T) {
	// Found on the real corpus (rlsbl): pkg/__main__.py is the `python -m
	// pkg` entry point — an implicit entry, never a dead-module candidate.
	fs := analyze(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "",
		"pkg/__main__.py": "import pkg\n",
	})
	if messagesContain(byRule(fs, "dead-modules"), "__main__") {
		t.Fatalf("pkg/__main__.py flagged dead: %+v", fs)
	}
}

func TestTSNoResolvedEntryPointsMeansNoDeadReport(t *testing.T) {
	// Donor safeguard (found on the real corpus): when no entry point
	// resolves to a scanned source file (e.g. exports point at built dist/
	// output), reachability cannot be determined — the BFS abstains instead
	// of reporting the whole tree dead.
	fs := analyze(t, map[string]string{
		"package.json":  "{\n  \"name\": \"app\",\n  \"main\": \"./dist/index.js\",\n  \"exports\": {\".\": \"./dist/index.mjs\"}\n}\n",
		"src/index.ts":  "import './helper';\n",
		"src/helper.ts": "export const h = 1;\n",
	})
	if got := byRule(fs, "dead-modules"); len(got) != 0 {
		t.Fatalf("BFS must abstain without resolved entry points: %+v", got)
	}
}

func TestGoNestedModuleDeclaredDeps(t *testing.T) {
	// Found on the real corpus: a member whose Go code lives in nested
	// modules (conformance harnesses). The nested go.mod's requires are the
	// member's declared deps — no false deps-undeclared.
	fs := analyze(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"conf\"\nname = \"conf\"\n\n[[projects]]\npath = \"lib\"\nname = \"lib\"\n",
		"conf/harness/go.mod":            "module example.com/conf/harness\n\ngo 1.22\n\nrequire example.com/lib v0.1.0\n",
		"conf/harness/main.go":           "package main\n\nimport \"example.com/lib\"\n\nfunc main() { _ = lib.V }\n",
		"lib/go.mod":                     "module example.com/lib\n\ngo 1.22\n",
		"lib/lib.go":                     "package lib\n\nvar V = 1\n",
	})
	if got := byRule(fs, "deps-undeclared"); len(got) != 0 {
		t.Fatalf("nested go.mod requires must count as declared: %+v", got)
	}
	if got := byRule(fs, "deps-unused"); len(got) != 0 {
		t.Fatalf("the declared dep is used: %+v", got)
	}
}
