# strictcode rule catalog v1 — registry, lifecycle, and the seed rules

**Status: catalog round output, 2026-08-03.** Rule IDs follow the mint-once scheme (design
round rung #10): flat lowercase-hyphenated names, each identifying exactly one diagnosis,
deliberately encoding nothing. All metadata lives in the registry (Go declarations + schema
dump once code exists; this document is the registry's design-phase form). Donor names from
the rlsbl-absorbed catalog are re-minted at their best form here, in the pre-ship window
where renaming is free. Nothing has shipped: the tombstone table is empty and donor lineage
is documentation, not aliasing.

## 1. Registry lifecycle

Two operations, ever:

- **Mint.** A new rule ID is created, named carefully, once. Semver class: minor.
- **Tombstone.** A rule is retired. Its ID is never reused and never deleted from the
  registry. Semver class: breaking (configs and suppressions referencing it hard-error; the
  error is the migration prompt). A tombstone carries actionable fields:

```
[tombstone]
id          = "<the dead rule id>"
retired_in  = "<version>"
reason      = "<why this diagnosis no longer exists as such>"
replaced_by = ["<id>", ...]   # empty = gone without successor; two+ = it split
migration   = "<what a consumer should do with existing config/suppressions>"
```

The unknown-rule hard error renders the tombstone: "retired in vX.Y: <reason>; use
<replaced_by>; <migration>." Never a bare "unknown rule".

- No renames, no aliases, no dual recognition. Metadata (severity, thresholds,
  capabilities, tiers, group membership) changes freely without touching IDs
  (minor, changelog-covered).
- CI enforcement: the registry dump is committed; the release diff mechanically classifies
  the bump (metadata-only = minor; any tombstone = breaking) and the claimed bump type must
  match.

## 2. Groups

Groups are convenience switches, written `group:<name>`, syntactically distinct from rule
IDs. A finding never carries a group; suppressions never target a group; config may toggle
or re-severity a group's members in one stroke.

| Group | Members |
|---|---|
| `group:library` | `library-forbidden-imports`, `library-stdout`, `library-direct-logging`, `library-entry-point` |

Groups are minted on demand; this is the only one v1 needs (it replaces the donor's
`library-lint` aggregate).

## 3. Capability model: requires vs uses

Each rule declares two capability lists against `schema/vocabulary.toml`:

- **requires** — every capability must be `supported` in a language's profile for the rule's
  matrix cell to be supported. Any `planned` → planned; a required capability that is
  `not-applicable` → the rule is n/a for that language.
- **uses** — optional enrichment. A `not-applicable` or absent uses-capability never blocks
  support; it means that facet of the diagnosis does not exist in that language (e.g. Go has
  no guarded imports, so guard-related refinements simply never fire).

Rules may additionally carry an explicit per-language override (`not_applicable` with
mandatory reason) for cases where the capability calculus is satisfied but the check is
meaningless in the ecosystem (e.g. import cycles in Go — the compiler rejects them).

## 4. The minted rules

Severity defaults from DESIGN.md §6.2; all rules ship detection-first (fix tier 3), with
planned tiers noted. "Lineage" records the donor name for provenance only.

### Dependency hygiene

**`deps-unused`** — error. Lineage: deps-unused (kept).
A workspace-internal dependency declared in the manifest that no source file imports.
Requires: `import-extraction`, `resolve-imports-internal`, `declared-dependency-extraction`,
`test-context-classification`. Uses: `import-attr-guarded`.
Query (AMENDED 2026-08-04): declared runtime/dev/peer/explicit edges (workspace-internal
dst) minus deps with any `imports` row that is not type-checking-only. ANY import — guarded
or test-context included — marks the dep used (lessons 1 and 8); only `type_checking` rows
never count (lesson 5). The earlier sketch ("guarded rows qualify only for dev/peer-scoped
deps") contradicted lesson 1 for the hard-dep-guarded-only case and would have
double-reported it with a false "never imported" message beside deps-hard-guarded-only's
correct one; the lessons register is the acceptance suite and wins.
Planned tier 2: remove the declaration from the manifest.

**`deps-hard-guarded-only`** — error. Lineage: part of deps-unused's special-case messaging
(lessons 1–2), minted separately because it is a distinct diagnosis.
A hard dependency (scope runtime/explicit) imported *only* under optional-import guards —
contradictory; declare it optional or import it unconditionally.
Requires: as deps-unused plus `import-attr-guarded`.
Query: hard-scoped declared edge whose every matching imports row has `guarded = true`.

**`deps-undeclared`** — error. Lineage: deps-undeclared (kept).
Production source importing a workspace package the manifest does not declare.
Requires: `import-extraction`, `resolve-imports-internal`, `declared-dependency-extraction`,
`test-context-classification`. Uses: `import-attr-guarded`, `import-attr-type-checking`.
Query: imports rows resolving to workspace members with no declared edge; exempt
test-context, guarded, type_checking, and self-imports.
Planned tier 2: add the declaration to the manifest.

**`deps-runtime-test-only`** — warning. Lineage: deps-runtime-test-only (kept).
A runtime-scoped dependency imported only by test code — should be dev-scoped.
Requires: as deps-undeclared minus type-checking uses.
Query: runtime-scoped declared edge whose every imports row has `test_context = true`.
Planned tier 2: rescope the declaration.

**`deps-dev-in-production`** — error. Lineage: deps-dev-in-lib (re-minted: the donor name
said "lib" but the semantics are "dev-scoped dependency imported by production code" in any
project kind).
Requires: as deps-runtime-test-only.
Query: dev-scoped declared edge with at least one non-test imports row.
Planned tier 2: rescope the declaration.

### Dead code

**`dead-modules`** — warning. Lineage: dead-modules (kept).
Source units unreachable/unreferenced, per-language algorithms as pinned in DESIGN.md §6.2
(Python/Go union-of-imports with export and scripts/ handling; TS/JS BFS from entry points).
Requires: `module-enumeration`, `import-extraction`, `resolve-imports-modules`,
`test-context-classification`. Uses: `export-extraction` (AMENDED 2026-08-04, was in
requires: the export-exemption facet — lesson 16 — is Python-only; Go's package-granular
algorithm needs no export surface, and requires would wrongly force the Go cell through a
capability the diagnosis does not need there), `entry-point-discovery` (BFS languages).
Suppression semantics: no entry-point laundering (lesson 14).
Planned tier 2: delete the dead unit (consent-gated — deletion is behavior-relevant).

**`dead-workspace-packages`** — warning. Lineage: dead-workspace-packages (kept).
A library workspace member no sibling imports; test-only importers reported distinctly.
Requires: `import-extraction`, `resolve-imports-internal`,
`declared-dependency-extraction`, `test-context-classification`. Workspace-member metadata
(library/dev-only/releasable flags) is engine-level input, not a profile capability.
Query: library members with zero non-self production importers across the workspace;
exemptions per lesson 28.

### Cycles

**`import-cycles`** — warning. Lineage: circular-deps (re-minted: the donor name suggested
manifest dependencies; the check operates on the intra-project module import graph).
Import cycles within a project: Tarjan SCC over the module imports projection, SCCs of
size ≥ 2 only.
Requires: `module-enumeration`, `import-extraction`, `resolve-imports-modules`.
Override: not_applicable for go — "the Go compiler rejects import cycles; re-checking is
noise" (lesson 20).

### Config hygiene

**`stale-suppression`** — error. Lineage: dead-modules-stale (re-minted: the diagnosis is
config rot, not dead code; generalized to every suppression kind, not only dead-module
paths).
A suppression in strictcode.toml referencing a path, rule, or (project, dep) pair that no
longer exists on disk or in the registry.
Requires: none (language-independent; evaluated against config, disk, and registry).

### Library boundary (group:library — runs only on `library = true` members, lesson 22)

**`library-forbidden-imports`** — error. Lineage: library-lint/forbidden-imports.
A library importing an application-concern module (per-language default lists, replaceable;
workspace and per-language allow lists subtracted — lesson 26).
Requires: `import-extraction`. Uses: `test-context-classification` (default excludes).

**`library-stdout`** — error. Lineage: library-lint/stdout (print and stream writes).
A library writing to standard streams (`print`, `sys.stdout.write`, `fmt.Print*`,
`console.log` family).
Requires: `callable-extraction`, `call-resolution-syntactic`.

**`library-direct-logging`** — warning. Lineage: library-lint/stdout's logging facet
(minted separately: distinct diagnosis, distinct severity — lesson 27).
A Python library calling the root logger directly instead of taking a logger.
Requires: `callable-extraction`, `call-resolution-syntactic`.
Override: not_applicable for go and ts — the diagnosis is specific to Python's root-logger
idiom.

**`library-entry-point`** — error. Lineage: library-lint/entry-point.
A library declaring a CLI entry point (`[project.scripts]`, `func main` in package main,
npm `bin`).
Requires: `entry-point-discovery`.

### Correctness

**`unreachable-code`** — error. Lineage: library-lint/unreachable-code. **Departure
approved 2026-08-03:** the donor ran this only on libraries as part of the aggregate; minted here as
a standalone rule scoped to all projects, because unreachable statements are a correctness
diagnosis with no library-boundary false-positive risk (lesson 22 protected the boundary
rules, not this one). Semantics unchanged: unconditional-terminator analysis with comment
and nested-scope rules (lessons 24–25).
Requires: `unreachable-statement-analysis`.
Overrides: not_applicable for go — "go vet reports unreachable code natively". Planned
for ts.
Planned tier 1: remove the unreachable statements (the flagship whitelisted transform).

## 5. Deferred to wave two

Layering violations, god classes/functions, unstable dependencies, interface-segregation
violations — minted when designed, against full-semantic-graph capabilities. The scheme
requires no changes to accommodate them.
