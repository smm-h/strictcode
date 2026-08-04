package extract

import (
	"fmt"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// run extracts a fixture workspace.
func run(t *testing.T, files map[string]string) *Result {
	t.Helper()
	root := fixture.Write(t, files)
	ws, err := workspace.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Extract(ws)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// rowSet renders rows of one kind as "src -> dst [attrs]" strings.
func rowSet(res *Result, kind vocab.RowKind) map[string]relation.Row {
	out := map[string]relation.Row{}
	for _, r := range res.Relation.Rows {
		if r.Kind != kind {
			continue
		}
		key := r.Src.String() + " -> " + r.Dst.String()
		out[key] = r
	}
	return out
}

func nodeSet(res *Result, kind vocab.NodeKind) map[string]relation.Node {
	out := map[string]relation.Node{}
	for _, n := range res.Relation.Nodes {
		if n.Kind == kind {
			out[n.ID.String()] = n
		}
	}
	return out
}

func boolAttr(t *testing.T, r relation.Row, name string) bool {
	t.Helper()
	v, ok := r.Attrs[name].AsBool()
	if !ok {
		t.Fatalf("row missing bool attr %s", name)
	}
	return v
}

// --- Python ---------------------------------------------------------------

func pyWorkspace() map[string]string {
	return map[string]string{
		".rlsbl-monorepo/workspace.toml": `
[[projects]]
path = "core"
name = "core"
library = true
releasable = "core"

[[projects]]
path = "transport"
name = "transport"
`,
		"core/pyproject.toml": `[project]
name = "orxtra-core"
dependencies = ["orxtra-transport>=0.1", "requests"]

[project.scripts]
core-cli = "core_lib.main:run"
`,
		// src layout, package name differs from member name via import_name?
		// core's package is core_lib.
		"core/src/core_lib/__init__.py": "from . import main\n__all__ = [\"util\"]\n",
		"core/src/core_lib/main.py":     "import os\nfrom . import util\nfrom orxt.transport import client\n",
		"core/src/core_lib/util.py":     "import json\n",
		"core/src/core_lib/legacy.py":   "# imported by nobody\n",
		"core/tests/test_main.py":       "import core_lib.main\nimport orxtra_transport\n",
		"core/scripts/build.py":         "import core_lib.legacy\n",
		// transport: namespace package orxt/transport.
		"transport/pyproject.toml": `[project]
name = "orxtra-transport"
dependencies = []
`,
		"transport/src/orxt/transport/__init__.py": "",
		"transport/src/orxt/transport/client.py": `try:
    import orjson
except ImportError:
    import json as orjson

from typing import TYPE_CHECKING
if TYPE_CHECKING:
    import orxtra_core
`,
	}
}

func TestPyModuleEnumeration(t *testing.T) {
	res := run(t, pyWorkspace())
	mods := nodeSet(res, vocab.NodeKindModule)
	for _, want := range []string{
		"py:core:core_lib:",
		"py:core:core_lib%2Emain:",
		"py:core:core_lib%2Eutil:",
		"py:core:core_lib%2Elegacy:",
		"py:core:tests%2Etest_main:",
		"py:core:scripts%2Ebuild:",
		"py:transport:orxt%2Etransport:",
		"py:transport:orxt%2Etransport%2Eclient:",
	} {
		if _, ok := mods[want]; !ok {
			t.Errorf("missing module node %s (have %v)", want, keys(mods))
		}
	}
	// Test context: tests/ yes, src/ no (lesson 6/8).
	test := mods["py:core:tests%2Etest_main:"]
	if v, _ := test.Attrs["test_context"].AsBool(); !v {
		t.Error("tests/test_main.py must be test context")
	}
	main := mods["py:core:core_lib%2Emain:"]
	if v, _ := main.Attrs["test_context"].AsBool(); v {
		t.Error("src module wrongly test context")
	}
}

func TestPyIntraMemberImports(t *testing.T) {
	res := run(t, pyWorkspace())
	imports := rowSet(res, vocab.RowKindImports)
	// Relative import from . import util in main.py.
	if _, ok := imports["py:core:core_lib%2Emain: -> py:core:core_lib%2Eutil:"]; !ok {
		t.Errorf("relative import not resolved to module: %v", keys(imports))
	}
	// __init__ imports main via from . import main.
	if _, ok := imports["py:core:core_lib: -> py:core:core_lib%2Emain:"]; !ok {
		t.Error("__init__ relative import row missing")
	}
}

func TestPyMemberResolution(t *testing.T) {
	res := run(t, pyWorkspace())
	imports := rowSet(res, vocab.RowKindImports)

	// Namespace-map resolution (lesson 11): from orxt.transport import
	// client in core -> member transport.
	key := "py:core:core_lib%2Emain: -> py:transport:_:"
	if _, ok := imports[key]; !ok {
		t.Fatalf("namespace import did not resolve to member: %v", keys(imports))
	}
	// Normalized registry-name resolution (lesson 10): import
	// orxtra_transport in tests -> member transport, test_context=true.
	testKey := "py:core:tests%2Etest_main: -> py:transport:_:"
	r, ok := imports[testKey]
	if !ok {
		t.Fatalf("registry-name import did not resolve: %v", keys(imports))
	}
	if !boolAttr(t, r, "test_context") {
		t.Error("test-file import must carry test_context=true")
	}
}

func TestPyGuardedAndTypeChecking(t *testing.T) {
	res := run(t, pyWorkspace())
	imports := rowSet(res, vocab.RowKindImports)

	// try-body import of orjson is guarded... orjson is external, so no
	// member row; check the TYPE_CHECKING import of orxtra_core -> core.
	key := "py:transport:orxt%2Etransport%2Eclient: -> py:core:_:"
	r, ok := imports[key]
	if !ok {
		t.Fatalf("TYPE_CHECKING import row missing: %v", keys(imports))
	}
	if !boolAttr(t, r, "type_checking") {
		t.Error("if TYPE_CHECKING import must carry type_checking=true (lesson 5)")
	}
}

func TestPyGuardedClassification(t *testing.T) {
	res := run(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"a\"\nname = \"a\"\n\n[[projects]]\npath = \"b\"\nname = \"b\"\n",
		"a/pyproject.toml":               "[project]\nname = \"a\"\ndependencies = [\"b\"]\n",
		"a/a_pkg/__init__.py":            "",
		"a/a_pkg/guarded.py": `try:
    import b
except ImportError:
    b = None
`,
		"a/a_pkg/fallback.py": `try:
    import fastjson
except (ValueError, ModuleNotFoundError):
    import b
`,
		"a/a_pkg/qualified.py": `try:
    import b
except builtins.ImportError as exc:
    pass
`,
		"a/a_pkg/unrelated.py": `try:
    import b
except KeyError:
    pass
`,
		"b/pyproject.toml": "[project]\nname = \"b\"\n",
		"b/b/__init__.py":  "",
	})
	imports := rowSet(res, vocab.RowKindImports)

	get := func(mod string) relation.Row {
		key := fmt.Sprintf("py:a:a_pkg%%2E%s: -> py:b:_:", mod)
		r, ok := imports[key]
		if !ok {
			t.Fatalf("no member import row for %s: %v", mod, keys(imports))
		}
		return r
	}
	// Lesson 1: try-body import IS guarded.
	if !boolAttr(t, get("guarded"), "guarded") {
		t.Error("try-body import of b must be guarded")
	}
	// Lesson 3: except-body fallback import is NOT guarded (the tuple form
	// catches ModuleNotFoundError, but the import sits in the except body).
	if boolAttr(t, get("fallback"), "guarded") {
		t.Error("except-body fallback import must NOT be guarded")
	}
	// Qualified builtins.ImportError with as-binding counts.
	if !boolAttr(t, get("qualified"), "guarded") {
		t.Error("qualified/as-bound except ImportError must guard")
	}
	// A try/except over an unrelated exception does not guard.
	if boolAttr(t, get("unrelated"), "guarded") {
		t.Error("except KeyError must not guard")
	}
}

func TestPyExports(t *testing.T) {
	res := run(t, pyWorkspace())
	exports := rowSet(res, vocab.RowKindExports)
	// __init__.py: from . import main (relative_import form) and __all__ =
	// ["util"] (declared_all form).
	relRow, ok := exports["py:core:core_lib: -> py:core:core_lib%2Emain:"]
	if !ok {
		t.Fatalf("relative-import export row missing: %v", keys(exports))
	}
	if form, _ := relRow.Attrs["form"].AsString(); form != "relative_import" {
		t.Errorf("form = %s", form)
	}
	allRow, ok := exports["py:core:core_lib: -> py:core:core_lib%2Eutil:"]
	if !ok {
		t.Fatalf("__all__ export row missing: %v", keys(exports))
	}
	if form, _ := allRow.Attrs["form"].AsString(); form != "declared_all" {
		t.Errorf("form = %s", form)
	}
}

func TestPyEntryPoints(t *testing.T) {
	res := run(t, pyWorkspace())
	eps := nodeSet(res, vocab.NodeKindEntryPoint)
	epID := "py:core:_:script/core-cli"
	if _, ok := eps[epID]; !ok {
		t.Fatalf("entry point node missing: %v", keys(eps))
	}
	resolves := rowSet(res, vocab.RowKindResolvesTo)
	if _, ok := resolves[epID+" -> py:core:core_lib%2Emain:"]; !ok {
		t.Fatalf("resolves_to row missing: %v", keys(resolves))
	}
}

func TestPyDeclaredDeps(t *testing.T) {
	res := run(t, pyWorkspace())
	deps := rowSet(res, vocab.RowKindDeclaresDependency)
	r, ok := deps["py:core:_: -> py:transport:_:"]
	if !ok {
		t.Fatalf("declared dep row missing: %v", keys(deps))
	}
	if scope, _ := r.Attrs["scope"].AsString(); scope != "runtime" {
		t.Errorf("scope = %s", scope)
	}
	// requests is external: exactly one declared edge.
	if len(deps) != 1 {
		t.Errorf("expected 1 declared dep row, got %v", keys(deps))
	}
}

func TestPySiblingPruning(t *testing.T) {
	// Lesson 13: member with path = "." must not ingest nested sibling.
	res := run(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \".\"\nname = \"root\"\n\n[[projects]]\npath = \"sub\"\nname = \"sub\"\n",
		"pyproject.toml":                 "[project]\nname = \"root\"\n",
		"rootpkg/__init__.py":            "",
		"sub/pyproject.toml":             "[project]\nname = \"sub\"\n",
		"sub/subpkg/__init__.py":         "import sub_only_dep\n",
	})
	mods := nodeSet(res, vocab.NodeKindModule)
	if _, ok := mods["py:root:sub%2Esubpkg:"]; ok {
		t.Fatal("root member ingested nested sibling's source")
	}
	if _, ok := mods["py:sub:subpkg:"]; !ok {
		t.Fatalf("sub member's own module missing: %v", keys(mods))
	}
}

