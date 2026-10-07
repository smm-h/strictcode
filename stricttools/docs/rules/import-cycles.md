+++
title = "import-cycles"
description = "import-cycles reports import cycles among a member's modules: strongly connected components of two or more modules in the import graph."
nav_group = "Rule reference"
nav_order = 80
+++

# `import-cycles`

:-: rule-facts id="import-cycles"

## Semantics

Tarjan's strongly connected components over the module-import projection of one member, reporting
only components of two or more modules (lesson 21). Test modules are left out of the graph, so a
cycle among tests is not reported (lesson 47). It is not applicable to Go, whose compiler already
rejects import cycles (lesson 20).

## Lineage

Renamed from the donor's `circular-deps`. The old name suggested manifest dependencies, but the
rule checks the module import graph inside one member.

## Suppressing

A suppression names the set of modules forming the cycle, by logical name, and matches only a
reported cycle with that exact set:

```toml
[[rules.import-cycles.suppressions]]
modules = ["app.models", "app.signals"]
reason = "Django signal registration requires the mutual import; the cycle is resolved at app load."
```
