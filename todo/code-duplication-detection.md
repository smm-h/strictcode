# Code duplication / clone detection ledger

## Status

Design decisions were in progress; the user chose to stop the live question
chain and defer all final decisions to a future session. This file records the
goal, the leanings expressed this session (with their full option sets, so
nothing is locked), the still-open decisions with options and trade-offs, and
the integration audit — so the next session can decide without re-investigating.
Every leaning below is revisable: the user explicitly asked that all options be
left for the next session to decide.

## Goal

A deterministic, fast capability that finds code duplication — not just exact
duplication, but similar subsets: functions that are mostly the same but differ
in a few lines, duplicated classes, duplicated loops, and so on. Output is a
ledger of duplicated/similar content with exact addresses and a severity rating,
so the owner can prioritize and start deduplicating — extracting shared logic
themselves.

The original framing was "hash long strings so similar strings get similar
hashes" (locality-sensitive hashing). For code, that instinct is right only as
the scaling layer: the front-end must be structural (AST/token), never raw-text
SimHash, or the results are noise. Hashing/LSH belongs inside the pipeline to
bucket near-identical fragments, not as the representation itself.

## Why strictcode is the right home

strictcode already parses Python/Go/TS-JS with the tree-sitter CGo bindings,
builds a semantic graph, has a findings/ledger model with text and `--json`
output, a strictspec-schema-driven rule registry, and a tiered fix system.
Duplication detection is a clean new capability and is not in the project's
non-goals. Its hard constraints bind the feature: deterministic, no LLM, no
silent fallbacks, no regex parsing backend, schema-driven rules.

## Background: clone-detection taxonomy

Standard classification; the user's examples span the first three:

- Type-1 — identical except whitespace, layout, comments (a verbatim copied
  class).
- Type-2 — identical structure, renamed identifiers/literals/types (same loop
  with different variable names).
- Type-3 — copied then modified: lines added, removed, or changed (a function
  mostly the same but differing in some lines). This is the core hard
  requirement.
- Type-4 — different code, same behavior (semantic). Out of scope: undecidable
  in general and needs the semantic analysis the project's non-goals rule out.

Prior art to borrow ideas from (not as dependencies): winnowing (the MOSS
algorithm), Deckard (AST characteristic-vectors + LSH), and token-based
CPD-style detectors.

## Leanings expressed this session (all revisable)

Each was chosen by the user during the question chain before they asked to defer.
Recorded as strong leanings to confirm, not locked decisions.

1. Granularity — report whole semantic units (functions, methods, classes,
   large loop/block bodies) AND large partial fragments (a duplicated stretch
   inside two otherwise-different units).
   - Alternatives on the table: semantic units only (cleaner, misses partials);
     pure token/line windows ignoring structure (catches everything, noisier,
     less actionable).

2. Languages — all three (Python, Go, TS-JS) from the start. Unlike the semantic
   checks (Python has full-graph depth; Go and TS/JS have import-graph depth
   only), clone detection needs only syntactic parsing, which tree-sitter
   provides for all three, so it is not blocked on the semantic-graph work.
   - Alternative: Python first, then Go/TS — matches the project's "Python first
     for depth" precedent, but that constraint does not apply here, so it defers
     value for no technical reason.

3. Action scope — produce the ledger AND emit Tier-3 (non-mutating) refactoring
   suggestions describing the shared abstraction each clone class could collapse
   into. No automated (Tier-2) extraction.
   - Alternatives: report-only ledger (nothing touches the fix engine);
     Tier-2 auto-extraction (behavior-changing, often unsound across differing
     closures/side-effects/scope, high risk).

4. Cross-language — per-language only; do not compare fragments across
   languages.
   - Alternative: cross-language detection — research-grade difficulty, rarely
     actionable (no shared-logic extraction path across languages), pushes
     toward the semantic analysis the project avoids.

5. Clone-class output representation — extend the findings model so a finding can
   carry multiple locations (first-class multi-location findings), so clone
   output flows through the existing analyze path, JSON schema, exit codes, and
   suppression. This also lets the existing import-cycles check stop cramming its
   member list into free-text and pointing at only the first module.
   - Alternatives: a dedicated `duplicates` subcommand with its own bespoke
     ledger and schema (isolates the shape but forks the output surface and
     duplicates rendering/suppression); or reusing the current single-location
     model and listing the other fragments as prose in the message (cheapest,
     but the other fragments are not machine-readable addresses — contradicts the
     "exact address per fragment" requirement).

