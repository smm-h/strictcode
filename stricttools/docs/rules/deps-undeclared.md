+++
title = "deps-undeclared"
description = "deps-undeclared reports production source importing a workspace package the member's manifest does not declare."
nav_group = "Rule reference"
nav_order = 30
+++

# `deps-undeclared`

:-: rule-facts id="deps-undeclared"

## Semantics

Exempt: imports from test code, guarded optional imports (lesson 4), type-only imports (lesson 5),
and self-imports (a package importing its own submodules). Imports resolve to the workspace member
name even when the registry name differs (lesson 10), and a sibling member's source is never
counted against another member (lesson 13).

## Lineage

Kept from the donor under the same name.

## Suppressing

```toml
[[rules.deps-undeclared.suppressions]]
project = "docs-site"
dep = "theme"
reason = "The theme is vendored by the build step and never installed as a dependency."
```
