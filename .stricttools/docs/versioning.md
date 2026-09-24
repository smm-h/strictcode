+++
title = "Versioning"
description = "Every strictcode schema artifact is a strictspec document with an exact-match format_version; vocabulary content changes are versioned through strictspec enum sourcing."
nav_order = 150
+++

# Versioning

Every machine-read artifact strictcode owns is a strictspec document: it carries an integer
`format_version`, is validated by a strictspec schema, and is read through generated Go readers.
The format-version check is exact-match, and format changes are migrated declaratively. The
strictspec manifest, `strictspec.toml`, lists the schemas:

| Document | Schema |
|---|---|
| `schema/vocabulary.toml` | `schema/strictspec/vocabulary.schema.toml` |
| `schema/profiles/*.toml` | `schema/strictspec/profile.schema.toml` |
| the JSON findings output | `schema/strictspec/findings.schema.toml` |
| `schema/registry.json` | `schema/strictspec/registry.schema.toml` |
| `strictcode.toml` | `schema/strictspec/config.schema.toml` |

## Vocabulary content versioning

Adding or removing a node kind changes the vocabulary's content, not its file format, so the
vocabulary's own `format_version` would not notice. strictcode closes that gap with strictspec's
**enum sourcing** (strictspec decision 32): the profile, findings, and registry schemas declare
their kind and capability enums as sourced from `schema/vocabulary.toml`, and strictspec bakes
the current values into the generated readers. A vocabulary change makes the baked values stale,
which `strictspec check` reports as an error; regenerating changes those schemas' accepted
documents, which is a format change governed by strictspec's version rules.

There is no separate `vocabulary_version` field. The trade-off is intended: every vocabulary
addition, however harmless, moves consumers through a version boundary. That matches the
ecosystem's exact-pairing rules better than tolerance for "additive" changes would.

## Designed for iteration

Speculative kinds are expected to change (see [the vocabulary](../vocabulary/)). Because every
artifact is a strictspec document, changing a shape is a migration rather than a permanent
commitment.
