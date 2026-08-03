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
