+++
title = "deps-stale"
description = "deps-stale reports an intra-workspace dependency constraint in a manifest that the dependency member's declared version no longer satisfies."
nav_group = "Rule reference"
nav_order = 55
+++

# `deps-stale`

:-: rule-facts id="deps-stale"

## Semantics

A member's manifest declares a dependency on another workspace member with a version constraint
(`"lib": "^1.2.0"` in `package.json`, `"lib>=1.2"` in `pyproject.toml`), and the dependency
member's own manifest declares a version the constraint does not admit. A release of the
dependency has left the constraint behind: an install from the registry resolves an older
version than the workspace builds against.

Only constraints resolved from a registry are judged. A path source (`name @ file:...`,
`file:../lib`) and a workspace-protocol source (`workspace:*`) resolve to the member itself.

The constraint is evaluated in its simple forms: one operator (`>=`, `>`, `<=`, `<`, `==`, `=`,
`~=`, `^`, or `~`) or none, followed by a dotted numeric version. A caret keeps the major version,
or the minor for a `0.x` constraint; a tilde and `~=` keep the major and minor. A constraint with
several conditions (a comma, `||`, or a space between parts), `!=`, a wildcard, or a pre-release
version is not evaluated and gives no finding, and neither does a dependency member whose manifest
declares no static version (a dynamic `pyproject.toml` version).

Go is not applicable: a `go.mod` requirement names the module version minimal version selection
resolves, and a member's `go.mod` declares no version of its own to compare it with.

## Origin

Moved from rlsbl's `deps-stale` check, which judged the same constraints from its dependency
graph. strictcode reads the constraints and versions from the manifests it already parses, and
matches a dependency to a member by the same rule its `declares_dependency` rows use. Environment
markers are removed before evaluation, so `"b~=1.4; python_version >= '3.11'"` is judged as
`~=1.4`.

## Suppressing

```toml
[[rules.deps-stale.suppressions]]
project = "app"
dep = "lib"
reason = "app stays on lib 1.x until its own migration ships."
```
