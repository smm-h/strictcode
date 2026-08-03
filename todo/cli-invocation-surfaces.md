# Model CLI invocations in non-code surfaces (shell, CI YAML, Makefiles, prompts)

Filed 2026-08-03.

## Context

The design models a graph of callables and types over Python/Go/TS-JS via tree-sitter, with tiered auto-fixes (Tier 1 guaranteed behavior-preserving). That node model covers *programmatic* call sites.

There is an emerging class of interface-migration work it cannot serve: command-line tools whose interface surface is published as a machine-readable schema per release, with graded per-version deltas (removed flags, renamed commands, type changes) planned upstream. Turning such a delta into automated fixes requires finding the *invocations* of that CLI across a codebase — and CLI invocations overwhelmingly do not live in Python/Go/TS source. They live in:

- shell scripts (`scripts/*.sh`, hooks)
- CI workflow YAML (`run:` blocks)
- Makefiles / justfiles
- Dockerfiles (`RUN` lines)
- documentation and agent-instruction markdown (fenced code blocks, inline commands)

None of these are modeled by a callable-and-type graph.

## Problem

"Scan the codebase, identify affected usages, fix them" is exactly the auto-fix shape the design is built for, but for CLI surface changes the call sites are invisible to the planned graph. Without a decision here, the tool will silently cover only the least common kind of CLI call site (subprocess invocations in code) and miss the dominant ones.

## Options

**A. First-class invocation nodes (recommended).** Extend the node model with a `command invocation` node kind, populated by dedicated extractors per surface (shell via tree-sitter-bash, YAML `run:` blocks, Makefile recipes, Dockerfile `RUN`, fenced blocks in markdown). Fixes against these nodes are Tier 2 at best in prose/docs (not provably behavior-preserving) and can be Tier 1 in structured surfaces (shell, Make) when the schema delta is mechanical (flag rename with same arity).
- Pros: makes the tool the consumer-side engine for schema-driven CLI migrations; extractors are individually small; shell/YAML/Make have real grammars.
- Cons: new extractor class beside the code graph; markdown/prose matching is inherently fuzzy and needs a confidence model; scope growth in the design phase.

**B. Separate scanner tool.** Keep the graph code-only; build CLI-invocation scanning as an independent tool that shares nothing.
- Pros: protects the core design's coherence; scanner can ship earlier.
- Cons: duplicates file-walking, config, and fix-application machinery; two tools to integrate for one migration; the fix-tier framework (the valuable part) would be reimplemented or skipped.

**C. Explicitly scope out.** Document that CLI invocations in non-code surfaces are out of scope.
- Pros: zero cost; honest.
- Cons: forfeits the most concrete near-term auto-fix use case the ecosystem has; someone builds B anyway, badly.

## Affected files

- `DESIGN.md` — node model section (new node kind), extractor architecture, fix-tier assignments per surface, scope statement either way.

## Effort

Small-to-medium as a design amendment (this project is design-phase; no code exists). Implementation cost lands later with the rest of the tool; extractors are parallelizable and individually small.
