# strictcode foundation build log

Durable record of every material decision and every deviation from the design
corpus (`DESIGN.md`, `CATALOG.md`, `schema/SPEC.md`) made during the autonomous
foundation build (started 2026-08-03). Per DESIGN.md §12 ("Foundation build
execution"), the builder session has full decision-making authority inside this
repository and must record its decisions here.

Format: dated entries, newest last. Each entry states the decision, the
rationale, and whether it deviates from a design document.

---

## 2026-08-03 — Scaffolding

**Go module and toolchain.** Module `github.com/smm-h/strictcode`, `go 1.26.3`
(the installed toolchain). No deviation.

**LICENSE.** Apache 2.0, verbatim from apache.org. Explicitly approved by the
owner in the build charter; DESIGN.md §11 lists it as a settled fact. No
deviation.

**SARIF todo.** Filed directly in `todo/.defer/` per DESIGN.md §10 (SARIF output
deferred; "todo filed in `todo/.defer/` at repo scaffolding time").

**rlsbl scaffold.** Ran `rlsbl scaffold --target go --publish-mode ci
--no-auto-tag --yes`. Decisions:

- `--no-auto-tag`: the auto-tag flag writes a GitHub topic to the remote repo.
  The build charter forbids touching the remote in any way (nothing is pushed
  before the first release), so the topic tag is left for the release session.
- The pre-existing `.github/workflows/publish.yml` was the hand-written PyPI
  placeholder workflow (manual dispatch, used for the name claim). Scaffold
  refuses to merge over a file it has no base for; the placeholder workflow was
  renamed to `.github/workflows/pypi-placeholder-publish.yml` (it remains
  useful if the placeholder ever needs republishing) and scaffold now owns
  `publish.yml` for the Go target.
- Scaffold detected the go target as `artifact: library` (no binaries). The
  DESIGN §12.5 goreleaser-binaries distribution is a release-round concern;
  the scaffold config can be extended then.

## 2026-08-03 — strictspec integration

**First toolchain contact: zero corrections.** The three schemas in
`schema/strictspec/` (authored from the spec appendix, never previously run
through the toolchain) passed `strictspec check` authoring validation, `strictspec
gen`, and full document validation (`--with-domain-checks`) of
`schema/vocabulary.toml` and all three profiles without a single surface-syntax
correction. The build charter anticipated fixes; none were needed.

**Manifest and reader layout.** `strictspec.toml` declares one Go target per
schema; generated readers live in per-schema packages
`internal/spec/{vocabspec,profilespec,findingsspec}` (one package per schema
avoids any future type-name collisions between generated bindings). Runtime
dependency `github.com/smm-h/strictspec/go v0.1.0` (registry version — no path
sources, per ecosystem policy).

**Round-trip tests.** `internal/spec/roundtrip_test.go` validates the committed
documents through the generated readers and enforces the closed-set rule
(SPEC.md §5): every profile declares a status for every vocabulary capability,
and no profile declares an unknown capability. Note: strictspec generates map
fields as raw `strictspec.Value` (entries via `.Entries()`), not Go maps.

## 2026-08-03 — Binding benchmark: CGo bindings win

Criteria were pinned in DESIGN.md §12.4; no judgment call remained. Verdict:
**the official CGo bindings (`github.com/tree-sitter/go-tree-sitter`) win** —
on every criterion, not just one.

**Candidates.** `github.com/odvcencio/gotreesitter` v0.48.0 (pure Go; the
design table referenced v0.15.x — the project moved fast) vs
`github.com/tree-sitter/go-tree-sitter` v0.25.0 with
`github.com/tree-sitter/tree-sitter-python` v0.25.0.

**Corpus.** `/home/m/Projects/rlsbl` (561 Python files) plus a fresh shallow
clone of django/django (2,927 Python files); 3,488 files, 27.15 MB total.

**Methodology.** Harness committed at `benchmark/binding-eval/` (a standalone
Go module, so the main module only carries the winning dependency). Three
modes:

- `identical`: parse every corpus file with both engines; serialize every node
  (all children, anonymous leaves included) as
  `(kind[!MISSING] start..end field:(child)...)` with identical walkers;
  compare byte-for-byte.
- `queries`: 15 Python queries covering every query form strictcode's
  extractors need (named nodes, fields, captures, anonymous leaves,
  alternation, wildcard, quantifiers `? * +`, anchor `.`, negated field
  `!field`, grouping, predicates `#eq? #not-eq? #match? #any-of?`); compile on
  both engines and compare full capture sets (pattern index, capture name,
  byte span) per file.
- `throughput`: single-threaded parse of the in-memory corpus, 3 rounds
  best-of, one reused parser per engine.

**Results.**

| Criterion | Result |
|---|---|
| Byte-identical trees (gate) | **FAIL** — rlsbl: 560/561 files mismatch; django: 2,265/2,927 mismatch |
| Query forms compile (gate) | pass — all 15 forms compile on both engines |
| Query results equal (gate) | **FAIL** — 537/561 rlsbl files differ (consequence of tree shape) |
| Throughput gts/cgo | **0.142** (0.60 MB/s vs 4.24 MB/s) — far below the 0.75 threshold |

**Root cause of the tree mismatch (not a harness artifact):** gotreesitter
deliberately normalizes Python trees away from C output —
`normalizePythonModuleNode` in `parser_result_python.go` unwraps single-child
`expression_statement` (and `_simple_statements`, `expression`,
`primary_expression`) wrapper nodes. Docstring statements therefore appear as
bare `(string)` children of `module`/`block` where the C grammar produces
`(expression_statement (string))`. This is by design in gotreesitter v0.48
(per-language "result compatibility" machinery), and it directly changes query
semantics: the docstring-anchor query
`(block . (expression_statement (string)) @docstring)` matches on CGo and not
on gotreesitter.

**Deviation from the design table.** DESIGN.md §9's comparison table recorded
"~1.15x faster full parse" for gotreesitter; measured reality on this corpus
is 7x slower. The table's numbers came from the project's own claims at
v0.15.x and do not hold for Python at v0.48.0.

**Consequences adopted with the CGo choice:** manual `Close()` on every
Parser/Tree/Query/QueryCursor (the integration layer owns this),
cross-compilation constraints (CGo), and `go test -race` limitations across
the FFI boundary. Grammar version pinning is now explicit in go.mod per
grammar (`tree-sitter-python` et al.), which is an upside: the C grammar and
the runtime are upstream-official.

## 2026-08-04 — Benchmark corrections (independent investigation)

A fresh-eyes investigation into the gotreesitter result (reported by the
coordinator at round-2 start) corrected three points; the CGo verdict stands:

1. **The design table's performance figures were withdrawn upstream claims.**
   The "~1.15x faster full parse / ~158x faster incremental" numbers came
   from a source benchmark that built no tree, never exercised GLR forking,
   and used mismatched grammar tables. Upstream's own current attested number
   is ~5.5x slower than C — corroborating our 7x-slower corpus measurement.
   DESIGN.md §9's binding table now carries the correction.
2. **The tree divergence is broader than the expression_statement
   normalization** documented in the 2026-08-03 entry: ~70% of mismatched
   files diverge for other reasons, the largest being a v0.48.0
   field-misattribution regression (the `name` field stamped on separator
   commas), reported upstream as odvcencio/gotreesitter#660.
3. **The verdict is bisect-robust.** Across a seven-version bisect, no
   gotreesitter version passes either absolute criterion (byte-identical
   trees, equal query results).

## 2026-08-03 — tree-sitter integration layer

`internal/treesitter`: the single parsing path. Owns grammar selection for
the trio (four concrete grammars: python, go, typescript, tsx — the ts
column spans two grammar variants because JSX conflicts with TS type
assertions upstream), LF normalization before parsing (so every byte span in
the system indexes LF-normalized UTF-8 per SPEC.md §3), and the CGo resource
lifecycle. Grammar versions: tree-sitter runtime v0.25.0, python v0.25.0, go
v0.25.0, typescript v0.23.2. `HasParseErrors` exposes error-tolerant parses
honestly; extractors will decide policy. Query text predicates are applied
by the binding's match iterator (verified in tests).

## 2026-08-03 — CLI framework: strictcli (deviation from DESIGN §9)

DESIGN.md §9's build-vs-depend table lists the CLI under "build". Decision:
**use strictcli** (`github.com/smm-h/strictcli/go` v0.27.0) instead.
Rationale: (1) the ecosystem standardized on strictcli after that table was
written — strictspec's own CLI is built on it; (2) strictcli enforces at
registration time exactly the flag conventions the ecosystem mandates
(banned bare `--force`, reserved `--no-*`, explicit bool defaults, mandatory
mutex choices), which an in-house layer would have to re-implement or drift
from; (3) it auto-injects `--dump-schema`, which the rlsbl release flow uses
to keep `.strictcli/schema.json` in sync. This is an internal-ecosystem
dependency, not third-party surface. The build-vs-depend principle's intent
(own the core IP, avoid prohibitive re-implementation) is unaffected: the
CLI shell is not core IP.

## 2026-08-03 — Vocabulary generator (built before the relation core)

Sequencing deviation from the charter's item order: the vocabulary/matrix
generator (charter item 5) was built before the relation core (item 4),
because the relation core enforces attribute discipline against the
generated vocabulary tables — dependency order beats listed order.

`cmd/vocabgen` + `internal/vocab/gen` read `schema/vocabulary.toml` and the
three profiles through the strictspec-generated readers (generation from an
invalid document is impossible), enforce the closed-set rule, and emit
`internal/vocab/vocab_gen.go`: NodeKind/RowKind/Capability/Layer/Lang
constants, attribute declarations, enums, and the per-language capability
statuses. Freshness is a test (`TestGeneratedVocabIsFresh`), so `go test`
(which the release flow runs) blocks stale generated code.

## 2026-08-03 — Rule registry and the committed artifacts

`internal/rules`: the 14 minted rules as Go declarations per CATALOG.md,
`group:library`, the empty tombstone table, and the matrix calculus
(requires/uses model; explicit per-rule overrides win; a required
not-applicable capability is terminal; uses never blocks). Invariant tests
enforce ID shape, uniqueness, capability existence, requires/uses
disjointness, group consistency, reason-carrying overrides, and
tombstone/live-ID disjointness.

Decisions taken where the design docs left gaps:

- **`dead-workspace-packages` suppression shape.** DESIGN §12.3 pins shapes
  for the path rules, dep rules, import-cycles, and stale-suppression but
  not this rule. Minted shape `member` (a single workspace member name) —
  its natural target.
- **`member-set` interpretation.** DESIGN §12.3 says "member-set for
  import-cycles". Interpreted as the set of *modules that are members of the
  reported cycle* (not workspace members — cycles are intra-project). The
  config schema field is `modules` with `min_len = 2` (an SCC has at least
  two members).
- **Library-boundary rules suppress via config lists, not suppressions.**
  The four `group:library` rules get shape `none`: their donor semantics
  exempt via allow lists / ignorable identifiers (rule options), which are a
  different mechanism from suppressions. Rule-option surfaces (forbidden and
  allow lists, ignorable stream identifiers, ignorable entry-point names)
  are deliberately **not** in the v1 config schema — they are designed with
  the library-rule implementation; strictspec migrations make that cheap.
- **Registry dump artifact:** `REGISTRY.json` at the repo root (visible
  beside CATALOG.md), produced by `strictcode registry dump`, checked by a
  strictspec schema (`schema/strictspec/registry.schema.toml`) whose
  capability enum is sourced from `schema/vocabulary.toml` (decision 32
  again). The dump self-validates through the generated reader before
  writing. The support matrix (`docs/MATRIX.md`) is produced by
  `strictcode matrix gen`. Freshness of both is enforced by tests.
- **Matrix presentation of language-independent rules.** `stale-suppression`
  requires no language capability; the vacuous calculus would render
  "supported" per language, which would read as an implementation claim.
  Presented instead in a distinct "language-independent rules" list.

## 2026-08-03 — Relation core

`internal/relation` implements SPEC.md faithfully: qualified IDs (structured
segments; serialized `<lang>:<member>:<module>:<chain>`; anonymous
`<hint>~<ordinal>~<fp8>`; overload `#<n>` omitted at 0), the node table and
typed rows, canonical form (version line + sorted node table + rows sorted
by the SPEC's total key), SHA-256 hash, and the two projections (algorithm
graph = distinct pairs per row kind; site feed = rows with spans/attrs).
The builder is the discipline chokepoint: unknown kinds, missing/undeclared
attributes, illegal enum values, ID collisions, case-only clashes, dangling
row endpoints, and src/dst kind violations are all hard errors.

One strengthening beyond SPEC §2.1's escape list (`%`, `:`, `.`, `#`,
whitespace): the tilde is also percent-encoded in segment names, so the
anonymous-segment structure `<hint>~<ordinal>~<fp8>` is unambiguous even
against crafted names (TS computed method names can contain `~`). Without
this, a named method literally called `cb~0~ab12cd34` would collide with an
anonymous unit. Recorded as a spec-side amendment candidate for SPEC.md.

