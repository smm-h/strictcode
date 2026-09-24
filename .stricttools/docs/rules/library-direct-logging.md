+++
title = "library-direct-logging"
description = "library-direct-logging reports a Python library calling the root logger directly instead of taking a logger."
nav_group = "Rule reference"
nav_order = 120
+++

# `library-direct-logging`

:-: rule-facts id="library-direct-logging"

## Semantics

Calls such as `logging.info(...)` in a library write to the root logger, which the application
owns. A library should accept a logger or create a named one. Direct logging is a warning while
standard-stream writes are errors (lesson 27).

The diagnosis is specific to Python's root-logger idiom, so it is not applicable to Go or
TypeScript/JavaScript.

## Lineage

Split out of the donor's standard-stream check in the `library-lint` aggregate, because it is a
distinct diagnosis with a distinct severity.

## Configuring

This rule takes no suppressions. Individual callees can be exempted through the rule's Python
`allow` list, matched against the canonical callee name:

```toml
[rules.library-direct-logging.allow]
py = ["logging.captureWarnings"]
```
