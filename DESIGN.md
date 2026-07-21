# strictcode — Design Document

*A linter for architecture, with tiered auto-fixes.*

**Status:** Design phase. No code exists yet. This document captures all decisions made to date and the open work remaining before implementation begins. It also fully absorbs the requirements of the rlsbl source-analysis contribution (§6) — the separate contribution document has been deleted; this file is self-sufficient.

---

## 1. What strictcode is

strictcode is a deterministic, non-LLM CLI tool that scans a codebase, builds a semantic graph of its structure, and uses that graph to find and fix architectural problems.

The premise: every modern programming language expresses programs through two fundamental abstractions — callable units (functions, methods, procedures) and type units (classes, structs, interfaces, enums). Any codebase can therefore be modeled as a graph where these are the nodes and their interactions (calls, inheritance, imports, references, instantiation) are the edges. strictcode builds that graph and evaluates architectural rules against it.

For every finding, strictcode offers a fix classified into exactly one of three tiers:

| Tier | Guarantee | Application |
|---|---|---|
| 1 | Behavior-preserving, guaranteed | Auto-applicable. Only whitelisted, hand-proven transforms qualify. After applying, the graph is re-extracted and verified against the expected result. |
| 2 | Behavior-changing, but arguably improved | Applied only with explicit consent. |
| 3 | Suggestion only | No auto-fix. The tool explains what is wrong and how a human or agent should fix it. |

## 2. Philosophy

strictcode is built for a world where AI agents are the primary readers and writers of code. Agents take shortcuts, ignore warnings, and choose the path of least resistance. The tool therefore enforces discipline through hard constraints, not soft guidance:

- **Deterministic.** No LLM anywhere in the analysis pipeline. The same input always produces the same output.
- **Stateless.** The graph is rebuilt from scratch on every run. No cache, no persistence, no stale-state bug class.
- **No silent degradation.** Analysis strategies are explicit modes selected by configuration, never runtime fallbacks. If a configured mode cannot run, that is a hard error, not a downgrade.
- **Hard errors, not warnings.** Findings above configured severity fail the run. No `--skip-checks`, no `--ignore-warnings`, no bypass flags.
- **Honest about limits.** When the analysis cannot resolve something (e.g., a dynamic call), it records an explicit `unresolved` edge rather than guessing. Every result is explainable.
- **Every suppression carries a reason.** Any config entry that silences a finding requires a mandatory, non-empty `reason`. Suppressions that reference paths that no longer exist are hard errors (config rot is a defect, not noise).

## 3. Product surface: the language × feature matrix

The product is a general-purpose matrix:

- **One axis: languages.** The initial set is exactly three columns: **Python**, **Go**, and **TypeScript/JavaScript**. Further languages are added only when a concrete need exists — never speculatively. The matrix is designed to grow, not pre-populated.
- **One axis: granular features.** Graph extraction capabilities, checks, and fix tiers.

Each cell is marked `supported`, `partial`, or `not applicable`. The goal is for most cells to read `supported`, while acknowledging that some features are very difficult or simply meaningless in certain ecosystems. `not applicable` is a first-class verdict, not a euphemism for "unsupported" — e.g., import-cycle detection on Go is n/a because the Go compiler already rejects cycles; re-checking would be noise.

Features are grouped by the graph capability they require, so that supporting a capability for a language lights up all checks that depend on it. Capabilities have **depth levels**: a language can be supported at the *import-graph* level (modules, imports, entry points — enough for the dependency-hygiene catalog in §6) before it is supported at the *full-semantic-graph* level (calls, types, inheritance — required for deep architectural checks). The initial trio is targeted at import-graph depth from the start; full-graph depth rolls out Python-first (§8).

### Language axis decisions

- **TypeScript and JavaScript are one column.** TS is a superset of JS; the same tree-sitter grammar parses both. Rules that work on TS work on JS with less type information available.
- **Python is the first column for full-graph depth** (see §8).
- Previously-considered columns (Java, Kotlin, and others) are deliberately out of scope until demand exists.

## 4. The graph model

### Node types (core)

Functions/methods/procedures and classes/types are the primary nodes. The full schema will be designed upfront as a versioned specification (see §12.1) and must also cover:

