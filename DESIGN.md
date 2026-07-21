# strictcode — Design Document

*A linter for architecture, with tiered auto-fixes.*

**Status:** Design phase. No code exists yet. This document captures all decisions made to date and the open work remaining before implementation begins.

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

## 3. Product surface: the language × feature matrix

The product is a general-purpose matrix:

- **One axis: languages.** Python, Go, TypeScript/JavaScript, Java, Kotlin, and more over time.
- **One axis: granular features.** Graph extraction capabilities, checks, and fix tiers.

Each cell is marked `supported`, `partial`, or `not applicable`. The goal is for most cells to read `supported`, while acknowledging that some features are very difficult or simply meaningless in certain ecosystems.

Features are grouped by the graph capability they require, so that supporting a capability for a language (e.g., call graph extraction for Python) lights up all checks that depend on it, rather than being tracked as isolated per-check facts.

### Language axis decisions

- **TypeScript and JavaScript are one column.** TS is a superset of JS; the same tree-sitter grammar parses both. Rules that work on TS work on JS with less type information available.
- **Java and Kotlin are separate columns** with a shared JVM interop story. Different syntax, idioms, and grammars — but constant cross-language calls within one codebase require cross-language edge resolution at the JVM level.
- **Python is the first column** (see §7).

## 4. The graph model

### Node types (core)

Functions/methods/procedures and classes/types are the primary nodes. The full schema will be designed upfront as a versioned specification (see §10.1) and is expected to also cover:

- Modules/packages (the coarse-grained grouping layer)
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
- **imports/depends on** — module-level dependency

Each call edge is annotated with its **resolution source**: `syntactic`, `type-informed`, or `unresolved`.

### Schema discipline

The full schema (all node types, all edge types) is designed and versioned **before** extraction code is written. This prevents schema churn from rippling through every language extractor later, and gives the matrix a stable vocabulary. The three-dimensional reality — language × check × fix-tier — hangs off this vocabulary.

## 5. What "wrong" means: checks

Rules are **built-in only**: Go code shipped with the tool, enabled/disabled and threshold-tuned via a config file (the Ruff model). No user-defined rule DSL, no embedded query language. Rules stay correct, testable, and fixable because they are code; users cannot write broken rules.

The graph enables checks that flat, single-file linters cannot do:

- Cyclic dependencies (module- and type-level)
- Dead/orphaned code (unreachable subgraphs)
- Layering violations (edges that cross declared architectural boundaries in the wrong direction)
- God classes / god functions (fan-in/fan-out and containment thresholds)
- Unstable dependencies (depending on modules more volatile than yourself)
- Interface segregation violations
- Naming and structural inconsistencies across sibling code paths

The concrete rule catalog v1 — each rule mapped to its required graph capabilities and its fix tier(s) — is open work (§10.2). The rule engine is the core intellectual property of the tool and is custom-built; ESLint's visitor pattern is a design inspiration, but rules here operate on graph nodes and edges, not AST nodes.

## 6. The three-tier fix system

### Tier 1: guaranteed behavior-preserving

Mechanism: **whitelist + graph re-verification.**

1. Only transforms from a hand-proven whitelist qualify (examples: remove unreachable code, sort imports, rename a private symbol together with all its references).
2. After applying a fix, strictcode re-parses the affected files, re-extracts the graph, and asserts the resulting semantic graph is isomorphic to the expected post-fix graph.

Two independent layers: proof by construction, plus mechanical verification. A transform whose re-verification fails is rolled back and reported as a tool bug — never silently accepted.

Running the project's own test suite after fixes is a possible optional extra layer, but is not the core mechanism (it depends on the user having tests, and "tests pass" is weaker than "behavior preserved").

### Tier 2: behavior-changing, arguably improved

Fixes that alter observable behavior in a way the rule argues is an improvement (e.g., tightening an over-permissive signature). Never applied automatically; requires explicit per-fix or per-rule consent.

### Tier 3: suggestion only

