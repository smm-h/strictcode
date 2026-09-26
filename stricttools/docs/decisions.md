+++
title = "Decisions"
description = "Why strictcode is built as it is: each design decision with its rejected alternatives, the dated build record, experiments, and open defects."
nav_order = 420
+++

# Decisions

This page is strictcode's record of why. Each design decision states what was chosen and why each
alternative lost, so a settled question is not reopened from scratch. The build record and the
experiments are dated: they describe what was true when they were written, and later entries
supersede earlier ones.

## Design decisions

### Implementation language: Go

Compared with Rust and Zig, through the lens that AI agents write and read most of the code.

- **Rust** lost on expressiveness. Traits, procedural macros, lifetimes, and deref coercion are
  where AI-generated code goes wrong: over-abstracted trait hierarchies, borrow-checker
  workarounds, and dispatch chains hidden across files. Its strengths serve human authors more
  than agents.
- **Zig** had real advantages (small binaries, tagged unions for graph modeling, and seamless C
  interop) and lost on everything else, per research in July 2026:
  - it was pre-1.0, with core standard-library APIs overhauled in consecutive releases;
  - no coding benchmark included it, and its community had built an MCP server to make up for
    weak model knowledge;
  - its package manager had no central registry, no conflict resolution, no vendoring, and
    cross-platform hash inconsistencies;
  - there was no precedent for a multi-language static analysis tool in Zig;
  - `comptime` could not be excluded to keep agents from getting creative, because Zig's generics
    and standard library are built on it.
- **Go** won on minimal expressiveness, fast compilation, ecosystem maturity, backward
  compatibility, built-in concurrency, a garbage collector suited to graph workloads, and
  precedent. Go 1.26's garbage collector was researched for large pointer-heavy heaps: code graphs
  stay well under the sizes where Go's historic pause problems appeared.

The comparison originally counted a pure-Go tree-sitter runtime as a Go advantage over Zig's C
interop. The benchmarks below showed that runtime unsuitable, so strictcode links C through CGo
after all. Go still wins: the remaining reasons hold, and CGo's cost is confined to one package
and the build.

### Parser: tree-sitter, through the official C runtime

tree-sitter was fixed from the start; the open question was the Go binding. The criteria were
pinned before measuring, so the verdict needed no judgment call:

1. the pure-Go runtime's parse trees must be byte-identical to the official C grammars' on real
   Python;
2. every tree-sitter query form strictcode needs must be supported, with equal results;
3. if both hold, the pure-Go runtime wins if its throughput is at least 75% of the CGo bindings'.

The CGo bindings won on every criterion, in August and again in September; see the experiments
below. Looking at the result again later changed the reasons, not the verdict:

- **The identical-trees criterion can never be met.** gotreesitter deliberately rewrites Python
  trees (removing single-child statement wrappers) and has no switch to turn that off. Remeasuring
  on a schedule is therefore pointless; the only thing that should reopen the question is upstream
  adding a C-compatible output mode.
