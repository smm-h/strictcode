package workspace

import (
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
	"github.com/smm-h/strictcode/internal/vocab"
)

func TestSingleProjectMode(t *testing.T) {
	root := fixture.Write(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"solo\"\ndependencies = [\"requests>=2\"]\n",
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
		".rlsbl-monorepo/workspace.toml": `
[[releasables]]
name = "core-rel"

[[projects]]
path = "core/"
name = "core"
library = true
releasable = "core-rel"
import_name = "core_lib"
lint_allow = ["click"]

[[projects]]
path = "tools"
name = "tools"
dev_only = true
releasable = false

[[projects]]
path = "legacy"
name = "legacy"
dev_node = true
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
	if len(ws.Members) != 3 {
		t.Fatalf("members: %d", len(ws.Members))
	}

	core := ws.MemberByName("core")
	if core == nil || !core.Library || !core.Releasable || core.DevOnly {
		t.Fatalf("core flags wrong: %+v", core)
	}
	if core.Path != "core" {
		t.Fatalf("core path %q (trailing slash must be trimmed)", core.Path)
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
	if tools == nil || !tools.DevOnly || tools.Releasable {
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
	if legacy == nil || !legacy.DevOnly {
		t.Fatal("legacy dev_node not read as dev marker")
	}
	gomod := legacy.Manifests[vocab.LangGo]
	if gomod == nil || gomod.GoModulePath != "example.com/legacy" {
		t.Fatalf("legacy go.mod: %+v", gomod)
	}
	if len(gomod.Deps) != 2 || gomod.Deps[0].Scope != ScopeRuntime {
		t.Fatalf("legacy deps: %+v", gomod.Deps)
	}
}

func TestMalformedWorkspaceIsHardError(t *testing.T) {
	cases := map[string]map[string]string{
		"bad-toml": {
			".rlsbl-monorepo/workspace.toml": "[[projects]\nname=",
		},
		"project-without-name": {
			".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"x\"\n",
		},
		"project-without-path": {
			".rlsbl-monorepo/workspace.toml": "[[projects]]\nname = \"x\"\n",
		},
		"duplicate-names": {
			".rlsbl-monorepo/workspace.toml": "[[projects]]\nname = \"x\"\npath = \"a\"\n\n[[projects]]\nname = \"x\"\npath = \"b\"\n",
		},
		"no-projects": {
			".rlsbl-monorepo/workspace.toml": "# empty\n",
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			root := fixture.Write(t, files)
			if _, err := Load(root); err == nil {
				t.Fatal("malformed workspace accepted")
			}
		})
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