Not built here (later rounds, per SPEC/DESIGN): the tier-1 structural
correspondence verifier (§7, fix-mechanics round) and the conforms_to
derived-row lazy materialization (extractors round).

## 2026-08-03 — Config schema

`schema/strictspec/config.schema.toml` implements exactly the pinned scope
(DESIGN §12.3): single `strictcode.toml`, analysis modes, group toggles,
rule enabled/severity/thresholds, and shaped suppressions with mandatory
non-empty reasons. Shape constraints are schema-enforced (exactly-one-of
across `path`/`dep`/`modules`/`member`, co-presence of `project`+`dep`);
shape-vs-rule matching and rule-ID validity (with tombstone rendering) are
consumer-native checks against the registry, to be implemented in the config
loader with the analysis engine. Analysis modes follow the explicit-mode
pattern: `python_call_resolution = "type-checker"` requires
`python_type_checker` (conditional-required), which is forbidden with
`"syntactic"`. Thresholds are a named-integer map — no seed rule has
thresholds; wave-two rules will use them.

## 2026-08-03 — Wrap-up state

- All commits covered in `.rlsbl/changes/unreleased.jsonl` (one user-facing
  feature entry: the CLI; the rest internal). `rlsbl check --tag changelog`
  passes; `changelog_format_version_enforced` set to true.
