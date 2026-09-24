+++
title = "Vocabulary"
description = "The node kinds, row kinds, attributes, and enumerations strictcode's graph is built from, rendered from schema/vocabulary.toml, plus how subtyping and import attributes work."
nav_order = 120
+++

# Vocabulary

The vocabulary is one universal set of kinds shared by every language, declared in
`schema/vocabulary.toml` and validated by `schema/strictspec/vocabulary.schema.toml`. The tables
on this page are rendered from that file at docs-build time. Language-specific nuance lives in
[profiles](../capabilities-and-profiles/), never in new per-language kinds.

Each kind carries a **maturity**:

- `stable`: exercised by an extractor or a rule.
- `speculative`: designed ahead of use and expected to change. Its existence is settled; its
  attributes are provisional by declaration.

## Node kinds

:-: vocab-kinds kind="node"

Tests and decorators are not node kinds. A test is a function with `is_test = true` plus
`validates` rows, and a decorator is a function plus `decorates` rows: when the underlying thing
already has a kind, a relationship is better than another kind.

## Row kinds

:-: vocab-kinds kind="row"

Only resolved calls between two local callables become `calls` rows. External and unresolved
call sites live in a side table; see [the graph model](../graph-model/).

## Enumerations

:-: vocab-enums

## Import attributes

Every `imports` row carries three mandatory booleans:

- `test_context`: the importing file is test code, per the shared predicate in
  [check semantics](../check-semantics/).
- `guarded`: the import sits in the try body of a Python `try` whose `except` catches
  `ImportError` or `ModuleNotFoundError`. An import in the `except` body is a fallback import and
  is not guarded.
- `type_checking`: the import is type-only: Python's `if TYPE_CHECKING:` or TypeScript's
  `import type`.

A profile that marks one of these capabilities not applicable always emits the attribute as
`false`, and the support matrix records the capability as not applicable, so the attribute never
silently means something weaker.

## Subtyping: `conforms_to`

All subtype and conformance relationships are one row kind, `conforms_to`, with three mandatory,
independent attributes:

:-: vocab-enums ids="provenance, discipline, mechanism"

- The `provenance` attribute says who asserted the relationship: the source (`declared`), the
  source out of band, as with Python's `ABC.register` (`declared_external`), or strictcode's own
  analysis (`derived`).
- `discipline` says whether the relationship is nominal or structural.
- `mechanism` says how it arises.

Declared rows are stored by extraction. Derived rows, such as Go's implicit interface
satisfaction, TypeScript's structural compatibility, and Python's `Protocol` conformance, are
designed to be materialized lazily, only when an enabled rule requires the
`conformance-derived` capability and the language's profile grants it. The gap between declared
and actual conformance ("claims X but does not satisfy it", "satisfies X but never declares it")
is designed as a projection, not a stored row. Derived materialization is not built yet; the
[support matrix](../support-matrix/) shows `conformance-derived` as planned.

Examples of the mapping:

| Construct | Row |
|---|---|
| Python `class B(A)` | `declared`, `nominal`, `inheritance` |
| Python `A.register(C)` | `declared_external`, `nominal`, `register` |
| TypeScript `class C implements I` | `declared`, `nominal`, `implements` |
| Go struct embedding | `declared`, `nominal`, `embedding` |
| Go implicit interface satisfaction | `derived`, `structural`, `satisfaction` |
| Python `typing.Protocol` conformance | `derived`, `structural`, `protocol` |

Each language's full construct mapping is on [capabilities and profiles](../capabilities-and-profiles/).