6. Quantitative fields — add a per-fragment / per-class similarity value and a
   computed numeric severity score as first-class fields, so the ledger is
   machine-sortable and prioritizable. Requires extending the severity
   representation (currently a two-value enum) and both the Go model and the
   strictspec schema.
   - Alternative: keep the error|warning enum and put similarity and ranking
     hints in the message text (minimal change, but the ledger cannot be sorted
     or filtered programmatically).

## Open decisions (never reached this session — for the next session)

A. Partial-fragment addressing. A byte/line span will be added to each location
   either way; the axis is whether to also mint new node kinds.
   - Enclosing unit + span (leaning candidate): add start/end line + byte span
     to each location; the kind names the enclosing function or type. Exact
     address for partials without growing the closed node-kind vocabulary.
   - Mint fragment/loop/block node kinds: the kind names the exact construct.
     More precise, but expands the closed node-kind vocabulary that every
     language profile must declare a status for.

B. Severity ranking inputs — which signals compose the rating that orders the
   ledger so the worst duplication surfaces first.
   - Size x copies x similarity x dispersion (spread across files/modules — a
     strong signal of a missing shared abstraction). Fully deterministic, no
     external data.
   - Also weight by churn (git history): ranks duplicates in frequently-edited
     code higher. More insight, but adds a git data dependency and risks
     non-determinism unless pinned to a specific commit.
   - Size x copies only: simplest; under-ranks near-duplicates and cross-module
     duplication.

C. Detection algorithm (implementation choice, recorded so it is decided
   deliberately). The pieces are complementary:
   - Exact pass: hash the normalized subtree forms and group identical hashes —
     Type-1 and Type-2 clone classes, exact and instant. Keep two normalized
     forms per fragment: a Type-1 form (strip comments/whitespace) and a Type-2
     form (also rename identifiers/literals to kind-placeholders).
   - Near pass for Type-3: winnowing fingerprints over the normalized token
     stream (guarantees detection of shared passages above a length threshold;
     catches cross-boundary partials), and/or Deckard-style AST
     characteristic-vectors + fixed-seed LSH bucketing (catches structural
     whole-unit near-clones). These cover different cases — token-window for
     partials, AST-vector for whole-unit structural near-clones.
   - Determinism: exact bucketing is trivially deterministic; the near pass is
     deterministic if the hash family and the LSH seed are fixed (never seeded
     from the clock). Within each LSH bucket, compute a real similarity score
     (token edit distance or Jaccard over k-grams) and threshold it.
   - Recommendation: exact-hash pass (Type-1/2) + winnowing (partials) +
     AST-vector/LSH (whole-unit Type-3). Cluster matches into clone classes
     (equivalence sets, not pairwise), so a fragment copied five times is one
     class, not ten confusing pairs.

D. Similarity threshold config type. The per-rule `thresholds` map is
   integer-only today. Encode the similarity threshold as an integer percentage
   (fits the existing schema, no change) vs extend the thresholds value type to
   allow floats. Leaning: integer percentage.

E. Minimum-fragment-size threshold. Below some size (tokens or lines),
   duplication is noise. Needs a default (the rule must define its own default,
   since the thresholds map does not validate declared keys — an unset key reads
   as zero) and should be user-tunable via the thresholds map.

F. Suppression shape. None of the existing suppression shapes fits a clone
   class. Mint a new shape (a fragment-set, or a clone-class identifier) plus a
   config-schema arm, vs reuse path/member suppression. Needs a decision.

G. Rule modeling. One rule or several? Options: a single `duplication` rule; or
   several node-kind-scoped rules (e.g. one for duplicated functions, one for
   duplicated types, one for duplicated blocks) collected under a `duplication`
   group, mirroring the existing `library` group pattern. Rule IDs are mint-once,
   flat lowercase-hyphenated, and never renamed or reused. Needs a decision.

H. Extraction capability and profiles. Clone detection can run on existing
   capabilities (it operates over already-parsed ASTs), so no new capability is
   strictly required. But minting a dedicated capability (e.g. fragment / clone
   index extraction) is the idiomatic way to express per-language support — and
   if minted it must be given a status in all three language profiles, or matrix
   generation fails (the capability set is closed and enforced). Needs a
   decision: reuse existing capabilities vs mint a new one.

## Integration audit (read from the code this session)

What the current models provide and what each needs. Anchored to symbols, not
line numbers, since line numbers drift.