- `install_paths: ["./cmd/strictcode"]` declared in `.rlsbl/config.json`
  (vocabgen is dev tooling, not installed).
- Nothing pushed, nothing released, no registries touched — per charter.
- Remaining before the extractors round: the config loader (consumer-native
  checks: rule-ID validity with tombstone rendering, shape-vs-rule matching,
  staleness), the findings pipeline wiring (findingsspec is generated and
  ready), and the extractors themselves (import-graph depth for the trio),
  which will flip profile capability statuses to `supported` as they land.

---

# Round 2 (2026-08-04): the seed catalog end-to-end on all three languages

## Workspace reading (internal/workspace)

- **Format fidelity over spelling purity.** rlsbl workspaces in the wild
  carry BOTH `dev_only` and the older `dev_node` as the dev marker (the
  donor reads either); strictcode reads either too. This is reading other
  projects' committed files as they exist — input fidelity, not a
  backward-compat surface of strictcode's own.
- **Dependency scope mapping:** pyproject `project.dependencies` → runtime,
  `project.optional-dependencies` (extras) → peer, `dependency-groups` →
  dev; package.json dependencies/devDependencies/peerDependencies as named,
  `optionalDependencies` → peer; go.mod requires → runtime (Go has no dev
  scope in go.mod). The `explicit` scope is reserved for workspace-declared
  edges (none observed in current workspace.toml files; the enum arm stays).
