+++
title = "deps-dev-in-production"
description = "deps-dev-in-production reports a dev-scoped dependency imported by production code."
nav_group = "Rule reference"
nav_order = 50
+++

# `deps-dev-in-production`

:-: rule-facts id="deps-dev-in-production"

## Semantics

A dependency declared with scope `dev` and imported by at least one non-test file. Guarded imports
are exempt: a guarded production import of a dev-scoped dependency is the legitimate optional
dependency pattern, which guard semantics exist to allow.

## Lineage

Renamed from the donor's `deps-dev-in-lib`. The old name said "lib", but the diagnosis applies to
production code in any kind of member.

## Suppressing

```toml
[[rules.deps-dev-in-production.suppressions]]
project = "cli"
dep = "profiler"
reason = "Imported only by the hidden --profile path, which release builds strip."
```
