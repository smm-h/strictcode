+++
title = "Check semantics"
description = "Semantics the rules share: the boundary with rlsbl, workspace inputs, import resolution per language, test context, dead modules, and library rules."
nav_order = 210
+++

# Check semantics

This page holds the semantics several rules share. Each rule's own page, under the rule
reference, links back here. The [lessons register](../lessons/) turns the edge cases below into
regression tests.

## Origin and boundary with rlsbl

rlsbl, the ecosystem's release orchestrator, is moving to a permanent invariant: **rlsbl never
parses source code.** It works only from declared metadata: manifests, its release declarations
(`.strictmetadata/releasables/releasables.toml`), git tags, and registry APIs. Everything that requires interpreting the contents of
a source file moves to strictcode.

rlsbl's implementation is the behavioral donor, not a mandate. strictcode adopts its check
semantics and its accumulated edge-case knowledge, re-implemented on the graph. Where rlsbl used
regular expressions or line heuristics, strictcode uses the graph, and observable results must be
equal or better.

Deliberate departures from the donor:

- **Languages.** The donor covered Python, Go, npm, Dart, and Java/Kotlin. strictcode implements
  Python, Go, and TypeScript/JavaScript. Dart and JVM support wait for a real need.
- **No regular-expression backends,** not even as a fallback. Everything goes through the graph.
- **One model instead of three subsystems.** The donor's import scanners, dependency-analysis
  engine, and lint engine became extraction that populates the graph, and rules that query it.
- **Native configuration.** The donor's configuration files are replaced by
  [`strictcode.toml`](../config/), keeping their principles: mandatory reasons, hard errors on
  malformed configuration, and staleness detection.
- **Maven and JVM lint delegation** stays with rlsbl as an external check, if rlsbl wants it.

Besides source analysis, strictcode took over the code-quality checks rlsbl ran: the stale
intra-workspace dependency constraints ([`deps-stale`](../rules/deps-stale/)), the Python tools
([`lint`](../rules/lint/), [`format`](../rules/format/), and [`type-check`](../rules/type-check/),
with their scope guards), and the strictspec certificate
([`strictspec-certificate`](../rules/strictspec-certificate/)). rlsbl keeps everything else:
version, name, and license consistency, changelog and release machinery, its test suites, and the
`__version__` bump, which is a targeted write rather than analysis. rlsbl runs strictcode as one of
its checks; see [rlsbl integration](../rlsbl-integration/).

## Workspace and manifest inputs

strictcode reads a workspace's committed files and reconstructs everything it needs from disk;
nothing is passed to it at runtime.

- **rlsbl's release declarations** (`.strictmetadata/releasables/releasables.toml`): the
  repository layout, and each `[[members]]` table's name, path, `library` flag, `import_name`
  override, lint allow list (`lint_allow`), `dev_only` marker, and `releasable` (a releasable name,
  or `false`). The document is rlsbl's, which validates every other key; strictcode refuses a
  document missing what it reads, and a path that is not canonical. rlsbl's old layout,
  `.rlsbl-monorepo/workspace.toml`, is refused, naming `rlsbl migrate records`, which converts it.
- **Manifests**: `pyproject.toml`, `package.json`, and `go.mod` (including nested `go.mod` files
  inside a member, whose requirements count toward the member's declared dependencies, and whose
  module paths resolve the packages under them).
- **Dependency scopes**: pyproject `project.dependencies` is `runtime`,
  `project.optional-dependencies` is `peer`, and `dependency-groups` is `dev`. package.json
  `dependencies`, `devDependencies`, and `peerDependencies` map to their names, and
  `optionalDependencies` is `peer`. go.mod requirements are `runtime`, since Go has no dev scope.
  The `explicit` scope is reserved for workspace-declared edges.
- **Single projects.** Without a declarations file, the project is one member named `_`. It has no
  `library = true` marker, so the library-boundary rules never run on it.

