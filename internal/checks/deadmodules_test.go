package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/fixture"
	"github.com/smm-h/strictcode/internal/workspace"
)

// deadNamed reports whether a dead-modules finding names the unit.
func deadNamed(fs []findings.Finding, unit string) bool {
	for _, f := range byRule(fs, "dead-modules") {
		if strings.Contains(f.Message, " "+unit+" ") {
			return true
		}
	}
	return false
}

// goModule is a single-project Go module named example.com/g.
func goModule(files map[string]string) map[string]string {
	out := map[string]string{"go.mod": "module example.com/g\n\ngo 1.22\n"}
	for k, v := range files {
		out[k] = v
	}
	return out
}

// Lesson 40: a directory named like a build artifact inside a source tree
// is an ordinary package and is walked; the name is left out only directly
// under a member's root.
func TestLesson40BuildDirectoryInsideASourceTreeIsWalked(t *testing.T) {
	fs := analyze(t, goModule(map[string]string{
		"main.go":                    "package main\n\nimport \"example.com/g/internal/build\"\n\nfunc main() { build.Run() }\n",
		"internal/build/build.go":    "package build\n\nimport (\n\t\"example.com/g/internal/gen\"\n\t\"example.com/g/internal/rules/sealed\"\n)\n\nfunc Run() { gen.X(); sealed.Y() }\n",
		"internal/gen/gen.go":        "package gen\n\nfunc X() {}\n",
		"internal/rules/sealed/s.go": "package sealed\n\nfunc Y() {}\n",
	}))
	for _, unit := range []string{"internal/gen", "internal/rules/sealed", "internal/build"} {
		if deadNamed(fs, unit) {
			t.Errorf("%s is imported through internal/build but was reported dead: %+v", unit, byRule(fs, "dead-modules"))
		}
	}
}

func TestLesson40BuildDirectoryAtTheMemberRootIsNotWalked(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":   "[project]\nname = \"p\"\n",
		"pkg/__init__.py":  "",
		"pkg/used.py":      "",
		"pkg/lonely.py":    "",
		"build/lib/run.py": "import pkg.used\nimport pkg.lonely\n",
	})
	if !deadNamed(fs, "pkg.lonely") {
		t.Fatalf("a build/ directory at the member root kept a module alive: %+v", byRule(fs, "dead-modules"))
	}
}

// Lesson 41: a source walk reads only what git lists, so a gitignored file
// (a third-party clone, say) is never read and never keeps a module alive.
func TestLesson41GitignoredFilesAreNeverRead(t *testing.T) {
	fs := analyze(t, map[string]string{
		".gitignore":                  "third_party/\n",
		fixture.DeclarationsPath:      fixture.Workspace("path = \"m\"\nname = \"m\"\nlibrary = true\n"),
		"m/pyproject.toml":            "[project]\nname = \"m\"\n",
		"m/pkg/__init__.py":           "",
		"m/pkg/lonely.py":             "",
		"m/third_party/clone/x.py":    "import flask\nimport pkg.lonely\n",
		"m/third_party/clone/deep.py": "x = (" + strings.Repeat("1 + ", 5000) + "1)\n",
	})
	if got := byRule(fs, "library-forbidden-imports"); len(got) != 0 {
		t.Errorf("a gitignored file was read: %+v", got)
	}
	if !deadNamed(fs, "pkg.lonely") {
		t.Errorf("a gitignored file kept a module alive: %+v", byRule(fs, "dead-modules"))
	}
}

func TestLesson41AWorkspaceOutsideGitIsRefused(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"p\"\n",
		"pkg/__init__.py": "",
	})
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = extract.Extract(ws)
	if err == nil || !strings.Contains(err.Error(), "git work tree") {
		t.Fatalf("a directory outside git was not refused: %v", err)
	}
}

// Lesson 42: an internal Go package imported only by other packages'
// _test.go files (a test helper) is alive, and an internal package main (a
// program run with go run) is an entry point, never a candidate; a
// package's own tests and testdata/ files never keep it alive.
func TestLesson42GoTestHelpersAndMainPackages(t *testing.T) {
	fs := analyze(t, goModule(map[string]string{
		"lib.go":                          "package g\n\nfunc F() int { return 1 }\n",
		"lib_test.go":                     "package g\n\nimport (\n\t\"testing\"\n\n\t\"example.com/g/internal/clitest\"\n)\n\nfunc TestF(t *testing.T) { clitest.Check(t) }\n",
		"internal/clitest/clitest.go":     "package clitest\n\nimport \"testing\"\n\nfunc Check(t *testing.T) {}\n",
		"internal/tools/gen/main.go":      "package main\n\nfunc main() {}\n",
		"internal/selfonly/s.go":          "package selfonly\n\nfunc S() {}\n",
		"internal/selfonly/s_test.go":     "package selfonly_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/g/internal/selfonly\"\n)\n\nfunc TestS(t *testing.T) { selfonly.S() }\n",
		"internal/fixtureonly/f.go":       "package fixtureonly\n\nfunc F() {}\n",
		"internal/x/testdata/user/use.go": "package user\n\nimport \"example.com/g/internal/fixtureonly\"\n\nvar _ = fixtureonly.F\n",
	}))
	if deadNamed(fs, "internal/clitest") {
		t.Error("a test helper imported by another package's tests was reported dead")
	}
	if deadNamed(fs, "internal/tools/gen") {
		t.Error("a main package was reported dead")
	}
	if !deadNamed(fs, "internal/selfonly") {
		t.Error("a package imported only by its own tests must stay dead")
	}
	if !deadNamed(fs, "internal/fixtureonly") {
		t.Error("an import from testdata/ kept a package alive")
	}
}