// --- Go -------------------------------------------------------------------

func goWorkspace() map[string]string {
	return map[string]string{
		".rlsbl-monorepo/workspace.toml": `
[[projects]]
path = "lib"
name = "lib"
library = true

[[projects]]
path = "app"
name = "app"
`,
		"lib/go.mod":                    "module example.com/lib\n\ngo 1.22\n",
		"lib/lib.go":                    "package lib\n\nimport \"example.com/lib/internal/parser\"\n\nvar _ = parser.X\n",
		"lib/internal/parser/p.go":      "package parser\n\nvar X = 1\n",
		"lib/internal/unused/u.go":      "package unused\n",
		"lib/internal/parser/p_test.go": "package parser\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n",
		"app/go.mod":                    "module example.com/app\n\ngo 1.22\n\nrequire example.com/lib v0.1.0\n",
		"app/main.go":                   "package main\n\nimport (\n\t\"fmt\"\n\t\"example.com/lib\"\n)\n\nfunc main() { fmt.Println(lib.V) }\n",
	}
}

func TestGoPackagesAndImports(t *testing.T) {
	res := run(t, goWorkspace())
	mods := nodeSet(res, vocab.NodeKindModule)
	for _, want := range []string{
		"go:lib:%2E:", // root package "."
		"go:lib:internal/parser:",
		"go:lib:internal/unused:",
		"go:app:%2E:",
	} {
		if _, ok := mods[want]; !ok {
			t.Errorf("missing package node %s (have %v)", want, keys(mods))
		}
	}
	imports := rowSet(res, vocab.RowKindImports)
	// Intra-member package import.
	if _, ok := imports["go:lib:%2E: -> go:lib:internal/parser:"]; !ok {
		t.Errorf("intra-member package import missing: %v", keys(imports))
	}
	// Cross-member import by module path.
	if _, ok := imports["go:app:%2E: -> go:lib:_:"]; !ok {
		t.Errorf("cross-member import missing: %v", keys(imports))
	}
}

