+++
title = "Graph model"
description = "strictcode's source of truth is a flat typed interaction relation plus a node table; the algorithm graph, findings, and JSON output are deterministic projections of it."
nav_order = 100
+++

# Graph model

## The interaction relation

Extraction produces a flat, typed **interaction relation**: an ordered set of rows, each

```
(row_kind, src_node, dst_node, file, span, attrs...)
```

Nodes live in a companion **node table** of `(node_kind, id, attrs...)` entries. The kinds of
rows and nodes, and the attributes each carries, are declared in `schema/vocabulary.toml`; the
[vocabulary](../vocabulary/) page renders them.

Everything else is a pure, deterministic projection of the relation. None of the projections
is primary, and no consumer reads rows it was not handed through one:

- the **algorithm graph**, the distinct `(src, dst)` pairs per row kind, which graph algorithms
  such as Tarjan's strongly connected components and reachability run on;
- the **site feed**, the rows with their spans and attributes, which findings and fixes read;
- the JSON findings output described in [the CLI](../cli-and-output/).

[Decisions](../decisions/) records why a relation was chosen over a multigraph, collapsed edges
with site lists, and the other designs considered.

## Side tables for sites without two nodes

A row needs both endpoints in the node table. Some sites have no node to point at, so the
extractors carry them in side tables instead of rows:

- **External imports**: imports of packages outside the workspace, such as `flask`,
  `net/http`, or `express`. `library-forbidden-imports` reads these.
- **External and unresolved calls**: calls to the standard library, builtins, or external
  packages, and calls whose target cannot be resolved. `library-stdout` and
  `library-direct-logging` read these. Only resolved calls between two local callables become
  `calls` rows.

The vocabulary's `resolution = "unresolved"` value has no relation rows for this reason.
Representing unresolved calls inside the relation needs a vocabulary change, such as a node kind
for unresolved targets or an optional destination, and is recorded as open design work in
[decisions](../decisions/).

## Canonical form

The canonical form of a graph build is defined once, on the relation:

1. a version line,
2. the node table sorted by `(node_kind, id)`,
3. the rows sorted by the total key
   `(row_kind, src_id, dst_id, file, span_start, span_end, attribute tuple)`,

serialized canonically and hashed with SHA-256. Every projection inherits its determinism from
this. [Fixes](../fixes/) uses the canonical form to verify tier-1 fixes.

## Spans and positions

A position is a byte span `(start_byte, end_byte)` over the file's LF-normalized UTF-8 bytes.
The parser layer normalizes line endings before parsing, so every span in the system indexes the
same bytes. Line and column numbers (1-based) are derived only at output time, for people and
editors. They are never stored as truth and never enter the canonical hash.

## Identity

Nodes are identified by hierarchical qualified names, never by location or content. See
[node identity](../node-identity/).
