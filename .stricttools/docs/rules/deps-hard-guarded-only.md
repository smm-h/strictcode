+++
title = "deps-hard-guarded-only"
description = "deps-hard-guarded-only reports a hard (runtime or explicit) dependency imported only under optional-import guards, which is contradictory."
nav_group = "Rule reference"
nav_order = 20
+++

# `deps-hard-guarded-only`

:-: rule-facts id="deps-hard-guarded-only"

## Semantics

A dependency declared with scope `runtime` or `explicit` promises the package is always installed.
Importing it only inside `try/except ImportError` says it may be missing. The two cannot both be
true, so the finding says to either declare the dependency optional or import it unconditionally
(lesson 2). Guards satisfy only `dev` and `peer` dependencies.

The rule requires guarded-import classification, so it is not applicable in languages without an
optional-import construct; see the support table above.

## Lineage

Split out of the donor's `deps-unused`, where it was special-case messaging, because it is a
distinct diagnosis with its own remedy.

## Suppressing

```toml
[[rules.deps-hard-guarded-only.suppressions]]
project = "cli"
dep = "telemetry"
reason = "Telemetry is required in production images; the guard exists only for the offline test harness."
```