- **The stronger argument is stability.** gotreesitter shipped a tree regression within days of a
  release twice (#660 in August, the `list_splat` misparse in September). With the ecosystem's
  always-latest dependency rule, strictcode's findings would change with every upgrade. The
  official runtime plus the official grammars is the stable reference every tree-sitter query is
  written against.
- **The 75% bar was arbitrary**, but moot: gotreesitter never cleared the first criterion.

**Rejected: writing a small tree-sitter runtime of our own.** strictcode only needs full parses of
a few grammars, but even that includes the hard parts: external scanners written in C per grammar
(Python's indentation, TypeScript's template strings and automatic semicolons), split parsing on
ambiguity, error recovery, and keeping trees identical to C. That is rebuilding gotreesitter,
including its difficulties, as a second product.

**The pure-Go fallback, if one is ever needed,** is the official C runtime compiled to
WebAssembly and run under wazero, a pure-Go WebAssembly runtime. Its trees would match by
construction, since it is the same C code; its speed is unmeasured. The existing benchmark
harness could measure it.

### Graph model: an interaction relation

How to represent several interactions between the same two nodes, such as five call sites with
different resolution sources, or one module importing another three times with different
attributes. Rejected:

- **A multigraph, one edge per site.** Every algorithm has to deduplicate, and canonical
  comparison has to sort parallel edges; an algorithm that forgets to deduplicate is silently
  wrong.
- **One collapsed edge per pair with a list of sites.** Per-site attributes become a second-class
  nested list, and a question like "is there a non-guarded import?" becomes list filtering rather
  than a graph question.
- **Collapsed edges with cached summary flags** such as `has_unguarded_import`. The flags can drift
  out of sync with the sites: two sources of truth.
- **Dual views, storing one form and deriving the other.** Two representations to keep coherent,
  with every consumer having to remember which view to ask for.
- **One edge per distinct attribute combination.** Edges multiply as attributes are added.
- **Call sites as nodes between caller and callee.** Every algorithm has to see through an extra
  layer of nodes.

Chosen: a flat typed relation as the only source of truth, with the algorithm graph, the site
feed, and the JSON output as projections, and canonical form defined once on sorted rows. See
[the graph model](../graph-model/).

### Node identity: qualified names

IDs appear in findings, suppressions, and fix verification, and must be reproducible from source
alone. Rejected:

- **Content hashes of the defining source.** Any fix edits the node, which gives it a new ID,
  which defeats post-fix verification. Every body edit would kill the node's suppressions under the
  stale-is-an-error rule. A grammar upgrade that shifts spans would change every ID. Identical code
  in two places collides, and the fix for that (salting with the path) removes the rename
  resilience that motivated hashing. The IDs are also unreadable in configuration.
- **Locations (file and position).** Any edit above a node moves it, so every unrelated edit breaks
  suppressions; formatting changes invalidate everything below them; and a fix that inserts or
  deletes lines moves every node after it. Location is the right key within one run and the right
  attribute, but the wrong identity.

Chosen: hierarchical qualified names from logical module identity, with a fingerprint that makes
anonymous-unit ordinal drift detectable, and verification by structural correspondence rather
than ID equality. See [node identity](../node-identity/).

### Vocabulary: universal kinds, capabilities, and profiles

How the vocabulary relates to languages that differ in kind: Go has no classes or inheritance but
has implicitly satisfied interfaces; Python has decorators and module-level code; TypeScript has
structural typing and overloads. Rejected:

- **One flat universal set of kinds with nothing else.** Language meaning is lost, so whole classes
  of checks become impossible.
- **Universal kinds plus a free-form detail string.** Checks that read the string silently couple
  to one language.
- **Per-language vocabularies.** Every rule has to handle every language separately, and the matrix
  stops meaning anything.
- **A universal core plus per-language extension kinds.** The boundary drifts, and extension checks
  bring back per-language rule code.
- **Tags on universal kinds.** Tag meaning is convention rather than contract, and equivalence
  across languages is unenforced.

Chosen: universal kinds; fine-grained capabilities bundled into layers; per-language profiles
declaring construct mappings and capability statuses; rules declaring required and used
capabilities; and the matrix generated from them, so a rule that cannot be supported in a language
says so by construction. See [capabilities and profiles](../capabilities-and-profiles/).

### Subtyping: one `conforms_to` row kind

Rejected:

- **A single subtype edge with no attributes.** It erases the difference between what the source
  claims and what analysis derived.
- **Separate kinds for nominal and structural.** That axis only loosely matches declared versus
  derived: TypeScript `implements` is nominal and declared, and Python's `ABC.register` is nominal
  and out of band.
- **Separate inherits and implements kinds.** Shaped by syntax, so Go satisfaction and embedding
  have no home.
- **Declared and derived as separate kinds.** One important axis becomes the kind, and the others
  become attributes, arbitrarily.
- **Reusing the call-resolution vocabulary** (`syntactic`, `type_informed`, `unresolved`) for who
  asserted the relationship. There is no such thing as an unresolved conformance.

Chosen: one row kind with three mandatory independent attributes, `provenance`, `discipline`, and
`mechanism`; declared rows stored; derived rows materialized lazily per rule; and the gap between
declared and actual as a projection. See [the vocabulary](../vocabulary/).

### Rule identity: mint once, retire with a record

Rule IDs are permanent once shipped. Rejected:

- **Keeping the donor names verbatim.** The inherited names were wrong in places (`circular-deps`
  checked module imports, not manifest dependencies; `dead-modules-stale` was configuration rot,
  not dead code), and the `library-lint` aggregate could not be suppressed per sub-check.
- **Renaming with the old names kept as aliases.** Two live names per rule is a
  backward-compatibility shim, and it makes every reference ambiguous.
- **Numeric codes.** Unreadable in configuration, and number ranges become an accidental
  taxonomy.
- **Both a number and a name.** Two identity spaces to keep in step.
- **Hierarchical names such as `lib.stdout`.** A frozen, arbitrary taxonomy, and a more complex ID
  grammar and configuration syntax.
- **Separate identities for checks, rules, and diagnoses.** Three spaces for a registry where
  almost every rule is one of each.

Chosen: flat names that each identify one diagnosis and encode nothing, with all metadata in the
registry; groups as a distinct, finding-free namespace replacing the aggregate; and two lifecycle
operations, adding and retiring, with retirement leaving a record that names successors. The donor
names were re-chosen before the first release, the only point where that was free. See
[rules](../rules/).

### Vocabulary content versioning: strictspec enum sourcing

A separate `vocabulary_version` field would need its own governance. strictspec's enum sourcing
makes a vocabulary change a change of every schema that sources from it, governed by strictspec's
exact-match version rules. That is stricter than tolerating "additive" changes, and consistent with
the ecosystem's exact-pairing rules. See [versioning](../versioning/).

### Speculative kinds, labeled

Kinds no rule uses yet (endpoints, database entities, variables, and type parameters) are fully
designed rather than left as empty reservations, so the vocabulary's machinery is exercised against
varied kinds early. They carry `maturity = "speculative"`. Speculation is fine; unlabeled
speculation is not. strictspec migrations make changing them cheap.

### Rules are built-in Go code

Rejected: a user-defined rule format (it becomes an API to stabilize and support forever) and an
embedded graph query language (a project of its own, and unlimited room for broken rules). Rules
stay correct, testable, and fixable because they are code.

### Stateless: rebuild every run

Rejected: a cache keyed by file hash (cache invalidation and a stale-state class of bug) and a
persistent graph database (heavy machinery for features nobody designed). The same input always
produces the same output.

### Tier-1 guarantee: whitelist plus graph re-verification

Rejected: the whitelist alone (a bug in a transform silently breaks user code, with the guarantee
resting entirely on strictcode's tests) and running the project's tests as the mechanism (it
depends on the project having tests, and passing tests are weaker than preserved behavior). See
[fixes](../fixes/).

### Suppressions: per-rule shapes

Rejected: suppressing by node ID everywhere. Users would copy IDs out of output into
configuration, and coarse intentions such as "this whole generated file" would need many entries
or prefix matching, which is a second mechanism in disguise. Each rule declares the natural shape
of its exceptions. See [configuration](../config/).

### Language scope

Python, Go, and TypeScript/JavaScript, deliberately different families: dynamic object-oriented,
procedural with implicit interfaces, and gradually typed. TypeScript and JavaScript are one column,
since TypeScript is a superset. Java and Kotlin were considered and dropped until a need exists.
Python was first to full-semantic depth because its dynamic typing makes call resolution hardest.

### Output formats

Human-readable text, JSON, and exit codes. SARIF is deferred.

### Documentation on selfdoc

Decided 2026-09-24. The design documents (a root `DESIGN.md`, `CATALOG.md`, and `BUILDLOG.md`, and
`schema/SPEC.md`) were replaced by these pages. selfdoc refuses a `docs/` directory and requires
`.stricttools/docs/`, so the pages live there. Tables are rendered by directives from committed
data (the registry dump, the vocabulary, the profiles, the strictspec schemas, and the CLI schema),
so the site cannot disagree with the tool.

The support matrix used to be a generated `docs/MATRIX.md`, written by `strictcode matrix gen`. A
generated page among handwritten ones breaks selfdoc's handwritten and generated split, and
rewriting the matrix calculus in a directive would duplicate the Go logic. So the registry
dump carries each rule's computed per-language cells, the `support-matrix` directive lays them
out, and `matrix gen` was removed. The registry dump moved from the root `REGISTRY.json` to
`schema/registry.json`.

## Build record

### 2026-08-03: foundation

- **Scaffolding.** Go module `github.com/smm-h/strictcode`, Apache 2.0, and rlsbl scaffolding. The
  hand-written PyPI placeholder workflow was renamed so scaffolding could own `publish.yml`.
- **strictspec, first contact.** The schemas were written from strictspec's syntax appendix without
  ever meeting the toolchain, and passed `strictspec check`, generation, and full document
  validation with no corrections. Each schema generates into its own package under
  `internal/spec/`, to avoid type-name collisions.
- **Binding benchmark.** CGo won; see the experiments.
- **CLI on strictcli, a deviation from the design,** which had listed the CLI as built in-house.
  The ecosystem had since standardized on strictcli, which enforces the mandated flag conventions
  when commands are registered and dumps the CLI schema. The CLI shell is not core to the tool.
- **Vocabulary generator before the relation core,** because the core enforces attribute
  discipline against the generated tables.
- **Relation core.** Every schema violation is a hard error: unknown kinds, missing or undeclared
  attributes, illegal enum values, ID collisions, case-only clashes, dangling endpoints, and
  wrong source or destination kinds. `~` was added to the escaped characters, so crafted names
  cannot impersonate anonymous-unit segments.
- **Registry.** The dead-workspace-packages suppression shape (`member`) was chosen where the
  design had none. The cycle suppression's "member set" was read as the set of modules in the
  cycle. The library-boundary rules take no suppressions: their exemptions are rule options.
  Language-independent rules are shown apart in the matrix, instead of as vacuously supported.
- **Configuration schema.** Suppression shapes are enforced by the schema; rule-ID validity and
  shape-to-rule matching are checked by the loader against the registry.

### 2026-08-04: the seed rules on three languages

- **Workspace reading.** Both dev-marker spellings found in real workspaces are read. Dependency
  scopes are mapped per manifest; see [check semantics](../check-semantics/). Single projects
  cannot be libraries.
- **Configuration loading.** Staleness became a rule rather than a load error, because it needs
  the workspace.
- **Extractor gaps filled.** Python files outside package roots keep member-relative dotted names.
  Go's root package is `.`, and so is a TypeScript root index. Entry points are identified by
  `<form>/<name>`. Manifest-declared rows point at the name's first occurrence. External imports
  became a side table instead of rows. Nested Go modules are honored.
- **Conflict resolved: `deps-unused` against lesson 1.** The catalog's first query sketch let
  guarded imports satisfy only dev and peer dependencies, which contradicted lesson 1 and would
  have reported a guarded-only hard dependency twice, once with a false "never imported" message.
  The lessons register wins: any import except a type-only one marks a dependency used, and
  `deps-hard-guarded-only` alone reports the contradiction.
- **`deps-dev-in-production` exempts guarded imports,** the legitimate optional-dependency
  pattern.
- **`dead-modules` moved `export-extraction` from requires to uses,** because the export exemption
  is Python-only and Go's algorithm needs no export surface.
- **Exit codes.** 1 for any error finding, 0 for warnings only, and 2 for tool or configuration
  errors. Per-rule severity is the threshold.
- **TypeScript reachability abstains without a resolved entry point,** matching a safeguard found
  in the donor's source.
- Declined on purpose: flipping `call-resolution-syntactic` to supported with a pattern matcher,
  which would have made "supported" untrue.

### 2026-08-04: Python full-semantic depth and tier-1 fixes

- Two extraction passes share the import pass's parse: nodes and containment first, resolution
  across the workspace second. `calls` rows exist only for resolved local-to-local calls; external
  and unresolved sites went to a side table, because a row needs two nodes.
- Resolution is conservative by construction; see [call resolution](../call-resolution/).
- Instantiation, declared conformance (inheritance and `ABC.register`), decorations, and
  `Protocol` and `Enum` classification came with it.
- Nested lambdas keep closure segments in their IDs, but attach their containment to the nearest
  non-closure ancestor.
- Tier-1 fixes: the declared delta is computed from the removal range; spans are ignored across
  the whole edited file (a deviation from the specification's "below the fix point", which proved
  unsound); some fixes are refused at planning time. See [fixes](../fixes/).
- The lessons register was complete, with further regressions added from real code; see
  [the lessons register](../lessons/).

### 2026-08-17: the strictcli v0.33.0 declaration regime

- Presence is declared once per flag and argument (required, optional, or defaulted).
- Mutating commands may not declare value defaults. Their four path fallbacks moved into the
  handlers and are stated in `--help`: every one names a search root or a destination, never a
  value written into an artifact.
- `fix`'s apply-or-preview mutex became a selector elected by flag presence, keeping the
  `--apply` and `--preview` spellings. This closed a hole where `--no-apply` satisfied the old
  mutex while choosing nothing.
- No command declares an update record: `fix`'s edits come from the planner, not from flags, and
  the other writers regenerate whole artifacts.

### 2026-09-24: new module paths, the matrix from data, and selfdoc

- strictcli and strictspec moved to the `stricttools` organization. strictcode moved to
  `github.com/stricttools/strictcli/go` v0.36.0 and `github.com/stricttools/strictspec/go`
  v0.3.0, and regenerated every strictspec reader, since generated code and runtime must come
  from the same release.
- The registry dump gained per-rule support cells, a language-independence flag, and the language
  list in column order. It moved to `schema/registry.json`, and its format version went to 2.
  `matrix gen` was removed.
- Documentation moved onto selfdoc (see the decision above).

## Experiments

### 2026-08-03: binding benchmark

gotreesitter v0.48.0 against `go-tree-sitter` v0.25.0 with `tree-sitter-python` v0.25.0, on rlsbl
(561 Python files) plus a shallow Django clone (2,927 files), 27.15 MB in total. The harness is in
`benchmark/binding-eval/`.

| Criterion | Result |
|---|---|
| Identical trees | Fail: 560 of 561 rlsbl files and 2,265 of 2,927 Django files differed. |
| Query forms compile | Pass: all forms compiled on both. |
| Query results equal | Fail: 537 of 561 rlsbl files differed. |
| Throughput relative to CGo | 0.142 (0.60 MB/s against 4.24 MB/s). |

The design had recorded gotreesitter as about 1.15 times faster than C. It was about 7 times
slower.

### 2026-08-04: why gotreesitter was slow

An independent investigation profiled the result:

- about 39% of CPU went to work the C runtime never does: Go map operations, garbage collection,
  and SHA-256 hashing of external-scanner state inside the parse loop, which Python's indentation
  scanner triggers on nearly every token;
- a fixed cost of about 1 ms per parse call, against 2.4 microseconds for CGo on an empty file;
- a second full parse whenever its compact parsing route gave up, with that route slower than the
  production route on this input when forced;
- allocation of about 300 times the input size.

The "1.15 times faster" and "158 times faster incremental" figures had been withdrawn upstream:
the source benchmark built no tree, never exercised split parsing, and compared against different
grammar tables. Upstream's own attested figure at the time was about 5.5 times slower than C. The
tree divergence was also broader than the documented wrapper removal: a field-misattribution
regression (the `name` field on separator commas) was reported upstream as
odvcencio/gotreesitter#660 and fixed within a day in v0.48.1. A bisect across releases showed no
version passing either tree criterion.

### 2026-09-24: second measurement

gotreesitter v0.48.0 and v0.54.0, the C side unchanged, on rlsbl (748 files at the time) and a
shallow Django clone at commit `951d13c` (2,932 files).

| Criterion | v0.48.0 | v0.54.0 |
|---|---|---|
| Trees differing, rlsbl | 744 of 748 | 744 of 748 |
| Trees differing, Django | 2,270 of 2,932 | 2,266 of 2,932 |
| Query results differing, rlsbl | 715 of 748 | 715 of 748 |
| Query results differing, Django | 996 of 2,932 | 1,220 of 2,932 |
| Throughput relative to CGo | 0.131 | 0.318 |

v0.54.0 was faster mainly because it turned the compact parsing route off by default. To separate
deliberate differences from bugs, the harness gained the `normalized` mode, which applies
gotreesitter's documented wrapper removal to the C tree before comparing. Files still differing:
21 of 3,680 on v0.48.1, 68 on v0.54.0's compact route, and 115 on its default production route.
What remained were misparses:

- `g(*a.b)` parsed as `g((*a).b)`, and likewise for subscripts, calls, and starred assignment
  targets. It entered the production route in v0.52.0 and became the default in v0.54.0. Reported
  as odvcencio/gotreesitter#1274.
- A phantom overlapping `escape_sequence` node for two consecutive escaped backslashes, present in
  every version checked. Reported as odvcencio/gotreesitter#1275.
- Three misparses that only appear in context: a class body span running about 1,300 bytes too
  long, an empty list reported as a pattern, and an `assert` with a call reported as a tuple
  expression. They are not yet reduced to small reproductions.

A cold build of strictcode (empty build cache, modules already downloaded) took about 11 seconds
and produced a 14.8 MB binary.

### Runs on real repositories

strictcode was run, read-only, on ecosystem repositories after each round. After the
full-semantic round, in August 2026:

| Repository | Findings |
|---|---|
| rlsbl | dead modules and import cycles, no errors |
| strictspec | dead modules, no errors |
| strictcli | dead modules and import cycles, no errors |
| selfdoc | dead modules, an import cycle, and `library-stdout` errors, all true positives |

The remaining dead-module findings were modules loaded dynamically, which is what path suppressions
are for. Each wrong finding along the way became a regression test.

## Open defects and design work

- **The type-checker call-resolution mode is silently ignored.** Configuring
  `python_call_resolution = "type-checker"` is accepted and has no effect, which breaks the
  no-silent-degradation principle. Until the mode exists, configuring it should be a hard error.
- **Unresolved calls have no representation in the relation.** It needs a vocabulary change, such
  as a node kind for unresolved targets or an optional destination.
- **Derived conformance** (lazy materialization and the declared-versus-actual projection),
  **reference extraction**, and **Go and TypeScript full-semantic depth** are not built. For Go,
  `go/parser` and `go/types` are the likely route; see [implementation](../implementation/).
- **The tier-2 consent flow** is not built.
- **SARIF output** is deferred.
- **The rlsbl handoff** needs the adapter on rlsbl's side and prebuilt binaries; see
  [distribution](../distribution/).