- **Modules/packages** (the coarse-grained grouping layer)
- **Workspace members** — a node type above modules: a project in a multi-project workspace, carrying its manifest-declared identity (name, registry name, library/app role) and its **declared dependency edges** (with scope: `runtime`, `dev`, `peer`, `explicit`) to other members. Declared-vs-actual dependency comparison is then a pure graph question.
- **Entry points** — first-class nodes: manifest-declared executables and exports (Python `[project.scripts]`, npm `exports`/`main`/`bin`, Go `package main`), the roots for reachability analysis.
- Interfaces/traits/protocols (contracts without implementation)
- Variables/fields (for data flow and reference analysis)
- Closures/lambdas (anonymous callables capturing scope)
- Generics/type parameters
- Macros/decorators/annotations (compile-time graph reshaping)
- Tests (callables with a `validates` relationship to production code)
- API endpoints/routes (entry points connecting the graph to the outside world)
- Database entities/schemas (the persistent-state layer driving real coupling)

### Edge types (core)

- **calls** — function A invokes function B
- **inherits/implements** — type A extends or implements type B
- **contains** — type A has method B; module A has function B
- **references** — function A takes/returns/uses type B
- **instantiates** — function A creates an instance of type B
- **imports** — module-level dependency, carrying attributes (below)
- **declares-dependency** — workspace member A's manifest declares member B, with scope

### Edge attributes

Each call edge is annotated with its **resolution source**: `syntactic`, `type-informed`, or `unresolved`.

Each import edge carries three boolean attributes, populated at extraction time:

- **`test_context`** — the importing file is test code (see §6.4 for the shared definition)
- **`guarded`** — the import sits in the try body of a `try/except ImportError` (Python; see §6.3)
- **`type_checking`** — the import sits under `if TYPE_CHECKING:` (Python)

These attributes exist so that dependency-hygiene checks are ordinary graph queries rather than special-cased scans.

### Schema discipline

The full schema (all node types, all edge types, all attributes) is designed and versioned **before** extraction code is written. This prevents schema churn from rippling through every language extractor later, and gives the matrix a stable vocabulary. The three-dimensional reality — language × check × fix-tier — hangs off this vocabulary.

## 5. What "wrong" means: checks

Rules are **built-in only**: Go code shipped with the tool, enabled/disabled and threshold-tuned via a config file (the Ruff model). No user-defined rule DSL, no embedded query language. Rules stay correct, testable, and fixable because they are code; users cannot write broken rules.

The catalog has two waves:

1. **The seed catalog (§6)** — dependency hygiene, dead code, cycles, and library-boundary lint, absorbed from rlsbl. Well-specified, edge-cases documented, immediately useful, and cheap to implement on the graph.
2. **Deep architectural checks** — enabled by full-graph depth: layering violations (edges crossing declared architectural boundaries in the wrong direction), god classes/functions (fan-in/fan-out and containment thresholds), unstable dependencies, interface segregation violations, structural inconsistencies across sibling code paths.

The rule engine is custom-built and operates on graph nodes and edges, not AST nodes. All checks in one run share the single graph built for that run — there is no per-check re-scanning by construction.

## 6. Seed catalog: the absorbed rlsbl source-analysis checks

### 6.1 Origin, authority, and boundary

The Go rewrite of rlsbl adopts a permanent invariant: **rlsbl never parses source code.** rlsbl operates only on declared metadata (manifests, `workspace.toml`, its own config, git tags, registry APIs). Everything that requires interpreting the *contents* of a source file moved out of rlsbl, and strictcode is its new home.

Authority: the original contribution document was a behavioral donor, not a mandate. strictcode adopts the **check semantics and the accumulated edge-case knowledge**, re-implemented natively on the strictcode graph. Where rlsbl used regex or line-based heuristics, strictcode uses the graph; observable results must be equal or better, and the false-positive protections in §6.7 are non-negotiable regressions tests.

Deliberate departures from the donor:

- **Languages:** the donor covered Python, Go, npm, Dart, and Java/Kotlin. strictcode implements the **Python / Go / TS-JS trio only**. Dart and JVM support (including the JVM class-to-file index) are dropped until a real need exists.
- **No regex backends.** The donor's regex lint fallback is not carried over — not even as a shim. Everything goes through the graph. This is a deliberate, documented simplification consistent with the no-fallback principle.
- **No subsystem split.** The donor's three subsystems (import scanners, dependency-analysis engine, lint engine) dissolve into strictcode's single model: extraction populates the graph; checks are queries over it.
- **Native config.** The donor's config files (`.rlsbl/lint/*.toml`, `dead-modules.toml`, `dep-overrides.toml`) are replaced by strictcode's own config surface (§6.6), preserving their principles: mandatory reasons, hard errors on malformed config, staleness detection.
- **Maven/JVM lint delegation** (a subprocess adapter in the donor) does not move to strictcode at all; if rlsbl wants it, it remains an rlsbl external check.

