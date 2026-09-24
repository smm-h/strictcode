+++
title = "Capabilities and profiles"
description = "How strictcode decides what it supports per language: fine-grained capabilities bundled into layers, per-language profiles declaring construct mappings and statuses, and rules requiring capabilities."
nav_order = 130
+++

# Capabilities and profiles

strictcode's product surface is a matrix: one axis is languages, the other is rules and the
graph capabilities they depend on. The matrix is not maintained by hand. It is computed from
three declarations:

1. **Capabilities** in `schema/vocabulary.toml`: fine-grained units of extraction, such as
   `resolve-imports-internal` or `call-resolution-syntactic`.
2. **Profiles** in `schema/profiles/`: one per language, declaring how its constructs map onto
   the vocabulary and the status of every capability.
3. **Rule requirements** in the Go rule registry: which capabilities each rule requires and which
   it uses. See [rules](../rules/).

The result is on the [support matrix](../support-matrix/).

## Layers

Capabilities are bundled into named **layers** for presentation. A layer is nothing more than its
set of capabilities and carries no meaning of its own. A language can be supported at the import-graph
layer, enough for the dependency-hygiene rules, before it reaches the full semantic graph.

:-: vocab-kinds kind="layer"

## Capability statuses

A profile declares, for every capability in the vocabulary, one of:

- `supported`: the extractor exists and is tested.
- `planned`: designed but not implemented.
- `not-applicable`: meaningless in that language, with a mandatory `reason`.

A profile that omits a capability, or names one the vocabulary does not declare, fails
validation: both sets are closed. A status flips from `planned` to `supported` only when the
extractor ships with tests.

## Requires and uses

Each rule declares two capability lists:

- **requires**: every one must be `supported` for the rule to be supported in a language. If any
  is `planned`, the rule is planned there. If any is `not-applicable`, the rule is not applicable
  there, and the capability's reason is surfaced.
- **uses**: optional enrichment. A `not-applicable` capability in this list never blocks support;
  it means that facet of the diagnosis does not exist in that language. For example, Go has no
  guarded imports, so `deps-unused` is supported in Go and its guard-related refinement never
  fires there.

A rule may also carry an explicit per-language `not_applicable` override with a mandatory reason,
for cases where the capabilities are present but the diagnosis is meaningless in that ecosystem:
import cycles in Go, which the compiler rejects, are the example.

## Language profiles

### Python

:-: profile-constructs lang="py"

### Go

:-: profile-constructs lang="go"

### TypeScript/JavaScript

TypeScript and JavaScript share one profile: TypeScript is a superset of JavaScript, and the same
grammar family parses both. The profile spans two tree-sitter grammars, `typescript` and `tsx`,
because JSX conflicts with TypeScript type assertions upstream.

:-: profile-constructs lang="ts"
