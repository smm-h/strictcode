# rlsbl Contribution: The Source-Code Analysis Subsystem

**Status:** Authoritative specification. This document is self-sufficient. It describes an
entire subsystem that is being **removed from rlsbl and absorbed into strictcode**. The
Python source it documents will be deleted from rlsbl once the Go rewrite lands, so this
file — not the Python code — is the source of truth for strictcode's implementers.

**About strictcode:** strictcode is a deterministic, non-LLM Go CLI that builds a semantic
graph of a codebase and evaluates architectural rules against it (see `DESIGN.md`). It uses
tree-sitter for parsing; a pure-Go tree-sitter runtime (`gotreesitter`) exists as one
binding option, but this document does **not** prescribe an implementation. It specifies
*behavior*: what each absorbed check must detect, the language-specific edge cases the
behavior must handle, and the config surfaces that move with the subsystem. Where the
existing rlsbl implementation used regex or line-based heuristics, strictcode is free (and
encouraged) to reimplement on top of its real graph — provided the observable results, and
especially the hard-won false-positive avoidance in §5, are preserved or improved.

---

## 1. Purpose and boundary

### The invariant

The Go rewrite of rlsbl adopts one permanent rule:

> **rlsbl never parses source code.**

rlsbl operates only on *declared metadata*: manifest files (`pyproject.toml`,
`package.json`, `go.mod`, `pubspec.yaml`, `pom.xml`, `build.gradle`), the workspace
descriptor (`workspace.toml`), rlsbl's own config/state, git tags, and registry APIs.
Anything that requires reading a `.py`/`.go`/`.ts`/`.dart`/`.java`/`.kt` file and
interpreting its *contents* (imports, call structure, reachability, statements) is out of
scope for rlsbl and belongs to strictcode.

### What moves to strictcode (source-parsing analysis)

Checks (rlsbl check names given for continuity; strictcode may rename):

| rlsbl check | Severity | Languages today |
|---|---|---|
| `deps-unused` | error | Python, Go, npm/JS-TS, Dart, Java, Kotlin |
| `deps-undeclared` | error | Python, Go, npm/JS-TS, Dart, Java, Kotlin |
| `deps-runtime-test-only` | warning | all of the above |
| `deps-dev-in-lib` | error | all of the above |
| `dead-modules` | warning | Python, Go, npm, Dart, Maven/JVM |
| `dead-workspace-packages` | warning | all (via the shared import scan) |
| `circular-deps` | warning | Python, npm, Dart, Maven/JVM (Go excluded) |
| `library-lint` | error (aggregates per-finding severities) | Python, Go, npm; Maven via subprocess |
| `unreachable-code` | error | Python only (emitted inside `library-lint`) |

Infrastructure that moves with them:

- The import scanners (`import_scanners.py`): per-language extraction of workspace-relevant
  imports, `ImportInfo` records, test-context detection, namespace/module/package mapping.
- The dependency-analysis engine (`dep_validation.py`): unused/undeclared/scope checks,
  dead-module detectors (union-of-imports and BFS-from-entry-point), Tarjan SCC cycle
  finder, JVM class-to-file indexing, workspace-level dead-package detection.
- The lint engine (`lint/`): AST and regex backends per language, `forbidden-imports`,
  `stdout`, `entry-point`, and `unreachable-code` rules, plus per-language config loading.
- The config files these consume: `.rlsbl/lint/{python,go,npm}.toml`, `.rlsbl/lint.toml`
  (parser selection), `.rlsbl/dead-modules.toml`, `dep-overrides.toml`, and the
  `lint_allow` / `import_name` / `library` fields in `workspace.toml`.

### What stays in rlsbl (declared-metadata only — never absorbed)

- **`layers-violations`** — evaluates the *manifest-declared* dependency graph
  (`workspace.toml` layers config + the inter-project dependency edges rlsbl already parses
  from manifests) against a layer ordering. No source file is read. (strictcode has its own,
  independent layering check operating on its graph; that is a separate product feature, not
  this contribution.)
- **`deps-stale`** — compares intra-workspace dependency *version constraints* against
  current manifest versions. Pure metadata.
- **`version-consistency`, `name-consistency`, `license-consistency`,
  `description-consistency`, `target-version-readable`** — read manifests only.
- **`dev-only-boundary`, `unversioned-boundary`** — reachability over the manifest
  dependency graph (`ctx.graph.transitive_rdeps`), not source imports.
