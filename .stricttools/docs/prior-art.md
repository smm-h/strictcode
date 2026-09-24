+++
title = "Prior art"
description = "Tools that model code as graphs, as surveyed while designing strictcode: competitors, possible foundations and their licenses, index formats, and why strictcode builds its own graph."
nav_order = 430
+++

# Prior art

This survey was made while designing strictcode, in July 2026. Licenses and maintenance status
change, so treat the details as a record of that survey rather than a current listing.

## Architecture and dependency analysis

These are the closest to what strictcode does:

| Tool | License | What it does |
|---|---|---|
| SciTools Understand | Commercial | Call graphs, dependency graphs, control-flow graphs, and UML views across many languages. The feature benchmark for code comprehension. |
| NDepend | Commercial | .NET dependency graphs and matrices, a LINQ-based query language, metrics, and architecture rule enforcement. The closest in spirit, for .NET only. |
| Lattix | Commercial | Design structure matrices, layering and module-boundary enforcement, and impact analysis. |
| Arcan | Academic and commercial | Dependency graphs for Java, C, and C++, with metrics and architectural smells such as hub-like and cyclic dependencies. |
| Tach | MIT | Python module-boundary enforcement declared in `tach.toml`, at module granularity only. |
| CodeScene | Commercial | Technical-debt prioritization combining code structure with version-control history. |

strictcode differs in being deterministic and language-agnostic by design, with a generated
language-by-rule support matrix, and in offering fixes in explicit safety tiers, the first of
which is mechanically verified.

## Security-oriented code graphs

| Tool | License | Notes |
|---|---|---|
| Joern | Apache 2.0 | Code property graphs merging syntax trees, control flow, and program dependence, queried with a Scala DSL. Security-focused and JVM-based. |
| CodeQL | Queries MIT; engine proprietary | A relational database of syntax, data flow, control flow, and types. Building a product on it for private code needs a commercial license. |
| Semgrep | Engine LGPL-2.1; rules under their own license | Pattern-based analysis with cross-file taint tracking. It finds patterns rather than relationships. |

These are out of scope; see [non-goals](../non-goals/).

## Code intelligence and index formats

| Tool | License | Notes |
|---|---|---|
| SCIP | Apache 2.0 | Sourcegraph's language-agnostic index of definitions, references, and symbols. It gives definition and reference edges, but no call graph or data flow. |
| Glean | BSD | Meta's store of typed facts about code, queried with Angle, with a Haskell core. |
| Kythe | Apache 2.0 | Google's graph of semantic nodes and edges from instrumented builds. |

## Graph storage options considered

| Option | License | Why it was not chosen |
|---|---|---|
| Neo4j Community | GPLv3 | Copyleft reaches anything linking to it, and it needs a server. |
| FalkorDB | SSPL | Problematic for embedding, and needs a server. |
| NetworkX, rustworkx, petgraph | BSD, Apache 2.0, MIT or Apache 2.0 | In-memory libraries; unnecessary for nodes, rows, and traversals strictcode implements directly. |

## Why strictcode builds its own graph

Joern and CodeQL build richer graphs, but they are security-oriented, heavy (JVM or a proprietary
engine), and shaped around vulnerability queries. strictcode needs a graph shaped around
architecture: workspace members, declared dependencies, entry points, and import attributes such
as guarded and type-only imports. Building it on tree-sitter keeps that model fully under its own
control, and the extraction rules are the core of the tool anyway. See
[implementation](../implementation/).
