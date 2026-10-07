package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
	"github.com/smm-h/strictcode/internal/vocab"
)

func TestSingleProjectMode(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		"pyproject.toml":       "[project]\nname = \"solo\"\ndependencies = [\"requests>=2\"]\n",
		"src/solo/__init__.py": "",
	})
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !ws.Single {
		t.Fatal("expected single-project mode")
	}
	if len(ws.Members) != 1 || ws.Members[0].Name != "_" || ws.Members[0].Path != "." {
		t.Fatalf("unexpected members: %+v", ws.Members[0])
	}
	mf := ws.Members[0].Manifests[vocab.LangPy]
	if mf == nil || mf.Name != "solo" {
		t.Fatalf("pyproject not loaded: %+v", mf)
	}
	if len(mf.Deps) != 1 || mf.Deps[0].Name != "requests" || mf.Deps[0].Scope != ScopeRuntime {
		t.Fatalf("deps: %+v", mf.Deps)
	}
}

func TestWorkspaceMembers(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		DeclarationsFile: `format_version = 1
repository_layout = "workspace"
release_branches = ["main"]

[[releasables]]
name = "core-rel"
tag_format = "core-v{version}"
publish_mode = "ci"

[[members]]
path = "."
name = "root"
releasable = false
dev_only = true

[[members]]
path = "core"
name = "core"
library = true
releasable = "core-rel"
import_name = "core_lib"
lint_allow = ["click"]

[[members.pipelines]]
name = "core-pypi"
type = "pypi"
target = "pypi"
local = false
artifact = "package"

[[members]]
path = "tools"
name = "tools"
dev_only = true
releasable = false

[[members]]
path = "legacy"
name = "legacy"
releasable = false
`,
		"core/pyproject.toml": `[project]
name = "orxtra-core"
dependencies = ["orxtra-transport>=0.1", "requests"]

[project.optional-dependencies]
speed = ["orjson"]

[project.scripts]
core-cli = "core.main:run"

[dependency-groups]
dev = ["pytest>=8"]
`,
		"tools/package.json": `{
  "name": "@x/tools",
  "main": "./lib/index.js",
  "bin": {"toolsit": "./bin/run.js"},
  "exports": {".": {"import": "./lib/index.mjs", "require": "./lib/index.cjs"}},
  "dependencies": {"commander": "^12"},
  "devDependencies": {"vitest": "^2"},
  "peerDependencies": {"react": "^19"}
}
`,
		"legacy/go.mod": "module example.com/legacy\n\ngo 1.22\n\nrequire (\n\texample.com/core v1.0.0\n\tgithub.com/pkg/errors v0.9.1\n)\n",
	})
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Single {
		t.Fatal("not single mode")
	}
	if len(ws.Members) != 4 {
		t.Fatalf("members: %d", len(ws.Members))
	}
	if ws.Layout != "workspace" {
		t.Fatalf("layout %q", ws.Layout)
	}

	core := ws.MemberByName("core")
	if core == nil || !core.Library || !core.Published || core.DevOnly {
		t.Fatalf("core flags wrong: %+v", core)
	}
	if core.Path != "core" {
		t.Fatalf("core path %q", core.Path)
	}
	if core.ImportName != "core_lib" || len(core.LintAllow) != 1 || core.LintAllow[0] != "click" {
		t.Fatalf("core overrides wrong: %+v", core)
	}
	py := core.Manifests[vocab.LangPy]
	if py == nil {
		t.Fatal("core pyproject missing")
	}
	if core.RegistryName(vocab.LangPy) != "orxtra-core" {
		t.Fatalf("registry name: %q", core.RegistryName(vocab.LangPy))
	}
	wantDeps := map[string]DepScope{
		"orxtra-transport": ScopeRuntime,
		"requests":         ScopeRuntime,
		"orjson":           ScopePeer,
		"pytest":           ScopeDev,
	}
	if len(py.Deps) != len(wantDeps) {
		t.Fatalf("core deps: %+v", py.Deps)
	}
	for _, d := range py.Deps {
		if wantDeps[d.Name] != d.Scope {
			t.Errorf("dep %s scope %s, want %s", d.Name, d.Scope, wantDeps[d.Name])
		}
	}
	if len(py.EntryPoints) != 1 || py.EntryPoints[0].Form != "script" ||
		py.EntryPoints[0].Name != "core-cli" || py.EntryPoints[0].Target != "core.main:run" {
		t.Fatalf("core entry points: %+v", py.EntryPoints)
	}

	tools := ws.MemberByName("tools")
	if tools == nil || !tools.DevOnly || tools.Published {
		t.Fatalf("tools flags wrong: %+v", tools)
	}
	ts := tools.Manifests[vocab.LangTS]
	if ts == nil || ts.Name != "@x/tools" {
		t.Fatalf("tools package.json: %+v", ts)
	}
	forms := map[string]int{}
	for _, ep := range ts.EntryPoints {
		forms[ep.Form]++
	}
	// main + 2 export leaves + 1 bin.
	if forms["export"] != 3 || forms["bin"] != 1 {
		t.Fatalf("tools entry points: %+v", ts.EntryPoints)
	}
	scopes := map[string]DepScope{}
	for _, d := range ts.Deps {
		scopes[d.Name] = d.Scope
	}
	if scopes["commander"] != ScopeRuntime || scopes["vitest"] != ScopeDev || scopes["react"] != ScopePeer {
		t.Fatalf("tools dep scopes: %+v", scopes)
	}

	legacy := ws.MemberByName("legacy")
	if legacy == nil || legacy.DevOnly || legacy.Published {
		t.Fatalf("legacy flags wrong: %+v", legacy)
	}
	gomod := legacy.Manifests[vocab.LangGo]
	if gomod == nil || gomod.GoModulePath != "example.com/legacy" {
		t.Fatalf("legacy go.mod: %+v", gomod)
	}
	if len(gomod.Deps) != 2 || gomod.Deps[0].Scope != ScopeRuntime {
		t.Fatalf("legacy deps: %+v", gomod.Deps)
	}
}

