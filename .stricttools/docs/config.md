+++
title = "Configuration"
description = "strictcode.toml: rule and group toggles, severities, allow lists, analysis modes, and per-rule suppressions, each with a mandatory reason."
nav_order = 300
+++

# Configuration

strictcode reads one configuration file, `strictcode.toml`, from the analyzed directory (the
`--config` flag names a different file, resolved relative to that directory). The file is a
strictspec document validated by `schema/strictspec/config.schema.toml`; the tables on this page
are rendered from that schema.

A missing file means the registry defaults: every rule enabled at its default severity, no
suppressions, and the syntactic call-resolution layer only. A file that is present but malformed
is a hard error (lesson 31): unknown keys, wrong types, and empty mandatory fields are never
coerced, defaulted, or skipped.

## Top level

:-: schema-fields path="schema/strictspec/config.schema.toml" type="Config"

## Analysis modes

:-: schema-fields path="schema/strictspec/config.schema.toml" type="Analysis"

A mode key's presence is the choice. A configured mode that cannot run is designed to be a hard
error, never a downgrade; see [call resolution](../call-resolution/). The type-checker mode is not
implemented: strictcode accepts `python_call_resolution = "type-checker"` and then ignores it,
which is an open defect recorded in [decisions](../decisions/).

## Groups

:-: schema-fields path="schema/strictspec/config.schema.toml" type="GroupConfig"

```toml
[groups.library]
severity = "warning"
```

Group settings apply first, and a rule's own settings override them.

## Rules

:-: schema-fields path="schema/strictspec/config.schema.toml" type="RuleConfig"

```toml
[rules.dead-modules]
severity = "error"

[rules.import-cycles]
enabled = false
```

## Suppressions

Every suppression carries a non-empty `reason`, and names its target in the shape its rule
declares. The rule tables on [rules](../rules/) show each rule's shape.

:-: schema-fields path="schema/strictspec/config.schema.toml" type="Suppression"

| Shape | Fields | Target |
|---|---|---|
| `path` | `path` | A file (Python, TypeScript/JavaScript) or a package directory (Go), relative to the workspace root. |
| `project-dep` | `project`, `dep` | A dependency of one workspace member. |
| `member-set` | `modules` | The set of modules forming one reported import cycle, by logical name. |
| `member` | `member` | One workspace member. |
| `none` | | The rule accepts no suppressions. |

```toml
[[rules.dead-modules.suppressions]]
path = "core/extractors/sql.py"
reason = "Registered by name in the extractor table and imported dynamically."
```

Suppression paths are relative to the workspace root, not to the member. There is one
convention, so a path never means different things in different members.

## Errors when the file loads

Beyond schema validation, these are hard errors when the configuration loads:

- a rule ID the registry does not know; a retired rule's error shows its retirement record,
  including its replacements and what to do;
- a group name that does not exist;
- a suppression whose fields do not match its rule's shape;
- any suppression on a rule whose shape is `none`.

A suppression naming something that no longer exists on disk or in the workspace is not a load
error. It is a finding of [`stale-suppression`](../rules/stale-suppression/), reported during
analysis, when the workspace is known.

## Workspace inputs

strictcode also reads `workspace.toml` and each member's manifest as committed inputs. They are
described on [check semantics](../check-semantics/).