Findings on manifest-declared facts point at the first occurrence of the dependency or entry-point
name in the manifest, so they carry a real `file:line`.

### Source-walk exclusions

A source walk reads only what git lists under the workspace root: tracked files, and untracked
files that are not ignored (`git ls-files --cached --others --exclude-standard`). A gitignored
file, a third-party clone among them, is never read, and a workspace root outside any git work
tree is refused.

Of what git lists, these directories are never scanned at any depth: `.venv`, `venv`,
`__pycache__`, `.git`, `node_modules`, `.tox`, `.mypy_cache`, `.pytest_cache`, `.ruff_cache`,
`.selfdoc`, and `*.egg-info`. The build-artifact and asset names `build`, `dist`, `_build`,
`static`, `public`, and `assets` are left out only directly under a member's root: deeper, the
same name is an ordinary package directory, such as a Go project's `internal/build`.

When a member's path contains another member (for example `path = "."`, or a member nested in
another's directory), the other member's tree is pruned, so one member's scan never takes in
another's source.

## Test context

One predicate, computed on the path relative to the member root, decides what counts as test code
for every rule:

1. **At any depth**: `__tests__/` and `testdata/`.
2. **As the first path component only**: `test/`, `tests/`, `example/`, `examples/`, and
   `integration_test/`. Matching the first component is required: a production `src/test/` is not
   test code.
3. **File names**: `test_*.py`, `*_test.py`, `conftest.py`, `*_test.go`, `*.test.[jt]sx?`, and
   `*.spec.[jt]sx?`.

## Import resolution

### Python

- **Guarded imports**: an import is guarded if and only if it sits in the try body of a `try`
  whose `except` catches `ImportError` or `ModuleNotFoundError` (directly, in a tuple, or bound
  with `as`). An import in the `except` body is a fallback import, not a guarded one.
- **Type-only imports**: an import under `if TYPE_CHECKING:`, bare or `typing.`-qualified, never
  counts for or against a declared dependency.
- **Relative imports** are resolved to absolute dotted names from the file's package position.
  They resolve modules inside the member only, never other members.
- **Resolving an import to a workspace member**, in order:
  1. drop relative imports and standard-library modules (a table baked from CPython's
     `sys.stdlib_module_names`);
  2. match the normalized top-level name against member names, using PyPI normalization
     (lowercase, with `-`, `_`, and `.` unified);
  3. apply a member's explicit `import_name` override from the declarations;
  4. match the longest prefix in the namespace map, which is discovered by locating each member's
     package root (for example `src/orxt`) and mapping `namespace.member` to the member when a
     matching subdirectory exists;
  5. match any dotted component that normalizes to a member name.

  Imports resolve to the **workspace** member's name even when its registry name differs (a PyPI
  package `orxtra-transport` whose workspace member is `transport`), so dependency rules compare
  like with like.
- **`__main__.py`** in a package is an implicit entry point, run by `python -m`. It is never a
  dead-module candidate, and its imports still keep other modules alive.

### Go

A map from each member to its module path is built from its `go.mod`. An import matches a member
if it equals the member's module path or starts with the module path followed by `/`. Imports of
the member's own module path are self-imports and are excluded.

### TypeScript/JavaScript

Specifiers come from `import`, `export … from`, `require()`, and dynamic `import()`. A specifier
reduces to its bare package name: relative specifiers (`./`, `../`, `/`) and Node builtins
(including `node:`-prefixed ones) are dropped, `@scope/pkg/...` becomes `@scope/pkg`, and
`pkg/sub` becomes `pkg`. Package names match member names case-insensitively.

Relative imports inside a member probe the extensions `.ts`, `.tsx`, `.js`, `.mjs`, and `.cjs`,
map `.js` to `.ts` and `.jsx` to `.tsx`, and resolve a directory to its `index.*` file.

JavaScript and TypeScript files inside a Python package (a directory tree with `__init__.py`
between the file and the member root) are data resources, not npm modules, and are never scanned
as JavaScript.