func TestGoTestOnlyPackage(t *testing.T) {
	res := run(t, map[string]string{
		"go.mod":                  "module example.com/solo\n\ngo 1.22\n",
		"internal/only/x_test.go": "package only\n",
		"internal/real/r.go":      "package real\n",
		"testdata/gen/gen.go":     "package gen\n",
	})
	mods := nodeSet(res, vocab.NodeKindModule)
	onlyTest := mods["go:_:internal/only:"]
	if v, _ := onlyTest.Attrs["test_context"].AsBool(); !v {
		t.Error("package with only _test.go files must be test context (lesson 9)")
	}
	td := mods["go:_:testdata/gen:"]
	if v, _ := td.Attrs["test_context"].AsBool(); !v {
		t.Error("testdata package must be test context (lesson 9)")
	}
	real := mods["go:_:internal/real:"]
	if v, _ := real.Attrs["test_context"].AsBool(); v {
		t.Error("real package wrongly test context")
	}
}

func TestGoMainEntryPoint(t *testing.T) {
	res := run(t, goWorkspace())
	eps := nodeSet(res, vocab.NodeKindEntryPoint)
	epID := "go:app:_:main_package/%2E"
	if _, ok := eps[epID]; !ok {
		t.Fatalf("main entry point missing: %v", keys(eps))
	}
	resolves := rowSet(res, vocab.RowKindResolvesTo)
	if _, ok := resolves[epID+" -> go:app:%2E:"]; !ok {
		t.Fatalf("resolves_to missing: %v", keys(resolves))
	}
}