- **All `workspace-*` checks** (CI routing, registration, stale entries),
  `config-schema`, `scaffold-*`, changelog checks, `test-suite`/`test-suite-workspace`
  (which *run* the project's own test command via subprocess — no parsing),
  `ruff-lint` and Maven `library-lint` (which **delegate to an external subprocess** — see
  §7), and the release/changelog machinery.

### Two deliberately-flagged boundary cases

1. **`dunder-version-missing`** parses `__init__.py` with Python's `ast` module to detect a
   version-like constant not named `__version__`. It reads source, but its concern is
   *version management*, tightly coupled to rlsbl's release-time `__version__` bump. It is
   **not** part of this contribution. rlsbl must reimplement it without a source parse (a
   line/regex scan of a single known file, or dropping the heuristic), or accept it as the
   one metadata check that peeks at `__init__.py`. Do not absorb it into strictcode.
2. **Version bumping writes `__version__` into package source** during release. That is a
   targeted write to a known manifest-adjacent file, not analysis. It stays in rlsbl.

---

## 2. Checks to absorb

Each check below states: what it detects, per-language support, config surface, severity,
and known edge cases (edge cases are also collected as numbered requirements in §5).

### 2.1 `deps-unused` (error)

**Detects:** an intra-workspace dependency declared in a project's manifest that **no source
file imports**. Only workspace-internal dependencies are considered (external/registry deps
are ignored — the manifest name must match another workspace member).

**Per-language:** all scanned languages. Uses the shared import scan (§3).

**Config:** `dep-overrides.toml` (`[[unused_allowed]]` with `package`, `dep`, `reason` —
all mandatory, non-empty `reason`) whitelists a specific `(project, dep)` pair.

**Scope semantics (critical):** the manifest dependency carries a *scope*: `runtime`,
`dev`, `peer`, or `explicit`. The scan splits imports into `lib_imports` (production),
`test_imports`, and `guarded_imports` (imports inside `try/except ImportError`, see §3).

- A dep found in `lib_imports ∪ test_imports` is used → OK.
- A **guarded** import satisfies **only optional deps** (scope `dev`/`peer`). A *hard* dep
  (scope `runtime`/`explicit`) that is imported **only** inside a `try/except ImportError`
  is contradictory and **is flagged**, with a dedicated message telling the author to either
  declare it optional or import it unconditionally.
- A whitelisted `(project, dep)` pair is never flagged.

### 2.2 `deps-undeclared` (error)

**Detects:** a source file that **imports a workspace package** which the project's manifest
does **not** declare as a dependency.

**Per-language:** all scanned languages.

**Rules:**

- Only **lib (non-test)** imports are checked. Test-context imports are exempt (test code has
  looser rules).
- **Guarded** imports (`try/except ImportError`) are exempt — optional imports need not be
  declared.
- **`TYPE_CHECKING`** imports are exempt (typing-only, never a runtime dependency).
- **Self-imports** (a package importing its own submodules) are never flagged.

### 2.3 `deps-runtime-test-only` (warning)

**Detects:** a dependency declared with scope `runtime` that appears in `test_imports` but
**not** in `lib_imports` — i.e., it should be a dev dependency.

### 2.4 `deps-dev-in-lib` (error)

**Detects:** a dependency declared with scope `dev` that appears in `lib_imports` — dev
dependencies must not be imported by production code.

### 2.5 `dead-modules` (warning)

**Detects:** source units that no other unit references / that are unreachable. Two distinct
algorithms by language:

- **Python — union-of-imports.** A `.py` module is dead if (a) no other production module's
  imports reference it by dotted-name prefix match, **and** (b) its leaf name is not exported
  by any `__init__.py` (`__all__` entry or relative import). Relative imports are resolved to
  absolute dotted paths using the file's package position. Files directly under a root
  `scripts/` directory are standalone executables, excluded from the candidate set (their
  imports also do not keep other modules alive).
- **Go — union-of-imports, package-granular.** The dead unit is a **package directory** (not
  a file). Only packages under an `internal/` path component are candidates. A package is
  dead if no non-test `.go` file **outside** it imports its full module path. Test-context
  files never define candidate packages and their imports never keep a package alive.
- **npm / Dart / Maven-JVM — BFS from entry points.** A file is dead if unreachable through
  the resolved import graph from any declared entry point.
  - npm entry points: `package.json` `exports` (recursively collected), `main`, `bin`.
    Relative imports resolved with `.ts/.tsx/.js/.mjs/.cjs` extension probing, `.js→.ts` and
    `.jsx→.tsx` mapping, and directory→`index.*` resolution.
  - Dart entry points: `lib/<package_name>.dart` barrel file and every `bin/*.dart`. Resolves
    relative and self-`package:` imports; skips `dart:` and external `package:` imports.
  - JVM entry points: Java files containing `public static void main(String`; Kotlin files
    with a top-level `fun main(`. The import graph is built from a class-to-file index (§3.6).

**Config:** `.rlsbl/dead-modules.toml`, `[[known_non_entry]]` array of tables, each with a
mandatory `path` and mandatory non-empty `reason`. A declared path is a legitimately
unreachable unit (demo app, tool config). Path semantics per detector: Python/npm/Dart/JVM
paths name a **file**; Go paths name a **package directory**.

**No entry-point laundering (critical):** for the union-of-imports detectors (Python, Go), a
suppressed unit is removed from **both** the candidate set **and** the reference union — its
own imports/exports must not keep any *other* unit alive. For the BFS detectors
(npm/Dart/JVM), a suppressed non-entry unit's edges are simply never traversed, so
subtracting the suppressed paths from the reported dead set is sufficient.

**Companion check `dead-modules-stale` (error):** every `path` declared in
`dead-modules.toml` must exist on disk. This gates config rot; it does not parse source, but
it moves with the subsystem because it validates the subsystem's config.

### 2.6 `dead-workspace-packages` (warning)

**Detects:** a workspace package marked `library = true` that **no other workspace sibling
imports**. Distinguishes:

- imported in production by a sibling → alive;
- imported **only in test code** by siblings → warning ("only imported in test code by …");
- zero workspace importers → warning ("not imported by any workspace package").

**Exemptions:** dev-only projects are skipped; non-library projects (apps/CLIs are consumers,
not consumed) are skipped; **published releasable members** (consumed externally via a
registry) are exempt. Self-imports never count.

### 2.7 `circular-deps` (warning)

**Detects:** import cycles within a single project via **Tarjan's strongly-connected-
components** over a file-level import graph. Only SCCs of size ≥ 2 are reported (self-loops
ignored). Output: one cycle per SCC as a sorted list of relative file paths.

**Per-language:** Python, npm, Dart, Maven/JVM. **Go is intentionally excluded** — the Go
compiler already rejects import cycles, so a redundant check would be noise (mark the matrix
cell "n/a", not "unsupported").

### 2.8 `library-lint` (error) and `unreachable-code`

`library-lint` enforces that a **library** package does not leak application concerns. It
runs the multi-language lint engine (§4) over production source and aggregates results: any
`error`-severity finding fails the check; only `warning`-severity findings yield a warning.
Rule categories and their severities are specified in §4. `unreachable-code` is a Python-only
lint rule emitted by the same engine (see §4.4). It is not separately registered — it runs
for every Python file the linter visits.

---

## 3. Import-scanning infrastructure

The scanners convert source files into `ImportInfo` records restricted to
**workspace-relevant** imports (imports that resolve to another workspace member). Each
record carries:

```
ImportInfo(package_name, file_path, line_number, is_test_context, guarded, type_checking)
```

`package_name` is the **workspace member name** the import resolved to (not the raw import
string). The consumers split records into `(lib_imports, test_imports, guarded_imports)` by
inspecting `is_test_context`, `guarded`, and `type_checking` (see §2.1–2.4).

### 3.1 Guarded imports (`try/except ImportError`)

An import is `guarded=True` iff it sits in the **try body** of a `try` statement whose
`except` catches `ImportError` or `ModuleNotFoundError` (directly, in a tuple, or via
`as`-binding). Semantics:

- A guarded import **counts as "used"** for `deps-unused`, but **only satisfies optional
  deps** (scope `dev`/`peer`). A hard dep imported only inside a guard is still flagged (§2.1).
- Guarded imports are **exempt** from `deps-undeclared` (optional imports need not be declared).
- An import inside the **`except` body** (a fallback import) is **not** guarded — walking up
  the parent chain must skip past the enclosing `try` when the node is reached through an
  `except` clause. Only try-body imports are optional-dependency imports.

### 3.2 `TYPE_CHECKING` exclusion (Python)

An import inside `if TYPE_CHECKING:` (bare `TYPE_CHECKING` identifier, or attribute form
`typing.TYPE_CHECKING`) is `type_checking=True` and is **excluded from both `deps-undeclared`
and `deps-unused`** — typing-only imports are never runtime dependencies.

### 3.3 Test-context detection (three layers, order matters)

`is_test_context(filepath, project_root)` is computed on the **path relative to the project
root** (root-relative matching is required — a production `src/test/` must **not** be
misclassified):

- **Layer 1 — unconditional directories, matched at any depth:** `__tests__/`, `testdata/`.
  Any path with one of these as a directory component is test context.
- **Layer 2 — root-relative directories, matched only as the first path component:**
  `test/`, `tests/`, `example/`, `examples/`, `integration_test/`.
- **Layer 3 — file-name patterns (basename):** `test_*.py`, `*_test.py`, `conftest.py`,
  `*_test.go`, `*_test.dart`, `*.test.[jt]sx?`, `*.spec.[jt]sx?`, `*Test.java`,
  `*Tests.java`, `*Test.kt`, `*Tests.kt`.

The same predicate is reused across all detectors so that `dead-modules`, `circular-deps`,
and the dep checks share one definition of "non-production."

### 3.4 Python scanner: name resolution

Raw imports come from a tree-sitter AST walk (§4.1). Post-processing to match a workspace
member, in order:

1. Drop empty/relative (`.`-prefixed) imports and stdlib modules (`sys.stdlib_module_names`).
2. **Top-level normalized match:** normalize the first dotted component (PyPI normalization:
   lowercase, `-`/`_`/`.` unified) and match against normalized workspace member names.
3. **`import_name` override:** if `full_path` equals or is prefixed by a member's declared
   `import_name` (from `workspace.toml`), match that member.
4. **Namespace-map longest-prefix match:** an auto-discovered map (§3.5) from
   namespace-qualified paths (e.g. `orxt.protocols`) to member names, matched longest-prefix
   first against `full_path`.
5. **Sub-component match:** any dotted component (after the first) that normalizes to a
   workspace member name (catches `from orxt.protocols import Tool` where `protocols` is the
   member under the `orxt` namespace).

**Workspace-name vs registry-name mapping (critical):** a workspace member's PyPI/registry
name can differ from its workspace name (e.g. PyPI `orxtra-transport` vs workspace
`transport`). The scanner must resolve imports to the **workspace name** so the dep checks
compare like with like. Steps 2–5 collectively bridge these differences; do not assume the
import string equals the workspace name.

### 3.5 Namespace-package auto-discovery (Python)

`build_namespace_map(projects, workspace_root)` produces `{namespace.member: member}` by, for
each project: detecting its Python package root (e.g. `src/orxt`), taking the leaf directory
as the **namespace** (`orxt`), and scanning the package root's immediate subdirectories for
one matching the member name (or its `-`→`_` form). If `src/orxt/protocols/` exists for member
`protocols`, it maps `orxt.protocols → protocols`. This makes `from orxt.protocols import X`
resolve correctly for `deps-unused`/`deps-undeclared`.

### 3.6 Non-Python scanners

- **Go:** requires a `{member: go-module-path}` map (from each `go.mod`). Excludes self-imports
  (own module path). An import matches a member if it equals the member's module path or is
  prefixed by `module-path + "/"`. Imports extracted via tree-sitter (§4.2).
- **npm/JS-TS:** extracts specifiers via tree-sitter (`import`, `export … from`, `require()`,
  dynamic `import()`). A specifier is reduced to its bare package name: relative
  (`./`,`../`,`/`) and Node builtins (incl. `node:`-prefixed) are dropped; scoped
  `@scope/pkg/...` → `@scope/pkg`; unscoped `pkg/sub` → `pkg`. Matched case-insensitively
  against member names.
- **Dart:** regex over `import`/`export 'package:<pkg>/…'`; `<pkg>` matched against member
  names. Also raises a hard error if `build.yaml` exists but **no** `.g.dart` generated files
  are present (missing code generation) — for dep validation this error is caught and the
  Dart scan skipped rather than aborting the whole run.
- **Java/Kotlin (JVM):** requires a `{package-prefix: member}` map built by
  `build_jvm_package_map` from each project's `pom.xml` `groupId[.artifactId]` or Gradle
  `group`. Imports extracted by regex `import [static] a.b.C[.*];` (Kotlin's optional
  semicolon and Java `static` imports handled). Wildcard `.*` stripped. Longest-prefix match
  against the package map.

### 3.7 The class-to-file index (JVM)

Intra-project JVM dead-code and cycle detection needs a `{FQN: relative-file}` index. Built by
walking `src/main/java` and `src/main/kotlin`, reading each file's `package` declaration and
its top-level type declarations (Java: `class`/`interface`/`@interface`/`enum`/`record`;
Kotlin: `class`/`object`/`interface`, allowing leading modifiers). Known, accepted
limitations to preserve behavior parity: inner classes map to the outer file; Kotlin
file-level functions (which compile to `FileNameKt`) are not indexed. The import graph uses
this index with a **package-level fallback**: an import that resolves to no exact FQN, and
whose parent (static-import case) also doesn't, but which matches a known package, marks all
files in that package as dependencies.

### 3.8 Directory exclusions and sibling pruning

The source walk excludes a fixed set of non-source directories:
`.venv`, `venv`, `__pycache__`, `.git`, `node_modules`, `build`, `dist`, `.tox`,
`.mypy_cache`, `.pytest_cache`, `.ruff_cache`, `.selfdoc`, `_build`, `static`, `public`,
`assets`, and any `*.egg-info` directory. When a project's declared path is a parent of a
sibling workspace project (e.g. `path = "."`), the sibling's directory tree is pruned from the
walk so one project's scan never ingests another's source (this is what prevented
`deps-undeclared` false positives from sibling source in a root project).

Additionally, npm module scans skip any JS/TS file that lives **inside a Python package**
(a directory containing `__init__.py` between the file and the project root) — those are data
resources consumed by Python, not npm modules.

---

## 4. Lint engine

`lint_library(project_path)` detects which languages are present (by manifest) and runs the
appropriate backend for each, returning `LintResult(file, line, rule, severity, message)`
records. It is the engine behind `library-lint` and `unreachable-code`.

### 4.1 Backend selection: AST vs regex

`.rlsbl/lint.toml` has a single `parser` key: `"ast"` (default) or `"regex"`. Missing file →
`"ast"`. A present-but-invalid value or malformed TOML is a **hard error** (never silently
defaulted). The AST backends use tree-sitter; the regex backends are line-oriented fallbacks
providing the same rules with less precision. Import **scanning** (§3) always uses the AST
backend regardless of this setting — accurate import extraction is non-negotiable. strictcode,
which parses everything through its graph, may treat "regex mode" as a compatibility shim or
drop it; if dropped, that is a deliberate, documented simplification, not a silent behavior
change.

### 4.2 Rule: `forbidden-imports` (severity: error)

Flags a library importing a module on its forbidden list. Per-language defaults:

- **Python:** `argparse`, `click`, `typer`, `flask`, `fastapi`, `django`, `uvicorn`,
  `granian`, `starlette`, `tornado`, `bottle`.
- **Go:** `net/http`, `github.com/spf13/cobra`, `github.com/urfave/cli`.
- **npm:** `express`, `koa`, `hono`, `commander`, `yargs`.

Matched against the **top-level** module (Python) / full package path (Go, npm). The forbidden
list, an `allow` list, and exclusions are configurable per language (§4.6). `allow` entries are
subtracted from the forbidden set before matching; the workspace-level `lint_allow` list is
merged in on top (§4.7).

### 4.3 Rule: `stdout` (severity: error, except direct-logging = warning)

Flags a library writing to standard streams:

- **Python:** `print(...)` (error), `sys.stdout.write`/`sys.stderr.write` (error),
  `logging.<method>(...)` used directly (**warning** — libraries should take a logger, not use
  the root logger). Each of `print`/`sys`/`logging` can be individually ignored via config.
- **Go:** `fmt.Print`/`Printf`/`Println` (error), `os.Stdout.Write` (error). `fmt`/`os`
  ignorable.
- **npm:** `console.log`/`warn`/`error`/`info` (error). `console` ignorable.

Enable/disable via `[stdout] enabled` (default true).

### 4.4 Rule: `unreachable-code` (Python only, severity: error)

Flags statements that follow an **unconditional terminator** within the same block. A node
"always terminates" if it is a `return`/`raise`/`break`/`continue`, or a block whose last
statement always terminates, or an `if/elif/else` where **every** branch (including a present
`else`) always terminates. Requirements:

- **Comment nodes are not statements** — inline/trailing comments after a terminator must
  never be flagged, and must not mask a real unreachable statement after them. (This is the
  fix for the original false-positive/false-negative pair; preserve it.)
- Nested `function`/`class` definitions are their own scopes — a `return` in an inner function
  does not make the outer block's following code unreachable. The walker descends into nested
  scopes to find unreachable code *within* them, but a terminator in an inner scope never
  terminates the outer block.

### 4.5 Rule: `entry-point` (severity: error)

Flags a library declaring a CLI entry point:

- **Python:** `[project.scripts]` / `[project.gui-scripts]` entries in `pyproject.toml`.
- **Go:** `func main()` in a `package main` file.
- **npm:** a `bin` field in `package.json` (string or object form).

Individual entry-point names ignorable via `[entry-point] ignore`. Enable via
`[entry-point] enabled` (default true).

### 4.6 Per-language config schema (`.rlsbl/lint/<language>.toml`)

```toml
[forbidden-imports]
modules = ["..."]   # replaces the per-language default list
allow    = ["..."]  # subtracted from the effective forbidden set

[stdout]
enabled = true
ignore  = ["print", "sys", "logging"]   # per-language ignorable identifiers

[entry-point]
enabled = true
ignore  = ["name", ...]

[files]
exclude = ["glob", ...]   # merged with the default test/example excludes
```

Loading rules (all enforced as **hard errors**, never coerced): a present section must be a
table; a present scalar must have the right type; a list must contain only strings. Absent
keys keep documented defaults. Default `[files] exclude` per language (merged with user
excludes): Python `tests/`, `test_*.py`, `conftest.py`, `examples/`; Go `*_test.go`,
`examples/`; npm `__tests__/`, `*.test.js`, `*.test.ts`, `*.spec.js`, `*.spec.ts`,
`examples/`.

**Two-level resolution (releasable members):** the member-level
`.rlsbl/lint/<language>.toml` wins wholesale if present; otherwise, if the project belongs to a
releasable and a releasable-level `lint/<language>.toml` exists, that is used; otherwise the
per-language defaults apply.

### 4.7 `lint_allow` (workspace.toml)

A per-project `lint_allow` list in `workspace.toml` supplies forbidden-import exceptions
(e.g. a library legitimately importing its own test utilities). It is merged (union) with the
per-language TOML `allow` list before the forbidden set is computed. This is the coarse,
workspace-level escape valve; the TOML `allow` is the fine, per-project one.

### 4.8 Maven/JVM lint backend

The Maven backend does **not** parse source in-process — it detects and runs the project's own
lint command (detekt, checkstyle, or `./gradlew check`) as a subprocess with a timeout,
surfacing combined stdout/stderr on failure. Whether strictcode absorbs this at all is a
deliberate open choice: it is a subprocess delegate, so by the letter of the invariant it
could equally remain an rlsbl external check (§7). Flagged, not decided.

---

## 5. Lessons register

Every item is a hard requirement extracted from rlsbl's changelog history and code — each one
encodes a real false-positive or false-negative that was fixed and must never regress. Stated
as MUST / MUST NOT.

1. **Guarded optional imports.** MUST NOT flag a `deps-unused` for a workspace dep imported
   inside `try/except ImportError`/`ModuleNotFoundError` — it counts as used.
2. **Scope-aware guards.** MUST still flag a *hard* dep (scope `runtime`/`explicit`) that is
   imported **only** inside a guard, with a message to declare it optional or import it
   unconditionally. A guard satisfies only `dev`/`peer` deps.
3. **Fallback imports are not optional imports.** An import in the `except` body MUST NOT be
   treated as guarded; only try-body imports are optional.
4. **`deps-undeclared` respects guards.** MUST NOT flag `deps-undeclared` for a guarded
   optional import.
5. **`TYPE_CHECKING` exclusion.** MUST exclude imports under `if TYPE_CHECKING:` (bare and
   `typing.`-qualified) from both `deps-undeclared` and `deps-unused`.
6. **Root-relative test detection.** MUST classify test context by root-relative path, not
   substring — a production `src/test/` MUST NOT be treated as test code.
7. **`testdata/` and `__tests__/` at any depth.** MUST treat these as test context wherever
   they appear in the path.
8. **`integration_test/` and root `test`/`tests`/`example`/`examples`.** MUST treat these as
   test context when they are the first path component.
9. **Go `testdata/` fixtures and test-only packages.** MUST NOT report as dead any Go package
   under `testdata/` (any depth) or any package consisting solely of `*_test.go` files —
   apply the shared test-context exclusion to the Go dead-package detector.
10. **PyPI-name ≠ workspace-name.** MUST resolve imports to the **workspace** name even when
    the member's registry/PyPI name differs (e.g. `orxtra-transport` vs `transport`); MUST NOT
    emit `deps-undeclared` false positives from this mismatch.
11. **Namespace-package imports.** MUST resolve `from <ns>.<member> import X` to `<member>`
    via auto-discovered namespace mapping, so `deps-unused`/`deps-undeclared` don't
    false-positive on namespace packages.
12. **`import_name` override.** MUST honor a member's explicit `import_name` from
    `workspace.toml` when resolving imports.
13. **Sibling-source isolation.** MUST prune sibling workspace project directories from a
    project's scan (especially when `path = "."`), so one project's `deps-undeclared` MUST NOT
    be triggered by a sibling's source files.
14. **No entry-point laundering (union detectors).** For Python/Go dead-module detection, a
    `dead-modules.toml`-suppressed unit MUST be removed from the reference union too — its own
    imports/exports MUST NOT keep other units alive.
15. **`scripts/` are executables, not modules (Python).** MUST exclude files under a root
    `scripts/` directory from the Python dead-module candidate set, and MUST NOT let their
    imports save other modules from being flagged.
16. **`__init__.py` exports keep modules alive.** MUST NOT flag a Python module whose leaf
    name is exported by any `__init__.py` (`__all__` or relative import).
17. **npm files inside Python packages are data, not modules.** MUST NOT treat a JS/TS file
    living inside a directory tree containing `__init__.py` as an npm module.
18. **npm `.js→.ts` / directory-index resolution.** MUST resolve npm relative imports through
    extension probing (`.ts/.tsx/.js/.mjs/.cjs`), `.js→.ts`/`.jsx→.tsx` mapping, and
    directory→`index.*` resolution, or reachability will under-count and over-report dead code.
19. **npm entry points include `exports`, `main`, `bin`.** MUST collect entry points from all
    three (with recursive traversal of the `exports` condition/subpath tree).
20. **Dart self-`package:` resolution.** MUST resolve `package:<self>/…` imports to `lib/…`
    and MUST skip `dart:` and external `package:` imports in the intra-package graph.
21. **Dart missing codegen.** When `build.yaml` exists but no `.g.dart` files are present, the
    Dart import scan MUST surface this (as an error in lint context; caught-and-skipped in dep
    context) rather than silently analyzing incomplete source.
22. **Circular-deps excludes Go.** MUST NOT run cycle detection on Go (compiler enforces
    acyclic imports); mark "n/a".
23. **Circular-deps reports SCCs ≥ 2 only.** MUST ignore self-loops.
24. **`library-lint` skips non-library projects.** MUST NOT run boundary lint on projects not
    marked `library = true` (running it on everything produced hundreds of spurious errors).
25. **`library-lint` excludes tests/examples by default.** MUST apply the default per-language
    test/example excludes so `stdout`/`forbidden-import` are not flagged in test or example
    files.
26. **`unreachable-code` ignores comments.** MUST NOT treat comment nodes as statements — no
    false positive on a trailing comment after `return`, and no false negative masking a real
    unreachable statement.
27. **`unreachable-code` respects nested scopes.** A `return`/`raise` inside a nested
    function/class MUST NOT mark the enclosing block's following code unreachable.
28. **`lint_allow` / `allow` exceptions.** MUST subtract both the workspace `lint_allow` and
    the per-language `allow` list from the forbidden-import set.
29. **Direct `logging` is a warning, not an error (Python).** MUST classify direct
    `logging.<method>()` use in a library as a warning; `print`/`sys.stdout` writes as errors.
30. **`dead-workspace-packages` exemptions.** MUST skip dev-only projects, non-library
    projects, and published releasable members; MUST distinguish "test-only importers"
    (warning) from "zero importers" (warning) with distinct messages; self-imports never count.
31. **Asset/build directory exclusions.** MUST exclude `.selfdoc`, `_build`, `static`,
    `public`, `assets`, `node_modules`, `dist`, `build`, virtualenvs, caches, and
    `*.egg-info` from all source walks (these caused `dead-modules` false positives).
32. **Shared single scan.** All dep checks MUST share one import scan per project (the
    `(lib, test, guarded)` cache), not re-walk the tree per check.
33. **Config errors are hard errors.** Malformed `lint.toml`/`lint/<lang>.toml`/
    `dead-modules.toml`/`dep-overrides.toml` (wrong types, empty mandatory `reason`/`path`,
    non-table entries) MUST fail loudly, never be coerced or skipped.
34. **`dead-modules.toml` staleness.** A declared `known_non_entry` path that does not exist
    on disk MUST be a hard error (`dead-modules-stale`).

---

## 6. Integration contract with Go rlsbl

Go rlsbl consumes strictcode through its **external-check protocol** — the same mechanism
rlsbl already uses to run mypy/ruff/arbitrary commands during `rlsbl check --tag <tag>` and
release preflight. strictcode is an ordinary external check from rlsbl's perspective; it just
happens to be the tool that owns all source analysis.

### 6.1 The `external_checks` mechanism

Projects declare checks in `.rlsbl/config.json` under `external_checks` (a list). Every entry
declares a mandatory **`kind`**:

- **`freeform`** — an opaque shell `command` run verbatim. rlsbl cannot see its scope, so the
  `kind` marker makes that opacity a deliberate, visible declaration.
- **`structured`** — a known `tool` plus an explicit `paths` list. rlsbl composes the argv
  itself (no shell), routes the timeout through its budget, and **emits an extra
  competing-scope guard check** that hard-fails if the tool's own config file carries scope
  (e.g. a `files`/`include` key) that would silently override or narrow `paths`.

Common entry keys: `name` (must match `^[a-z][a-z0-9-]*$` — excludes glob metacharacters so a
name can't pattern-match a built-in check), `tag`, `kind`, optional `depends_on` (list of
check names), optional `cwd` (relative to project root). Structured entries add `tool` +
`paths`; freeform entries add `command`. Unknown keys are a hard error. Binary existence is
validated at registration time. Failure semantics: **non-zero exit = hard fail, no bypass.**

The current structured `tool` adapters are `mypy`, `ruff-check`, `ruff-format`. **strictcode
integrates as either** (a) a `freeform` entry `command = "strictcode check --format json ..."`,
or (b) a new first-class structured adapter (`tool = "strictcode"`) added to rlsbl's adapter
table — the preferred path, since it gives rlsbl argv composition, budgeted timeouts, and
`depends_on` ordering for free. This document does not mandate which; it specifies what
strictcode must accept and emit so either works.

### 6.2 Inputs available to strictcode

strictcode runs as a subprocess in a project or workspace directory. Everything it needs is
**on disk and committed** — it never depends on rlsbl passing it parsed data (rlsbl has none):

- the source tree itself (strictcode does its own parsing);
- the committed workspace descriptor (`workspace.toml`) — member names, paths, `library`
  flags, `import_name` overrides, `lint_allow`, dev-only/releasable markers;
- package manifests (`pyproject.toml`, `package.json`, `go.mod`, `pubspec.yaml`, `pom.xml`,
  `build.gradle[.kts]`) — for declared dependencies, entry points, package names, module
  paths, groupIds;
- its own config files (the migrated `lint/*.toml`, `dead-modules.toml`, `dep-overrides.toml`,
  or their strictcode-native equivalents).

strictcode must reconstruct, from these committed files, everything the old Python code
received as function arguments: the workspace member set, the `{member: go-module-path}` map,
the Python namespace map, the `{package-prefix: member}` JVM map, and the per-member manifest
dependency scopes. None of this comes from rlsbl at runtime.

### 6.3 Outputs

- **Exit code:** zero = pass, non-zero = fail. This is the hard gate rlsbl keys on.
- **Human text** on stdout for the CLI/logs, and **JSON** for machine consumption (rlsbl
  surfaces the first lines of output on failure). strictcode already plans CLI-text + JSON +
  exit-code output (`DESIGN.md` §9); that is exactly what this contract needs.
- Findings should carry `file:line`, a rule/check identifier, a severity, and a message, so
  rlsbl (and agents) can render them. Severity-to-exit mapping is strictcode's own
  configured-severity gate; rlsbl only observes the process exit code.

### 6.4 Naming, tags, ordering conventions

- Each external check has a stable `name` and a `tag`; rlsbl selects checks by tag
  (`preflight`, `changelog`, `workspace`, etc.) and can also select an external check by exact
  name. strictcode-backed checks should carry the `preflight` tag if they must gate releases.
- `depends_on` orders a check after named prerequisites (e.g. run strictcode after
  `test-suite`, or after a build-freshness check). rlsbl resolves the ordering.
- For a `structured` strictcode adapter, rlsbl would additionally emit the paired
  `<name>-scope-guard` check; strictcode's config-scope keys (if any) must be declared so that
  guard has something meaningful to compare, or the guard is a no-op pass.

---

## 7. Non-goals for the contribution

- **rlsbl keeps every manifest-level graph check.** `layers-violations`, `deps-stale`,
  `dev-only-boundary`, `unversioned-boundary`, and the version/name/license/description
  consistency checks all operate on the dependency graph and metadata rlsbl parses from
  manifests. They never read source, so they stay. The boundary is drawn exactly at "does
  answering this require interpreting the *contents* of a source file?" — if yes, strictcode;
  if it only needs manifests/`workspace.toml`/tags/registries, rlsbl.
- **rlsbl keeps subprocess-delegating checks.** `ruff-lint` and the Maven `library-lint`
  backend shell out to an external tool; rlsbl performs no in-process parsing, so these do not
  violate the invariant and need not move (Maven lint's disposition is explicitly flagged in
  §4.8 as an open choice).
- **`dunder-version-missing` does not move** (§1) — it is version-management, coupled to
  rlsbl's `__version__` bump, and rlsbl must reimplement it without a general source parse.
- **strictcode does not take over rlsbl's release orchestration, changelog system, CI
  scaffolding, or registry operations.** This contribution is strictly the source-analysis
  subsystem — the scanners, the dependency-analysis engine, the lint engine, and their config.
- **The regex lint backend is not a requirement.** It existed as a no-tree-sitter fallback.
  strictcode parses everything through its graph; it may keep regex mode as a shim or drop it
  as a documented simplification. Dropping it must be a deliberate decision, not silent
  behavior drift.
- **No behavior may silently degrade.** Per strictcode's own principles and rlsbl's, if a
  configured mode or dependency is unavailable, that is a hard error, never a quiet fallback to
  a weaker analysis.
