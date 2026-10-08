+++
title = "dead-workspace-packages"
description = "dead-workspace-packages reports a library workspace member that no sibling member imports."
nav_group = "Rule reference"
nav_order = 70
+++

# `dead-workspace-packages`

:-: rule-facts id="dead-workspace-packages"

## Semantics

Only members marked `library = true` are candidates. Exempt (lesson 28): dev-only members,
non-library members (applications and command-line tools are consumers, not consumed), and
published releasable members, which are consumed through a registry. Self-imports never count. A
member imported only by test code gets a different message from one imported by nothing at all.

## Lineage

Kept from the donor under the same name.

## Suppressing

A suppression names the member:

```toml
[[rules.dead-workspace-packages.suppressions]]
member = "legacy-adapter"
reason = "Kept for the migration tool, which lives outside this workspace."
```