- Findings model (`internal/findings/findings.go` + the strictspec schema at
  `schema/strictspec/findings.schema.toml`): the `Finding` type holds exactly one
  `Target`. To carry a clone class it must become multi-location (a list of
  located fragments). The Go model and the strictspec schema must agree — the
  document is validated twice (once in `findings.Build`, once by the framework
  where it writes the `--json` envelope). `findings.Build`, the `findingJSON`
  shape, and `RenderText` must all learn multiple locations. The exit-code
  contract and the framework envelope need no change.

- Address range: `Target` carries a single 1-based anchor `line` and no
  range/span in the output, even though the extractor works in byte spans
  internally (`relation.Span{Start,End}`). A clone fragment is a range, so
  start/end line plus byte span fields are needed on the location record and its
  schema.

- Severity: `rules.Severity` is `error | warning` only — no `info`, no numeric
  score. A numeric severity score and a similarity value are new fields with new
  schema arms. The exit-code path uses `FailRun` (only error-severity fails the
  run; warnings never do; tool/config errors exit 2). Decide how the numeric
  score maps onto error vs warning for the exit code.

- Node kinds: the closed node-kind enum (in `internal/vocab`) stops at
  function/type — there is no loop/block/fragment kind. See open decision A.

- Config: the per-rule `thresholds` map exists and is the natural home for tuning
  (`internal/config/config.go` + `schema/strictspec/config.schema.toml`), but it
  is integer-typed and free-form — unrecognized keys are accepted and stored
  silently, and an unset key reads as zero, so any rule reading a threshold must
  define its own default. See open decisions D and E.

- Tier-3 suggestion path: `Finding.Fix{Tier, Description}` already exists and is
  validated, rendered (an indented `fix (tier N): description` line), and
  JSON-serialized — proven end-to-end by the single Tier-1 unreachable-code
  usage. But no check emits a Tier-3 suggestion yet, and `Fix` is a flat
  `{tier, description}` string pair with no structured extraction target or
  per-site addresses. A description-only Tier-3 suggestion works as-is; a
  structured "extract to shared function X across these sites" suggestion
  requires extending `Fix`.

- Extraction model: all checks in a run share one graph built once — a check may
  run its own computation over the already-parsed data but must never re-parse.
  Clone detection needs new extractor output (a fragment/token index plus
  normalized fragment hashes) added to `extract.Result`, produced during the
  single extraction pass. See open decision H.

- Rule minting: adding a rule requires a registry entry in
  `internal/rules/rules.go`, an implementation registered in the `implemented`
  map in `internal/checks/run.go`, and regeneration of `REGISTRY.json` and
  `docs/MATRIX.md` (both committed and CI-diffed). See open decision G.

## Affected files (approximate)

- `internal/findings/findings.go`, `schema/strictspec/findings.schema.toml` —
  multi-location findings, per-location span, similarity value, numeric severity
  score; update the renderer and builder.
- `internal/checks/cycles.go` — migrate import-cycles to the new multi-location
  shape (it currently text-stuffs its member list).
- `internal/rules/rules.go`, `CATALOG.md` — mint the duplication rule(s)/group;
  possibly a new suppression shape.
- `internal/checks/run.go` plus a new detection implementation (a new
  `internal/checks/duplication.go`, or a dedicated `internal/dup/` package
  consumed by a thin check).
- `internal/extract/*` — new fragment/token index in `extract.Result`, with
  per-language fragment extraction and normalization for Python, Go, and TS-JS.
- `internal/config/config.go`, `schema/strictspec/config.schema.toml` —
  threshold keys (and the float-vs-percentage decision), and any new suppression
  arm.
- `internal/vocab/*` and `schema/profiles/{go,python,ts-js}.toml` — only if new
  node kinds or a new capability are minted.
- `cmd/strictcode/main.go` — multi-location rendering already routes through
  `RenderText`; extend it to print multiple locations.
- `REGISTRY.json`, `docs/MATRIX.md` — regenerated.
- Tests — red-green acceptance cases for known clone examples per language, plus
  extractor tests for the new fragment index.

## Effort estimate

- Model/schema extension (multi-location + span + similarity + severity score,
  plus migrating import-cycles): moderate — touches two schemas, the renderer,
  and the builder.
- Extraction (fragment index + normalization for the language trio): the largest
  piece; per-language work. Python is easiest (full-semantic extraction already
  exists); Go and TS/JS need fragment extraction over their ASTs.
- Detection (exact hash + winnowing + AST-vector/LSH) and clustering into clone
  classes: moderate, self-contained, deterministic.
- Severity scoring and Tier-3 suggestion text: small once the model exists.
- Overall: a multi-phase minor-release feature. Release once, at the end.
