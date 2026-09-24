+++
title = "deps-runtime-test-only"
description = "deps-runtime-test-only reports a runtime-scoped dependency that only test code imports, which should be a dev dependency."
nav_group = "Rule reference"
nav_order = 40
+++

# `deps-runtime-test-only`

:-: rule-facts id="deps-runtime-test-only"

## Semantics

A dependency declared with scope `runtime` whose every import comes from test code, per the
test-context predicate on [check semantics](../../check-semantics/). Moving it to a dev scope keeps
it out of installs that never run the tests.

## Lineage

Kept from the donor under the same name.

## Suppressing

```toml
[[rules.deps-runtime-test-only.suppressions]]
project = "server"
dep = "fixtures"
reason = "Shipped at runtime on purpose: the demo mode loads the test fixtures."
```