func TestGoDeclaredDeps(t *testing.T) {
	res := run(t, goWorkspace())
	deps := rowSet(res, vocab.RowKindDeclaresDependency)
	if _, ok := deps["go:app:_: -> go:lib:_:"]; !ok {
		t.Fatalf("go declared dep missing: %v", keys(deps))
	}
}

// --- TS/JS ----------------------------------------------------------------

func tsWorkspace() map[string]string {
	return map[string]string{
		".rlsbl-monorepo/workspace.toml": `
[[projects]]
path = "ui"
name = "ui"

[[projects]]
path = "sdk"
name = "sdk"
library = true
`,
		"ui/package.json": `{
  "name": "@x/ui",
  "main": "./src/index.ts",
  "dependencies": {"@x/sdk": "^1"}
}
`,
		"ui/src/index.ts":            "import { api } from './api';\nimport type { Cfg } from './types';\nexport { helper } from './util/helper.js';\nimport '@x/sdk';\n",
		"ui/src/api.ts":              "import fs from 'node:fs';\nexport const api = 1;\n",
		"ui/src/types.ts":            "export interface Cfg {}\n",
		"ui/src/util/helper.ts":      "export const helper = () => require('../dead-maybe');\n",
		"ui/src/dead-maybe/index.ts": "export default 1;\n",
		"ui/src/orphan.ts":           "export const nobody = 1;\n",
		"ui/pkg/__init__.py":         "",
		"ui/pkg/embedded.js":         "import './never-scanned';\n",
		"sdk/package.json":           "{\n  \"name\": \"@x/sdk\",\n  \"main\": \"./index.ts\"\n}\n",
		"sdk/index.ts":               "export const sdk = 1;\n",
	}
}

func TestTSModulesAndResolution(t *testing.T) {
	res := run(t, tsWorkspace())
	mods := nodeSet(res, vocab.NodeKindModule)
	for _, want := range []string{
		"ts:ui:src:", // src/index.ts collapses to src
		"ts:ui:src/api:",
		"ts:ui:src/types:",
		"ts:ui:src/util/helper:",
		"ts:ui:src/dead-maybe:", // index collapse
		"ts:ui:src/orphan:",
		"ts:sdk:%2E:", // root index.ts collapses to "."
	} {
		if _, ok := mods[want]; !ok {
			t.Errorf("missing module %s (have %v)", want, keys(mods))
		}
	}
	// Lesson 17: JS files inside a Python package tree are not modules.
	if _, ok := mods["ts:ui:pkg/embedded:"]; ok {
		t.Error("python-package-embedded JS file wrongly scanned as module")
	}

	imports := rowSet(res, vocab.RowKindImports)
	// Extension probing: './api' -> src/api.ts.
	if _, ok := imports["ts:ui:src: -> ts:ui:src/api:"]; !ok {
		t.Errorf("probe resolution missing: %v", keys(imports))
	}
	// .js -> .ts mapping (lesson 18): './util/helper.js' -> helper.ts.
	if _, ok := imports["ts:ui:src: -> ts:ui:src/util/helper:"]; !ok {
		t.Error("js->ts mapped import missing")
	}
	// require() with directory -> index resolution.
	if _, ok := imports["ts:ui:src/util/helper: -> ts:ui:src/dead-maybe:"]; !ok {
		t.Error("require directory->index resolution missing")
	}
	// Bare scoped specifier -> member.
	if _, ok := imports["ts:ui:src: -> ts:sdk:_:"]; !ok {
		t.Error("bare @x/sdk import did not resolve to member")
	}
	// import type carries type_checking.
	r, ok := imports["ts:ui:src: -> ts:ui:src/types:"]
	if !ok {
		t.Fatal("type import row missing")
	}
	if !boolAttr(t, r, "type_checking") {
		t.Error("import type must carry type_checking=true")
	}
}

