+++
title = "dead-modules"
description = "dead-modules reports unreferenced Python modules and Go internal packages, and TypeScript files no entry point reaches."
nav_group = "Rule reference"
nav_order = 60
+++

# `dead-modules`

:-: rule-facts id="dead-modules"

## Semantics

The algorithm differs per language: union of imports for Python and Go, and reachability from
entry points for TypeScript/JavaScript. [Check semantics](../../check-semantics/) describes each,
including the Python export and `scripts/` exemptions, Go's `internal/` candidates with its test
helpers and main packages, the mapping of TypeScript build output back to its sources through
`tsconfig.json`, the abstention when no entry point resolves to source, and the rule that a
suppressed unit never keeps anything alive (lesson 14). Only the files git lists are read.

Most remaining findings on real code are units loaded dynamically, such as plugins, documentation
directives, or extractors registered by name. A path suppression is the intended remedy for those.

## Origin

Kept from the donor under the same name, with the donor's later false-positive fixes carried as
lessons 40 to 45.

## Suppressing

A suppression names a file (Python, TypeScript/JavaScript) or a package directory (Go), relative
to the workspace root:

```toml
[[rules.dead-modules.suppressions]]
path = "core/extractors/sql.py"
reason = "Registered by name in the extractor table and imported dynamically."
```