Boundary retained by rlsbl (never strictcode's business): manifest-level dependency-graph checks (`layers-violations` over declared deps, `deps-stale`, version/name/license consistency, dev-only/unversioned boundary checks), subprocess-delegating checks, changelog and release machinery, and the `__version__` bump (a targeted write, not analysis).

### 6.2 The checks

All nine checks operate per project (or per workspace member), using the shared graph. Severities are defaults, configurable like any rule.

| Check | Severity | Languages | Detects |
|---|---|---|---|
| `deps-unused` | error | Py, Go, TS/JS | A workspace-internal dependency declared in the manifest that no source file imports. |
| `deps-undeclared` | error | Py, Go, TS/JS | Production source importing a workspace package the manifest does not declare. |
| `deps-runtime-test-only` | warning | Py, Go, TS/JS | A `runtime`-scoped dependency imported only by test code — should be a dev dependency. |
| `deps-dev-in-lib` | error | Py, Go, TS/JS | A `dev`-scoped dependency imported by production code. |
| `dead-modules` | warning | Py, Go, TS/JS | Source units unreachable / unreferenced (algorithms below). |
| `dead-modules-stale` | error | all | A suppression path in config that does not exist on disk. |
| `dead-workspace-packages` | warning | all | A `library = true` workspace member no sibling imports (production vs test-only importers distinguished). |
| `circular-deps` | warning | Py, TS/JS; **Go: n/a** | Import cycles within a project: Tarjan SCC over the module import subgraph, SCCs of size ≥ 2 only (self-loops ignored). |
| `library-lint` (incl. `unreachable-code`) | error (aggregates per-finding severities) | Py, Go, TS/JS | A library package leaking application concerns (§6.5); plus Python unreachable-statement detection. |

**`deps-unused` scope semantics (critical):**

- Only workspace-internal deps are considered (the manifest name must resolve to another workspace member); external/registry deps are ignored.
- An import edge in production or test context marks the dep used.
- A **guarded** import (`guarded=true`) satisfies **only optional deps** (scope `dev`/`peer`). A *hard* dep (scope `runtime`/`explicit`) imported **only** under guards is contradictory and **is flagged**, with a message telling the author to either declare it optional or import it unconditionally.
- A configured suppression (`(project, dep)` pair with reason) is never flagged.

**`deps-undeclared` exemptions:** test-context imports; guarded imports (optional imports need not be declared); `type_checking` imports; self-imports (a package importing its own submodules).

**`dead-modules` algorithms per language:**

- **Python — union-of-imports.** A module is dead if (a) no other production module's imports reference it by dotted-name prefix, **and** (b) its leaf name is not exported by any `__init__.py` (via `__all__` or a relative import). Relative imports are resolved to absolute dotted paths from the file's package position. Files directly under a root `scripts/` directory are standalone executables: excluded from the candidate set, **and** their imports do not keep other modules alive.
- **Go — union-of-imports, package-granular.** The dead unit is a package **directory**. Only packages under an `internal/` path component are candidates. A package is dead if no non-test `.go` file outside it imports its full module path. Test-context files never define candidate packages and never keep a package alive. Packages under `testdata/` (any depth) and packages consisting solely of `*_test.go` files are never reported.
- **TS/JS — BFS from entry points.** A file is dead if unreachable through the resolved import graph from any entry point. Entry points come from `package.json` `exports` (recursively traversing the condition/subpath tree), `main`, and `bin` (string or object form). Relative import resolution must probe extensions (`.ts/.tsx/.js/.mjs/.cjs`), map `.js→.ts` and `.jsx→.tsx`, and resolve directories to `index.*`.

**No entry-point laundering (critical):** for union-of-imports detectors (Python, Go), a suppressed unit is removed from **both** the candidate set **and** the reference union — its own imports/exports must not keep any other unit alive. For the BFS detector (TS/JS), a suppressed non-entry unit's edges are simply never traversed.

**`dead-workspace-packages` exemptions:** dev-only projects skipped; non-library projects skipped (apps/CLIs are consumers, not consumed); published releasable members exempt (consumed externally via a registry); self-imports never count. Test-only importers produce a distinct message from zero importers.

### 6.3 Import semantics (Python)

- **Guarded:** an import is `guarded` iff it sits in the **try body** of a `try` whose `except` catches `ImportError` or `ModuleNotFoundError` (directly, in a tuple, or `as`-bound). An import in the **except body** (a fallback import) is **not** guarded — only try-body imports are optional-dependency imports.
- **`TYPE_CHECKING`:** an import under `if TYPE_CHECKING:` (bare or `typing.`-qualified) is excluded from both `deps-undeclared` and `deps-unused`.
- **Name resolution to workspace members**, in order: (1) drop relative imports and stdlib modules; (2) normalized top-level match (PyPI normalization: lowercase, `-`/`_`/`.` unified) against member names; (3) explicit `import_name` override from `workspace.toml`; (4) namespace-map longest-prefix match — auto-discovered by locating each member's package root (e.g. `src/orxt`) and mapping `namespace.member → member` when a matching subdirectory exists; (5) any dotted sub-component normalizing to a member name. **Registry name ≠ workspace name** (e.g. PyPI `orxtra-transport` vs workspace member `transport`): imports must resolve to the **workspace** name so dep checks compare like with like.

### Import semantics (Go)

Requires a `{member: go-module-path}` map built from each member's `go.mod`. Self-imports (own module path) are excluded. An import matches a member if it equals the member's module path or is prefixed by `module-path + "/"`.

### Import semantics (TS/JS)

Specifiers extracted from `import`, `export … from`, `require()`, and dynamic `import()`. A specifier reduces to its bare package name: relative (`./`, `../`, `/`) and Node builtins (including `node:`-prefixed) are dropped; `@scope/pkg/...` → `@scope/pkg`; `pkg/sub` → `pkg`. Matched case-insensitively against member names. JS/TS files living **inside a Python package** (a directory with `__init__.py` between the file and project root) are data resources, not modules — never scanned as npm source.

### 6.4 Test-context definition (shared)

One predicate, computed on the path **relative to the project root**, is shared by every check so "non-production" has a single definition:

1. **Unconditional directories, any depth:** `__tests__/`, `testdata/`.
2. **Root-relative directories, first path component only:** `test/`, `tests/`, `example/`, `examples/`, `integration_test/`. (Root-relative matching is required: a production `src/test/` is **not** test code.)
3. **File-name patterns:** `test_*.py`, `*_test.py`, `conftest.py`, `*_test.go`, `*.test.[jt]sx?`, `*.spec.[jt]sx?`.

### 6.5 Library-boundary lint rules

`library-lint` runs only on projects marked `library = true` — never on apps/CLIs (running it on everything produces mass false positives). Test and example files are excluded by default per language (Python: `tests/`, `test_*.py`, `conftest.py`, `examples/`; Go: `*_test.go`, `examples/`; TS/JS: `__tests__/`, `*.test.*`, `*.spec.*`, `examples/`), merged with user-configured excludes.

- **`forbidden-imports` (error).** A library importing an application-concern module. Default lists — Python: `argparse`, `click`, `typer`, `flask`, `fastapi`, `django`, `uvicorn`, `granian`, `starlette`, `tornado`, `bottle`; Go: `net/http`, `github.com/spf13/cobra`, `github.com/urfave/cli`; TS/JS: `express`, `koa`, `hono`, `commander`, `yargs`. Matched against the top-level module (Python) or full package path (Go, TS/JS). The forbidden list is replaceable per language; an `allow` list (per-language config) and a per-project workspace-level allow list are both subtracted from the effective forbidden set.
- **`stdout` (error; direct logging = warning).** A library writing to standard streams. Python: `print(...)` and `sys.stdout/stderr.write` are errors; direct `logging.<method>(...)` is a **warning** (libraries should take a logger, not use the root logger). Go: `fmt.Print*` and `os.Stdout.Write` are errors. TS/JS: `console.log/warn/error/info` are errors. Individual stream identifiers are ignorable via config; the rule is disableable.
- **`entry-point` (error).** A library declaring a CLI entry point: Python `[project.scripts]`/`[project.gui-scripts]`; Go `func main()` in `package main`; npm `bin` field. Individual names ignorable; rule disableable.
- **`unreachable-code` (Python, error).** Statements following an **unconditional terminator** in the same block. A node always-terminates if it is `return`/`raise`/`break`/`continue`, a block whose last statement always terminates, or an `if/elif/else` where **every** branch (including a present `else`) always terminates. Two hard requirements: **comment nodes are not statements** (a trailing comment after `return` is never flagged, and never masks a real unreachable statement after it); **nested scopes are independent** (a terminator inside a nested function/class never marks the enclosing block's following code unreachable, while the walker still descends to find unreachable code within nested scopes).

### 6.6 Config surface

strictcode-native configuration (exact format designed with the schema; principles fixed now):

- **Suppressions are structured and justified.** Dead-module suppressions name a path (file for Python/TS-JS, package directory for Go) with a mandatory non-empty `reason`. Dep-check suppressions name a `(project, dep)` pair with a mandatory reason.
- **Staleness is a hard error.** A suppression referencing a nonexistent path fails the run (`dead-modules-stale`).
- **Config errors are hard errors.** Wrong types, non-table entries, empty mandatory fields, unknown keys: fail loudly, never coerce, never skip.
- **Workspace inputs are read, not owned.** strictcode reads `workspace.toml` (member names, paths, `library` flags, `import_name` overrides, per-project lint allow lists, dev-only/releasable markers) and the manifests (`pyproject.toml`, `package.json`, `go.mod`) as committed inputs. It reconstructs everything it needs from disk: the member set, the Go module-path map, the Python namespace map, and per-member declared dependency scopes. Nothing is passed to it at runtime by any caller.
- **Source-walk exclusions.** Non-source directories are always excluded: `.venv`, `venv`, `__pycache__`, `.git`, `node_modules`, `build`, `dist`, `.tox`, `.mypy_cache`, `.pytest_cache`, `.ruff_cache`, `.selfdoc`, `_build`, `static`, `public`, `assets`, `*.egg-info`. When a member's declared path is a parent of a sibling member (e.g. `path = "."`), the sibling's tree is pruned so one project's scan never ingests another's source.

### 6.7 Lessons register (regression requirements)

Each item encodes a real false-positive or false-negative fixed in the donor implementation. These are MUST/MUST-NOT acceptance criteria for strictcode's implementations; none may regress. (Donor items specific to dropped languages are omitted.)

1. A workspace dep imported inside `try/except ImportError`/`ModuleNotFoundError` MUST count as used for `deps-unused`.
2. A *hard* dep (scope `runtime`/`explicit`) imported **only** under guards MUST still be flagged, with the declare-optional-or-import-unconditionally message. Guards satisfy only `dev`/`peer` deps.
3. An import in an `except` body (fallback import) MUST NOT be treated as guarded.
4. `deps-undeclared` MUST NOT flag guarded optional imports.
5. Imports under `if TYPE_CHECKING:` (bare and `typing.`-qualified) MUST be excluded from both `deps-undeclared` and `deps-unused`.
6. Test context MUST be classified by root-relative path, not substring — a production `src/test/` MUST NOT be test code.
7. `testdata/` and `__tests__/` MUST be test context at any depth.
8. `integration_test/` and root-level `test`/`tests`/`example`/`examples` MUST be test context as first path components.
9. Go packages under `testdata/` (any depth) or consisting solely of `*_test.go` files MUST NOT be reported dead.
10. Imports MUST resolve to the **workspace** member name even when the registry name differs; no false `deps-undeclared` from the mismatch.
11. Namespace-package imports (`from ns.member import X`) MUST resolve to the member via the auto-discovered namespace map.
12. A member's explicit `import_name` override MUST be honored.
13. Sibling workspace directories MUST be pruned from a project's scan (especially with `path = "."`); a sibling's source MUST NOT trigger `deps-undeclared`.
14. For union-of-imports dead-module detection (Python, Go), a suppressed unit MUST be removed from the reference union too — its imports/exports MUST NOT keep other units alive.
15. Python files under a root `scripts/` directory MUST be excluded from dead-module candidates, and their imports MUST NOT save other modules.
16. A Python module exported by any `__init__.py` (`__all__` or relative import) MUST NOT be flagged dead.
17. JS/TS files inside a Python package tree MUST NOT be treated as npm modules.
18. TS/JS relative-import resolution MUST probe extensions, map `.js→.ts`/`.jsx→.tsx`, and resolve directory→`index.*` — otherwise reachability under-counts and dead code over-reports.
19. TS/JS entry points MUST be collected from `exports` (recursive), `main`, and `bin`.
20. Cycle detection MUST NOT run on Go (compiler-enforced); the matrix cell is `n/a`.
21. Cycle detection MUST report only SCCs of size ≥ 2 (self-loops ignored).
22. `library-lint` MUST NOT run on projects not marked `library = true`.
23. `library-lint` MUST apply the default per-language test/example excludes.
24. `unreachable-code` MUST NOT treat comment nodes as statements — no false positive on trailing comments, no false negative masking real unreachable statements after them.
25. A terminator inside a nested function/class MUST NOT mark the enclosing block's following code unreachable.
26. Both the workspace-level allow list and the per-language `allow` list MUST be subtracted from the forbidden-imports set.
27. Direct `logging` use in a Python library MUST be a warning; `print`/`sys.stdout` writes MUST be errors.
28. `dead-workspace-packages` MUST skip dev-only, non-library, and published releasable members; MUST distinguish test-only importers from zero importers with distinct messages; self-imports never count.
29. Asset/build/vendor directories (§6.6 list) MUST be excluded from all source walks.
30. All checks in a run MUST share one graph build — no per-check re-scanning.
31. Malformed config MUST fail loudly (hard error), never be coerced or skipped.
32. A suppression path that does not exist on disk MUST be a hard error.

### 6.8 Fix tiers for the seed catalog

Seed checks ship **detection-first**: every finding is at least a tier-3 suggestion with file:line, rule id, severity, and message. Tier-1/tier-2 fixes are added per transform through the standard whitelist process (§7); the natural early candidates are unreachable-statement removal (tier 1) and manifest dependency-declaration edits (tier 2 — they change the install graph, so they are consent-gated, not auto-applied).

## 7. The three-tier fix system

### Tier 1: guaranteed behavior-preserving

Mechanism: **whitelist + graph re-verification.**

1. Only transforms from a hand-proven whitelist qualify (examples: remove unreachable code, sort imports, rename a private symbol together with all its references).
2. After applying a fix, strictcode re-parses the affected files, re-extracts the graph, and asserts the resulting semantic graph is isomorphic to the expected post-fix graph.

Two independent layers: proof by construction, plus mechanical verification. A transform whose re-verification fails is rolled back and reported as a tool bug — never silently accepted.

Running the project's own test suite after fixes is a possible optional extra layer, but is not the core mechanism (it depends on the user having tests, and "tests pass" is weaker than "behavior preserved").

### Tier 2: behavior-changing, arguably improved

Fixes that alter observable behavior in a way the rule argues is an improvement (e.g., tightening an over-permissive signature, editing a manifest's dependency declarations). Never applied automatically; requires explicit per-fix or per-rule consent.

### Tier 3: suggestion only

Findings where auto-fixing is impossible or unwise. The tool explains the problem, the evidence in the graph, and the recommended manual fix.

## 8. Python first (full-graph depth)

Python is the first column to reach full-semantic-graph depth: it has the largest ecosystem of messy codebases needing architecture enforcement, and its dynamic typing makes call resolution the hardest problem in the space — solving Python first means every subsequent language is easier. (All three trio languages get import-graph depth from the start, since the seed catalog requires it.)

### Call resolution: two explicit layers

1. **Syntactic + import-aware (always on).** Calls resolved through imports, module attributes, and class methods using strictcode's own symbol table. Dynamic dispatch (duck typing, monkey-patching, `getattr`) becomes an explicit `unresolved` edge — never a guess.
2. **Type-checker-backed (explicit opt-in mode).** Consumes type information from an external type checker (candidates: ty, pyright) to resolve method calls on inferred types. Dramatically better call graphs on typed codebases.

Per the no-silent-degradation principle: the presence of the type-checker mode in configuration is the choice. If configured, it must work — a missing or failing type checker is a hard error, never a silent downgrade to syntactic-only.

## 9. Implementation decisions

### Language: Go

Chosen over Rust and Zig after a full comparison plus targeted research (mid-2026 state):

**Why Go — the AI-authorship lens.** The codebase will be primarily written and read by AI agents, which inverts the usual valuation: verbosity is an ally, expressivity is an enemy. Go's deliberate minimalism (one way to do things, no macros, no operator overloading, no implicit conversions, limited generics) means AI cannot get creative, generates Go reliably, and reads it linearly.

Go's decisive advantages:

1. Less expressivity — AI can't get creative
2. Faster compilation
3. Richer ecosystem / libraries
4. Language maturity and stability (backwards-compatible since 2012)
5. Built-in concurrency (goroutines + channels) for parallel parsing and analysis
6. Better debugging (Delve)
7. Better string/text processing (source analysis is fundamentally string processing)
8. Safer memory model — GC handles cyclic graph structures with zero cognitive load
9. Better dependency management (Go modules)
10. Pure Go tree-sitter runtime exists (no CGo, no C toolchain, cross-compiles anywhere)
11. Go 1.26 "Green Tea" GC handles graph-sized workloads comfortably (code graphs are well under 1–2 GB; the historic GC horror stories were at 60+ GB heaps)
12. Proven AI code generation quality
13. Abundant precedent — many static analysis tools are built in Go

**Why not Zig.** Zig's genuine advantages (smaller binaries; tagged unions for graph modeling; native C interop) were outweighed by: pre-1.0 instability with breaking stdlib overhauls every release; unproven AI code generation (absent from every coding benchmark; the community built an MCP server to compensate for weak model knowledge); an immature package manager (no central registry, no conflict resolution, no vendoring, cross-platform hash bugs); and zero precedent for a multi-language static analysis tool in Zig. The C-interop advantage was neutralized entirely by the pure Go tree-sitter runtime.

**Why not Rust.** Rust's expressivity (traits, proc macros, lifetimes) is exactly the surface area where AI-generated code goes wrong — over-abstracted trait hierarchies, borrow-checker workarounds, invisible dispatch chains. Its strengths serve human authors more than AI ones.

Go's lack of sum types is the accepted cost: graph nodes are modeled with a `Kind` field plus type switches (or a small interface hierarchy), with access funneled through a small, well-tested accessor layer. The node vocabulary is stable and small, so the missing exhaustiveness checking is a bounded risk.

### Parser: tree-sitter (non-negotiable)

tree-sitter is the parsing foundation: MIT-licensed, 300+ language grammars, incremental, error-tolerant, universally adopted, very actively maintained. No parsing layer will be built or maintained in-house. There is exactly one parsing path — the graph extractor on tree-sitter ASTs; no regex fallback exists anywhere in the product.

**Binding choice is open** (§12.3). Two candidates, to be evaluated hands-on against real Python codebases before committing:

| | gotreesitter (pure Go) | Official CGo bindings |
|---|---|---|
| Project | `github.com/odvcencio/gotreesitter` | `github.com/tree-sitter/go-tree-sitter` |
| Nature | Ground-up pure Go reimplementation of the tree-sitter runtime | CGo wrapper maintained by the tree-sitter org |
| Grammars | 205 embedded (204 fully functional), lazy-loaded | Loaded at runtime, guaranteed upstream compatibility |
| Performance | ~1.15x faster full parse; ~158x faster incremental edits | FFI cost per call; slower incremental |
| Cross-compilation | Any GOOS/GOARCH incl. WASM, no C toolchain | Broken by CGo unless using `zig cc` as cross-compiler |
| Tooling | Race detector and fuzzing work normally | `go test -race` problematic across CGo boundary |
| Memory | GC-managed | Manual `Close()` on every Parser/Tree/Cursor/Query or C memory leaks |
| Risk | Young (v0.15.x), single author, bus factor of one | Org-backed, stable |

### Build-vs-depend policy

Build and maintain in-house anything simple; depend externally only where reimplementation is prohibitive.

- **Depend:** tree-sitter (parsing). For language-specific formatting-preserving transforms, candidates include LibCST (Python, MIT) and Comby (language-agnostic structural rewrites, Apache 2.0) — decision deferred until fix implementation.
- **Build:** graph construction (tree-sitter queries → nodes/edges; this is the core IP), graph storage (in-memory adjacency; no graph database), rule engine, fix engine, CLI, output formatting, visualization (emit Graphviz DOT when needed).

### Runtime model

- Stateless batch CLI: parse everything (parallelized via goroutines), build graph in memory, analyze, report, exit.
- No persistence between runs. If rebuild times ever genuinely hurt on huge repos, a per-file-hash cache can be considered later — it is explicitly *not* in the initial design.

## 10. Interface

### Output formats

- **Human-readable CLI text** — primary interface; findings grouped with locations, explanations, and suggested fixes.
- **JSON** — machine-readable structured output for editors, scripts, and AI agents. Every finding carries `file:line`, a stable rule/check identifier, a severity, and a message.
- **Exit codes** — nonzero when findings exceed configured severity; usable as a CI gate and pre-commit hook with zero integration work.
- **SARIF** — deferred (todo filed in `todo/.defer/` at repo scaffolding time).

### Configuration

A config file controls: enabled rules, per-rule thresholds, severity levels, declared architectural layers/boundaries, analysis modes (e.g., Python type-checker-backed resolution), and structured suppressions with mandatory reasons (§6.6). Configuration selects modes explicitly; there are no implicit defaults for anything that changes analysis behavior.

### Integration contract with rlsbl

strictcode is consumed by rlsbl through rlsbl's **external-check protocol** — strictcode is an ordinary external check from rlsbl's perspective that happens to own all source analysis. The contract:

- strictcode runs as a subprocess in a project or workspace directory. Everything it needs is **on disk and committed** (§6.6); rlsbl passes no parsed data at runtime.
- **Exit code** is the hard gate rlsbl keys on: zero = pass, nonzero = fail, no bypass.
- Human text on stdout for logs; JSON for machine consumption — both already core outputs.
- Check names are stable identifiers (lowercase, hyphenated) so rlsbl configs can reference them and order them via `depends_on`.
- Preferred long-term shape: a first-class `structured` adapter (`tool = "strictcode"`) in rlsbl's adapter table, which gives argv composition, budgeted timeouts, and dependency ordering for free; a `freeform` command entry works from day one. Which one, and when, is rlsbl's decision — strictcode only guarantees the I/O contract above.

## 11. Settled project facts

| Item | Decision |
|---|---|
| Name | strictcode (claimed on npm and PyPI; repo github.com/smm-h/strictcode) |
| License | Apache 2.0 |
| Implementation language | Go |
| Parser | tree-sitter (binding choice pending evaluation); no regex fallback anywhere |
| Language trio | Python, Go, TypeScript/JavaScript — more only on demonstrated need |
| First full-graph language | Python |
| Seed rule catalog | The nine absorbed rlsbl checks (§6), reimplemented on the graph |
| Rules | Built-in Go code, config toggles, no user DSL |
| Graph schema | Full versioned spec upfront (incl. import-edge attributes, entry points, workspace members) |
| Tier-1 guarantee | Whitelisted transforms + post-apply graph re-verification |
| Persistence | None — stateless, rebuilt every run |
| Output | CLI text + JSON + exit codes; SARIF deferred |
| rlsbl integration | Subprocess external check; exit code + JSON contract (§10) |
| Location | `~/Projects/strictcode`, standalone repo |

## 12. Open work (in dependency order)

1. **Graph schema spec.** The versioned node/edge/attribute vocabulary everything else builds on — now including import-edge attributes (`test_context`, `guarded`, `type_checking`), entry-point nodes, and workspace-member nodes with scoped declared-dependency edges. Full design upfront, before any extraction code.
2. **Rule catalog v1.** The seed catalog (§6) is specified; remaining work is mapping each seed check to its graph queries, finalizing the deep-architectural checks of wave two, and fixing stable rule identifiers and severities.
3. **Config format design.** The strictcode-native config surface implementing §6.6's principles.
4. **Binding evaluation.** Benchmark gotreesitter vs official CGo bindings on real Python codebases: grammar fidelity, query support, throughput, memory. Commit to one.
5. **Repo scaffolding.** rlsbl scaffold, Apache 2.0 LICENSE, `todo/` directory (including the deferred SARIF todo), Go module init.
6. **Extractors.** Import-graph depth for the trio (Python, Go, TS/JS) — required by the seed catalog; then full-graph depth for Python.
7. **Tier-1 fix mechanics detail.** The whitelist, the transform implementations, and the graph re-verification harness.
8. **rlsbl adapter coordination.** Once the CLI surface is stable, coordinate the `structured` adapter entry in rlsbl's adapter table.

## 13. Non-goals

- **No LLM in the analysis pipeline.** Ever. Determinism is the product.
- **No security scanning.** Joern, CodeQL, and Semgrep own that space; strictcode targets architecture, correctness, and discipline.
- **No user-extensible rule language.** Rules are code, maintained in-tree.
- **No watch mode / daemon / server** in the initial design. Batch CLI only.
- **No silent fallbacks of any kind** — including no regex parsing backend.
- **No speculative language support.** Dart, Java/Kotlin, and everything else wait for a demonstrated need.
- **No release orchestration, changelog, scaffolding, or registry duties.** Those are rlsbl's; strictcode is the source-analysis specialist rlsbl delegates to.