// decl renders a releasables.toml with the given layout and member tables.
// Every releasable a member names is declared, publishing nothing.
func decl(layout string, members ...string) string {
	return declWith(layout, "", members...)
}

// declWith renders a releasables.toml with the given layout, [[releasables]]
// tables, and member tables; an empty releasables argument declares one
// releasable publishing nothing for each name a member gives.
func declWith(layout, releasables string, members ...string) string {
	out := "format_version = 1\nrepository_layout = \"" + layout + "\"\nrelease_branches = [\"main\"]\n"
	if releasables == "" {
		seen := map[string]bool{}
		for _, m := range members {
			for _, line := range strings.Split(m, "\n") {
				name, ok := strings.CutPrefix(line, "releasable = \"")
				if !ok || seen[name] {
					continue
				}
				seen[name] = true
				name = strings.TrimSuffix(name, "\"")
				releasables += "\n[[releasables]]\nname = \"" + name + "\"\ntag_format = \"v{version}\"\npublish_mode = \"none\"\n"
			}
		}
	}
	out += releasables
	for _, m := range members {
		out += "\n[[members]]\n" + m
	}
	return out
}

func TestMalformedWorkspaceIsHardError(t *testing.T) {
	cases := map[string]string{
		"bad-toml":                  "[[members]\nname=",
		"no-format-version":         "repository_layout = \"workspace\"\n\n[[members]]\npath = \".\"\nname = \"root\"\nreleasable = false\n",
		"no-layout":                 "format_version = 1\n\n[[members]]\npath = \".\"\nname = \"root\"\nreleasable = false\n",
		"unknown-layout":            decl("monorepo", "path = \".\"\nname = \"root\"\nreleasable = false\n"),
		"member-without-name":       decl("workspace", "path = \"x\"\nreleasable = false\n"),
		"member-without-path":       decl("workspace", "name = \"x\"\nreleasable = false\n"),
		"member-without-releasable": decl("workspace", "path = \"x\"\nname = \"x\"\n"),
		"releasable-true":           decl("workspace", "path = \"x\"\nname = \"x\"\nreleasable = true\n"),
		"non-canonical-path":        decl("workspace", "path = \"core/\"\nname = \"core\"\nreleasable = false\n"),
		"climbing-path":             decl("workspace", "path = \"../x\"\nname = \"x\"\nreleasable = false\n"),
		"duplicate-names":           decl("workspace", "name = \"x\"\npath = \"a\"\nreleasable = false\n", "name = \"x\"\npath = \"b\"\nreleasable = false\n"),
		"duplicate-paths":           decl("workspace", "name = \"x\"\npath = \"a\"\nreleasable = false\n", "name = \"y\"\npath = \"a\"\nreleasable = false\n"),
		"no-members":                decl("workspace"),
		"standalone-two-members":    decl("standalone", "name = \"root\"\npath = \".\"\nreleasable = \"r\"\n", "name = \"x\"\npath = \"x\"\nreleasable = false\n"),
		"standalone-not-at-root":    decl("standalone", "name = \"x\"\npath = \"x\"\nreleasable = \"r\"\n"),
		"undeclared-releasable":     declWith("workspace", "\n", "name = \"x\"\npath = \"x\"\nreleasable = \"r\"\n"),
		"releasable-without-mode":   declWith("workspace", "\n[[releasables]]\nname = \"r\"\n", "name = \"x\"\npath = \"x\"\nreleasable = \"r\"\n"),
		"releasable-without-name":   declWith("workspace", "\n[[releasables]]\npublish_mode = \"ci\"\n", "name = \"x\"\npath = \"x\"\nreleasable = false\n"),
		"duplicate-releasables":     declWith("workspace", "\n[[releasables]]\nname = \"r\"\npublish_mode = \"ci\"\n\n[[releasables]]\nname = \"r\"\npublish_mode = \"none\"\n", "name = \"x\"\npath = \"x\"\nreleasable = \"r\"\n"),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			root := fixture.Write(t, map[string]string{DeclarationsFile: content})
			if _, err := Load(root); err == nil {
				t.Fatal("malformed declarations accepted")
			}
		})
	}
}

