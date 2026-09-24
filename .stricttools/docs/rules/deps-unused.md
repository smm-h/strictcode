+++
title = "deps-unused"
description = "deps-unused reports a workspace-internal dependency that a member's manifest declares but no source file imports."
nav_group = "Rule reference"
nav_order = 10
+++

# `deps-unused`

:-: rule-facts id="deps-unused"

## Semantics

Only workspace-internal dependencies are considered: the declared name must resolve to another
workspace member. External registry dependencies are ignored.

Any import of the dependency marks it used, including guarded imports and imports from test code
(lessons 1 and 8). Only type-only imports under `if TYPE_CHECKING:` never count (lesson 5). A hard
dependency imported only under guards is used, but contradictory, and is reported by
[`deps-hard-guarded-only`](../deps-hard-guarded-only/) instead, so the same dependency is never
reported twice with conflicting messages.

Import resolution follows [check semantics](../../check-semantics/).

## Lineage

Kept from the donor under the same name.

## Suppressing

A suppression names the member and the dependency:

```toml
[[rules.deps-unused.suppressions]]
project = "transport"
dep = "codec"
reason = "Loaded through the plugin entry-point table, which no import statement names."
```