## Dead modules

[`dead-modules`](../rules/dead-modules/) uses a different algorithm per language.

- **Python: union of imports.** A module is dead if no other production module's imports reference
  it by dotted-name prefix, and no `__init__.py` exports its leaf name through `__all__` or a
  relative import. Files directly under a root `scripts/` directory are standalone programs: they
  are never candidates, and their imports do not keep other modules alive.
- **Go: union of imports, per package.** The dead unit is a package directory, and only packages
  under an `internal/` path component are candidates. A package is dead if no `.go` file outside it
  imports its full module path. The `_test.go` files of other packages count, so a test helper
  imported only by other packages' tests is alive; a package's own tests, and files under
  `testdata/`, never keep it alive. Packages under `testdata/`, packages made only of `*_test.go`
  files, and main packages (programs built or run with `go run`, entry points in their own right)
  are never reported.
- **TypeScript/JavaScript: reachability from entry points.** A production file is dead if the
  resolved import graph cannot reach it from any entry point. Entry points come from
  `package.json` `exports` (traversing the condition and subpath tree recursively), `main`, and
  `bin` (string or object form). Test files are neither candidates nor entry points, so a module
  used only by tests is dead. An entry point inside `tsconfig.json`'s `compilerOptions.outDir`
  names compiler output: it resolves to the source it is compiled from, the same path under
  `rootDir` (tsconfig's own, or else the one directory every `include` pattern starts in), and the
  files under `outDir` are never scanned as source. `tsconfig.json` is read as JSON with comments
  and trailing commas. When no entry point resolves to scanned source, for example because every
  export points at `dist/` output and no `rootDir` is known, the rule reports nothing for that
  member rather than declaring the whole tree dead.

**No entry-point laundering.** A suppressed unit must not keep anything alive. For the
union-of-imports languages, a suppressed unit is removed from both the candidate set and the
reference union, so its own imports and exports keep no other unit alive. For reachability, a
suppressed non-entry unit's edges are never traversed.

## Library-boundary rules

The rules in `group:library` run only on members marked `library = true`, never on applications
or command-line tools, where they would produce mass false positives. Test and example files are
excluded by default, merged with configured excludes:

- Python: `tests/`, `test_*.py`, `conftest.py`, and `examples/`.
- Go: `*_test.go` and `examples/`.
- TypeScript/JavaScript: `__tests__/`, `*.test.*`, `*.spec.*`, and `examples/`.

Default forbidden imports for [`library-forbidden-imports`](../rules/library-forbidden-imports/),
matched against the top-level module (Python) or the full package path (Go and
TypeScript/JavaScript):

- Python: `argparse`, `click`, `typer`, `flask`, `fastapi`, `django`, `uvicorn`, `granian`,
  `starlette`, `tornado`, and `bottle`.
- Go: `net/http`, `github.com/spf13/cobra`, and `github.com/urfave/cli`.
- TypeScript/JavaScript: `express`, `koa`, `hono`, `commander`, and `yargs`.

Each language's list can be replaced, and both the per-language `allow` list and the workspace's
`lint_allow` list are subtracted from the effective set.

Standard-stream writes for [`library-stdout`](../rules/library-stdout/): Python `print(...)` and
`sys.stdout` or `sys.stderr` writes. The rule needs full-semantic extraction, which exists for
Python only; the designed lists for the other languages are Go `fmt.Print*` and
`os.Stdout.Write`, and TypeScript `console.log`, `warn`, `error`, and `info`. Direct use of Python's root logger
(`logging.<method>(...)`) is the separate, lower-severity
[`library-direct-logging`](../rules/library-direct-logging/): a library should take a logger
rather than use the root logger.

CLI entry points for [`library-entry-point`](../rules/library-entry-point/): Python
`[project.scripts]` and `[project.gui-scripts]`, Go `func main()` in `package main`, and npm `bin`.