Findings where auto-fixing is impossible or unwise. The tool explains the problem, the evidence in the graph, and the recommended manual fix.

## 7. Python first

Python is the first matrix column: it has the largest ecosystem of messy codebases needing architecture enforcement, and its dynamic typing makes call resolution the hardest problem in the space — solving Python first means every subsequent language is easier.

### Call resolution: two explicit layers

1. **Syntactic + import-aware (always on).** Calls resolved through imports, module attributes, and class methods using strictcode's own symbol table. Dynamic dispatch (duck typing, monkey-patching, `getattr`) becomes an explicit `unresolved` edge — never a guess.
2. **Type-checker-backed (explicit opt-in mode).** Consumes type information from an external type checker (candidates: ty, pyright) to resolve method calls on inferred types. Dramatically better call graphs on typed codebases.

Per the no-silent-degradation principle: the presence of the type-checker mode in configuration is the choice. If configured, it must work — a missing or failing type checker is a hard error, never a silent downgrade to syntactic-only.

## 8. Implementation decisions

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

tree-sitter is the parsing foundation: MIT-licensed, 300+ language grammars, incremental, error-tolerant, universally adopted, very actively maintained. No parsing layer will be built or maintained in-house.

**Binding choice is open** (§10.3). Two candidates, to be evaluated hands-on against real Python codebases before committing:

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

## 9. Interface

### Output formats

- **Human-readable CLI text** — primary interface; findings grouped with locations, explanations, and suggested fixes.
- **JSON** — machine-readable structured output for editors, scripts, and AI agents.
- **Exit codes** — nonzero when findings exceed configured severity; usable as a CI gate and pre-commit hook with zero integration work.
- **SARIF** — deferred (todo filed in `todo/.defer/` at repo scaffolding time).

### Configuration

A config file controls: enabled rules, per-rule thresholds, severity levels, declared architectural layers/boundaries, and analysis modes (e.g., Python type-checker-backed resolution). Configuration selects modes explicitly; there are no implicit defaults for anything that changes analysis behavior.

## 10. Open work (in dependency order)

1. **Graph schema spec.** The versioned node/edge vocabulary everything else builds on. Full design upfront, before any extraction code.
2. **Rule catalog v1.** Concrete list of checks, each mapped to required graph capabilities and fix tier(s). This defines the feature axis of the matrix.
3. **Binding evaluation.** Benchmark gotreesitter vs official CGo bindings on real Python codebases: grammar fidelity, query support, throughput, memory. Commit to one.
4. **Repo scaffolding.** git init, rlsbl scaffold, Apache 2.0 LICENSE, `todo/` directory (including the deferred SARIF todo).
5. **Python extractor.** First matrix column: tree-sitter queries + symbol table for syntactic/import-aware resolution.
6. **Tier-1 fix mechanics detail.** The whitelist, the transform implementations, and the graph re-verification harness.

## 11. Settled project facts

| Item | Decision |
|---|---|
| Name | strictcode (verified available on npm and PyPI, 2026-07-21) |
| License | Apache 2.0 |
| Implementation language | Go |
| Parser | tree-sitter (binding choice pending evaluation) |
| First target language | Python |
| Rules | Built-in Go code, config toggles, no user DSL |
| Graph schema | Full versioned spec upfront |
| Tier-1 guarantee | Whitelisted transforms + post-apply graph re-verification |
| Persistence | None — stateless, rebuilt every run |
| Output | CLI text + JSON + exit codes; SARIF deferred |
| Location | `~/Projects/strictcode`, standalone repo |

## 12. Non-goals

- **No LLM in the analysis pipeline.** Ever. Determinism is the product.
- **No security scanning.** Joern, CodeQL, and Semgrep own that space; strictcode targets architecture, correctness, and discipline.
- **No user-extensible rule language.** Rules are code, maintained in-tree.
- **No watch mode / daemon / server** in the initial design. Batch CLI only.
- **No silent fallbacks of any kind.**