// Lesson 43: a member nested in another member's directory is left out of
// the enclosing member's walk; its files are never the enclosing member's
// dead-module candidates or imports.
func TestLesson43NestedMembersAreLeftOutOfTheEnclosingWalk(t *testing.T) {
	fs := analyze(t, map[string]string{
		fixture.DeclarationsPath:           fixture.Workspace("path = \"sdk\"\nname = \"sdk\"\n", "path = \"sdk/python\"\nname = \"sdk-python\"\n"),
		"sdk/pyproject.toml":               "[project]\nname = \"sdk\"\n",
		"sdk/sdk/__init__.py":              "",
		"sdk/sdk/lonely.py":                "",
		"sdk/python/pyproject.toml":        "[project]\nname = \"sdk-python\"\n",
		"sdk/python/sdkpy/__init__.py":     "import sdk.lonely\n",
		"sdk/python/sdkpy/unreferenced.py": "",
	})
	if !deadNamed(fs, "sdk.lonely") {
		t.Errorf("a nested member's import kept the enclosing member's module alive: %+v", byRule(fs, "dead-modules"))
	}
	count := 0
	for _, f := range byRule(fs, "dead-modules") {
		if f.Target.File == "sdk/python/sdkpy/unreferenced.py" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("a nested member's module must be one candidate, its own member's; reported %d times: %+v", count, byRule(fs, "dead-modules"))
	}
}

// tsPackage is a TypeScript member whose package.json points at build
// output while the sources live under src/.
func tsPackage(tsconfig string, extra map[string]string) map[string]string {
	files := map[string]string{
		"package.json":       `{"name": "widget", "main": "./dist/index.js", "exports": {".": "./dist/index.js"}, "bin": {"widget": "dist/cli.js"}}`,
		"src/index.ts":       "import { helper } from './util/helper.js';\nexport const api = helper;\n",
		"src/cli.ts":         "import { api } from './index.js';\nconsole.log(api);\n",
		"src/util/helper.ts": "export const helper = 1;\n",
		"src/orphan.ts":      "export const nobody = 1;\n",
	}
	if tsconfig != "" {
		files["tsconfig.json"] = tsconfig
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

// Lesson 44: a TypeScript package whose package.json names build output
// reaches its sources through tsconfig's outDir and rootDir, and the build
// output is never walked as source.
func TestLesson44TypeScriptEntryPointsInBuildOutputMapToSources(t *testing.T) {
	tsconfig := `{
  // JSON with comments, as tsc reads it
  "compilerOptions": {
    "outDir": "./dist/",
    "rootDir": "./src", /* the sources */
  },
}`
	fs := analyze(t, tsPackage(tsconfig, map[string]string{
		".gitignore":    "",
		"dist/index.js": "export const api = 1;\n",
		"dist/stale.js": "export const old = 1;\n",
	}))
	dead := byRule(fs, "dead-modules")
	if len(dead) != 1 || !strings.Contains(dead[0].Message, "src/orphan") {
		t.Fatalf("want only src/orphan reported dead: %+v", dead)
	}
}

func TestLesson44RootDirFromASingleIncludeDirectory(t *testing.T) {
	fs := analyze(t, tsPackage(`{"compilerOptions": {"outDir": "dist"}, "include": ["src/**/*", "src/types.d.ts"]}`, nil))
	dead := byRule(fs, "dead-modules")
	if len(dead) != 1 || !strings.Contains(dead[0].Message, "src/orphan") {
		t.Fatalf("want only src/orphan reported dead: %+v", dead)
	}
}

func TestLesson44WithoutARootDirReachabilityAbstains(t *testing.T) {
	for name, tsconfig := range map[string]string{
		"no tsconfig":            "",
		"no rootDir, no include": `{"compilerOptions": {"outDir": "dist"}}`,
		"include in two places":  `{"compilerOptions": {"outDir": "dist"}, "include": ["src", "lib"]}`,
	} {
		if got := byRule(analyze(t, tsPackage(tsconfig, nil)), "dead-modules"); len(got) != 0 {
			t.Errorf("%s: an unmappable build-output entry point reported sources dead: %+v", name, got)
		}
	}
}

// Lesson 45: a src-layout Python package's modules are named by their
// import path (pkg.app), not by their file path (src.pkg.app), so a module
// imported only from inside the package is alive.
func TestLesson45SrcLayoutModulesAreNamedByImportPath(t *testing.T) {
	fs := analyze(t, map[string]string{
		"pyproject.toml":         "[project]\nname = \"widget\"\n\n[tool.hatch.build.targets.wheel]\npackages = [\"src/widget\"]\n",
		"src/widget/__init__.py": "from widget.app import run\n",
		"src/widget/app.py":      "from widget import util\n\ndef run():\n    return util.x\n",
		"src/widget/util.py":     "x = 1\n",
	})
	for _, f := range byRule(fs, "dead-modules") {
		if strings.Contains(f.Message, "src.widget") {
			t.Errorf("module named by its file path: %+v", f)
		}
		if strings.Contains(f.Message, "widget.app") || strings.Contains(f.Message, "widget.util") {
			t.Errorf("a module imported inside its package was reported dead: %+v", f)
		}
	}
}