func TestTSReexportsAndEntryPoints(t *testing.T) {
	res := run(t, tsWorkspace())
	exports := rowSet(res, vocab.RowKindExports)
	r, ok := exports["ts:ui:src: -> ts:ui:src/util/helper:"]
	if !ok {
		t.Fatalf("reexport row missing: %v", keys(exports))
	}
	if form, _ := r.Attrs["form"].AsString(); form != "reexport" {
		t.Errorf("form = %s", form)
	}

	eps := nodeSet(res, vocab.NodeKindEntryPoint)
	epID := "ts:ui:_:export/main"
	if _, ok := eps[epID]; !ok {
		t.Fatalf("main entry point missing: %v", keys(eps))
	}
	resolves := rowSet(res, vocab.RowKindResolvesTo)
	if _, ok := resolves[epID+" -> ts:ui:src:"]; !ok {
		t.Fatalf("main resolves_to missing: %v", keys(resolves))
	}
}

// --- shared ----------------------------------------------------------------

func TestDeterminism(t *testing.T) {
	// Two extractions of the same fixture produce identical canonical forms.
	files := pyWorkspace()
	h1 := run(t, files).Relation.Hash()
	h2 := run(t, files).Relation.Hash()
	if h1 != h2 {
		t.Fatal("extraction is not deterministic")
	}
}

func TestLineIndex(t *testing.T) {
	res := run(t, pyWorkspace())
	// core/src/core_lib/main.py: line 1 "import os", line 2 "from . import util".
	file := "core/src/core_lib/main.py"
	if got := res.Line(file, 0); got != 1 {
		t.Fatalf("line at 0 = %d", got)
	}
	if got := res.Line(file, 10); got != 2 {
		t.Fatalf("line at 10 = %d", got)
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestExternalImportsSideTable(t *testing.T) {
	res := run(t, pyWorkspace())
	byMember := map[string][]string{}
	for _, e := range res.ExternalImports {
		byMember[e.Member] = append(byMember[e.Member], e.Specifier)
	}
	// core_lib/main.py imports os; util.py imports json — externals of core.
	found := map[string]bool{}
	for _, s := range byMember["core"] {
		found[s] = true
	}
	if !found["os"] || !found["json"] {
		t.Fatalf("stdlib externals not recorded: %v", byMember["core"])
	}
	// transport's guarded orjson import is external with the site recorded.
	guarded := false
	for _, e := range res.ExternalImports {
		if e.Member == "transport" && e.Specifier == "orjson" {
			guarded = true
		}
	}
	if !guarded {
		t.Fatalf("external orjson site missing: %v", byMember["transport"])
	}
}

func TestPyMemberRootIsThePackage(t *testing.T) {
	// Real-corpus regression (selfdoc): the member's path points at the
	// package directory itself (member root contains __init__.py). Modules
	// must get proper dotted logical names anchored at the directory name,
	// and intra-package imports must resolve.
	res := run(t, map[string]string{
		".rlsbl-monorepo/workspace.toml": "[[projects]]\npath = \"selfblog\"\nname = \"selfblog\"\n",
		"selfblog/pyproject.toml":        "[project]\nname = \"selfblog\"\n",
		"selfblog/__init__.py":           "from . import cli\n",
		"selfblog/cli.py":                "import selfblog.posts\n",
		"selfblog/posts.py":              "",
	})
	mods := nodeSet(res, vocab.NodeKindModule)
	for _, want := range []string{
		"py:selfblog:selfblog:",
		"py:selfblog:selfblog%2Ecli:",
		"py:selfblog:selfblog%2Eposts:",
	} {
		if _, ok := mods[want]; !ok {
			t.Errorf("missing module %s (have %v)", want, keys(mods))
		}
	}
	imports := rowSet(res, vocab.RowKindImports)
	if _, ok := imports["py:selfblog:selfblog: -> py:selfblog:selfblog%2Ecli:"]; !ok {
		t.Errorf("root-package relative import unresolved: %v", keys(imports))
	}
	if _, ok := imports["py:selfblog:selfblog%2Ecli: -> py:selfblog:selfblog%2Eposts:"]; !ok {
		t.Errorf("absolute intra-package import unresolved: %v", keys(imports))
	}
}