- **Single-project scans cannot be libraries.** Without workspace.toml there
  is no `library = true` marker, so the synthesized `_` member is
  non-library and the library rules never run on single projects. A config
  surface for it can be minted when a consumer needs one.
- **Manifest reading via `strictspec.LoadValue`** (already a dependency —
  lossless TOML/JSON) and `golang.org/x/mod/modfile` for go.mod. No new
  TOML library.

## Config loader (internal/config)

- Registry checks implemented as load-time hard errors: unknown rule
  (tombstoned IDs render retired_in/reason/replaced_by/migration — the
  rendering is tested by injection since the tombstone set is empty),
  unknown group, suppression-shape-vs-rule mismatch, suppressions on a
  shape-`none` rule. Group toggles apply before per-rule overrides;
  per-rule wins.
- Disk/registry staleness is NOT a load error: it is the stale-suppression
  RULE (error severity), evaluated with the workspace in hand. Split per
  the round-2 charter.
- Config schema gained per-language `allow` / `forbidden` maps on rule
  config (lesson 26 needs the per-language allow list; `forbidden` is the
  replaceable default list). Migration-cheap by design.

## Extractors (internal/extract) — decisions where the spec left gaps

- **Python module identity:** files under a discovered package root get the
  dotted path from the base (src/ stripped); files OUTSIDE any package root
  (scripts/, conftest.py, tests without __init__) get their full
  member-relative dotted path (scripts/build.py → scripts.build). Package
  root discovery: member root and src/, one namespace level deep
  (ns/pkg — the donor's shape); discovery feeds both module naming and the
  cross-member namespace map.
- **Go module identity:** package dir relative to the member root; the root
  package is `.` (SPEC 2.2 gives the relative-path rule but no root
  spelling).
- **TS module identity:** extension stripped, index collapsed to its
  directory; a root-level index file is `.`. Known sharp edge (recorded
  round 1): `util.ts` + `util/index.ts` in one member is an ID collision
  hard error per SPEC 2.6.
- **Entry-point node IDs:** module segment `_`, single chain segment
  `<form>/<declared-name>` — SPEC section 2 does not pin entry-point IDs;
  this is deterministic and collision-free per (form, name).
- **Manifest-declared rows carry located sites:** declares_dependency and
  resolves_to rows get the span of the first occurrence of the dep/entry
  name in the manifest text (real file:line in findings instead of line 1).
- **Relative imports never member-resolve** (6.3 step 1 drops them); they
  are pre-resolved to absolute names from the file's package position for
  intra-member module rows only.
- **External imports are side data, not relation rows.** The vocabulary's
  imports row targets module/member nodes; external targets (flask,
  net/http, express) have no node kind. library-forbidden-imports needs the
  specifiers, so extraction carries an ExternalImports side table (lang,
  member, src module, specifier, site, test context). If a future round
  needs external deps in the graph proper, that is a vocabulary mint, not a
  silent extension.
- **Nested Go modules** (found on the real corpus: conformance harnesses
  with their own go.mod): nested requires aggregate into the member's
  declared edges, and intra-member package resolution uses the nearest
  enclosing module path. Red-green regression test.
- Python stdlib table baked from CPython 3.14.5 `sys.stdlib_module_names`
  (297 names); regenerate command: the python3 one-liner in git history of
  `internal/extract/pystdlib.go`.

## Checks (internal/checks) — decisions and conflicts resolved

- **deps-unused vs lesson 1 (CATALOG amended).** The CATALOG query sketch
  ("guarded rows qualify only for dev/peer") contradicted lesson 1 ("a
  guarded import MUST count as used for deps-unused") in the
  hard-dep-guarded-only case and would have double-reported it beside
  deps-hard-guarded-only with a false "never imported" message. Resolution:
  the lessons register is the acceptance suite and wins — ANY
  non-type-checking import marks the dep used; deps-hard-guarded-only alone
  owns the contradictory-guard diagnosis.
- **deps-dev-in-production exempts guarded imports.** A guarded production
  import of a dev-scoped dep is THE legitimate optional-dependency pattern
  (guards satisfy dev/peer, lessons 1-2); flagging it would criminalize the
  pattern the guard semantics exist for. The rule's declared uses of
  import-attr-guarded is exactly this refinement.
- **Suppression path semantics:** workspace-root-relative (the config file
  lives at the workspace root; one convention, no per-member ambiguity).
  import-cycles member-set suppressions match the reported SCC's sorted
  logical-name set exactly.
- **stale-suppression finding targets:** the config file is the site
  (file = strictcode.toml, line 1); the node is the named member's node
  when resolvable, else the first member. The findings schema requires a
  node-shaped target; the config file itself has no node kind. Good enough
  and honest; revisit if findings ever need config-file spans.
- **Exit-code rule:** exit 1 iff at least one error-severity finding;
  warnings alone exit 0; tool/config errors exit 2. DESIGN section 10 says
  "nonzero when findings exceed configured severity" — the configured
  per-rule severities ARE the threshold mechanism (set a rule to warning
  and it stops failing runs).
- **TS dead-modules abstains without resolved entry points** (donor
  safeguard, verified in the donor source: "No entry points declared —
  cannot determine reachability"): exports pointing at built dist/ output
  resolve to no scanned source, and reporting the whole tree dead would be
  the worst false-positive class in the catalog. When an entry DOES resolve
  (e.g. a bin script) but imports through a build step, unreachable source
  files are reported — donor-equal behavior, suppressions are the remedy.
- **`pkg/__main__.py` is an implicit entry point** (found on the real
  corpus: rlsbl's `python -m rlsbl` runner was flagged dead): never a
  dead-module candidate; its imports still count. Red-green regression.
- **TS dead-module candidates are production files only**; test files are
  neither candidates nor entry points (a module used only by tests is dead
  — consistent with the union-of-imports languages).
- Rules whose matrix cell is n/a for a language never run there (lesson 20
  mechanically: the check consults the same MatrixCell calculus the matrix
  renders).

## Lessons register coverage

Implemented as red-green tests (suite failed before the checks existed):
lessons 1-16, 18-23, 26, 28-32 in `internal/checks/lessons_test.go` (6-8
and 17-19 additionally at their home packages testctx/extract). Lessons
24-25 (unreachable-code) and 27 (library-direct-logging) belong to rules
whose capabilities are not in this round. Three real-corpus regressions
were added beyond the register: __main__ entry points, TS entry-point
abstention, nested Go modules.

## Real-corpus verification (read-only)

- rlsbl (561 py files): 12 findings in 2.4 s — real import cycles, dead
  docs directives (dynamically loaded — the path-suppression use case),
  zero false errors after the __main__ fix.
- strictspec (py+go+ts monorepo): runs clean end-to-end; TS source flagged
  unreachable behind the dist/ build step via the resolving bin entry
  (donor-equal; see abstention note).
- strictcli monorepo: 15 warnings, 0 errors after the nested-go.mod fix
  (the one error finding it surfaced was the real blind spot, now fixed).

## Matrix cells flipped planned→supported

- Python: all 10 import-graph capabilities.
- Go: 7 (guarded/type-checking are n/a; export-extraction stays planned —
  no Go export surface needed at import-graph depth).
- TS/JS: 9 (guarded is n/a).
- Rule rows now supported on their applicable languages: deps-unused,
  deps-undeclared, deps-runtime-test-only, deps-dev-in-production,
  dead-modules, dead-workspace-packages, library-forbidden-imports,
  library-entry-point (all three languages); deps-hard-guarded-only and
  import-cycles per their n/a overrides; stale-suppression
  (language-independent). Still planned: library-stdout,
  library-direct-logging, unreachable-code (full-semantic round).

## Stretch declined (deliberately)

Python full-semantic extraction (callables, types, syntactic call
resolution) is a design round of its own: SPEC 2.3-2.5 identity mechanics
(anonymous fingerprints, overload indexes, receiver normalization) and the
symbol-table resolution layer deserve the same care the import graph got.
Shipping a print/logging pattern-matcher and flipping
call-resolution-syntactic to supported would be dishonest about what
"supported" means. The two library call-graph rules land with that round.