func TestStandaloneLayoutReadsTheRootMember(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		DeclarationsFile: decl("standalone", "path = \".\"\nname = \"root\"\nreleasable = \"solo\"\n"),
		"pyproject.toml": "[project]\nname = \"solo\"\n",
	})
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Single || ws.Layout != "standalone" {
		t.Fatalf("standalone declarations read as %+v", ws)
	}
	if len(ws.Members) != 1 || ws.Members[0].Name != "root" || ws.Members[0].Published {
		t.Fatalf("members: %+v", ws.Members)
	}
}

// The old layout's workspace file is refused, naming the migration; once the
// migration has replaced it with the declarations file, the workspace loads.
func TestOldLayoutIsRefusedUntilMigrated(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"a\"\nname = \"a\"\n",
		"a/pyproject.toml":               "[project]\nname = \"a\"\n",
	})
	_, err := Load(root)
	if err == nil {
		t.Fatal("old layout accepted")
	}
	for _, want := range []string{".rlsbl-monorepo/workspace.toml", DeclarationsFile, "rlsbl migrate records"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q: %v", want, err)
		}
	}

	// What the migration does: the old file goes, the declarations arrive.
	if err := os.RemoveAll(filepath.Join(root, ".rlsbl-monorepo")); err != nil {
		t.Fatal(err)
	}
	declPath := filepath.Join(root, filepath.FromSlash(DeclarationsFile))
	if err := os.MkdirAll(filepath.Dir(declPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := decl("workspace",
		"path = \".\"\nname = \"root\"\nreleasable = false\ndev_only = true\n",
		"path = \"a\"\nname = \"a\"\nreleasable = \"a\"\n")
	if err := os.WriteFile(declPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := Load(root)
	if err != nil {
		t.Fatalf("migrated repository refused: %v", err)
	}
	if ws.MemberByName("a") == nil {
		t.Fatalf("member a missing: %+v", ws.Members)
	}
}

func TestOwnerIsTheLongestContainingMember(t *testing.T) {
	ws := &Workspace{Members: []*Member{{Name: "root", Path: "."}, {Name: "a", Path: "a"}, {Name: "ab", Path: "a/b"}}}
	cases := map[string]string{".": "root", "x": "root", "a": "a", "a/c": "a", "a/b": "ab", "a/b/c": "ab", "ab": "root"}
	for path, want := range cases {
		if got := ws.Owner(path); got == nil || got.Name != want {
			t.Errorf("Owner(%q) = %+v, want %s", path, got, want)
		}
	}
}

func TestDepScopeOptional(t *testing.T) {
	if ScopeRuntime.Optional() || ScopeExplicit.Optional() {
		t.Fatal("hard scopes reported optional")
	}
	if !ScopeDev.Optional() || !ScopePeer.Optional() {
		t.Fatal("optional scopes reported hard")
	}
}

func TestParseRequirement(t *testing.T) {
	cases := []struct {
		req, name, constraint string
		source                DepSource
	}{
		{"requests>=2", "requests", ">=2", SourceRegistry},
		{"my-lib[extra,fast] >= 1.0 ; python_version < '3.12'", "my-lib", ">= 1.0", SourceRegistry},
		{"foo (>=1.0)", "foo", ">=1.0", SourceRegistry},
		{"bare", "bare", "", SourceRegistry},
		{"foo @ file:///work/foo", "foo", "file:///work/foo", SourcePath},
		{"foo[x] @ {root:uri}/foo ; os_name == 'posix'", "foo", "{root:uri}/foo", SourcePath},
		{"", "", "", ""},
	}
	for _, c := range cases {
		name, source, constraint := ParseRequirement(c.req)
		if name != c.name || source != c.source || constraint != c.constraint {
			t.Errorf("ParseRequirement(%q) = (%q, %q, %q), want (%q, %q, %q)",
				c.req, name, source, constraint, c.name, c.source, c.constraint)
		}
	}
}

func TestManifestVersionsAndDependencySources(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		DeclarationsFile: decl("workspace",
			"path = \"py\"\nname = \"py\"\nreleasable = false\n",
			"path = \"js\"\nname = \"js\"\nreleasable = false\n",
			"path = \"go\"\nname = \"go\"\nreleasable = false\n"),
		"py/pyproject.toml": "[project]\nname = \"py\"\nversion = \"1.2.3\"\ndependencies = [\"lib>=1\"]\n",
		"js/package.json":   `{"name": "js", "version": "0.4.0", "dependencies": {"a": "^1.0.0", "b": "workspace:*", "c": "file:../c"}}`,
		"go/go.mod":         "module example.com/go\n\ngo 1.22\n\nrequire example.com/lib v1.4.0\n",
	})
	ws, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	py := ws.MemberByName("py").Manifests[vocab.LangPy]
	if py.Version != "1.2.3" || py.Deps[0].Constraint != ">=1" || py.Deps[0].Source != SourceRegistry {
		t.Errorf("pyproject: %+v", py)
	}
	js := ws.MemberByName("js").Manifests[vocab.LangTS]
	if js.Version != "0.4.0" {
		t.Errorf("package.json version %q", js.Version)
	}
	sources := map[string]DepSource{}
	for _, d := range js.Deps {
		sources[d.Name] = d.Source
	}
	if sources["a"] != SourceRegistry || sources["b"] != SourceWorkspace || sources["c"] != SourcePath {
		t.Errorf("package.json sources: %+v", sources)
	}
	gomod := ws.MemberByName("go").Manifests[vocab.LangGo]
	if gomod.Version != "" || gomod.Deps[0].Constraint != "v1.4.0" {
		t.Errorf("go.mod: %+v", gomod)
	}
}
