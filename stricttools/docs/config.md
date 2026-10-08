+++
title = "Configuration"
description = "strictcode.toml declares what strictcode reads: analysis modes, allow lists, per-rule suppressions with mandatory reasons, the Python tool declarations, and the strictspec certificate. Every rule is switched through its strictcode:<rule id> option."
nav_order = 300
+++

# Configuration

strictcode reads one declarations file, `strictcode.toml`, from the analyzed directory (the
`--config` flag names a different file, resolved relative to that directory). The file is a
strictspec document validated by `schema/strictspec/config.schema.toml`; the tables on this page
are rendered from that schema.

The file declares; it switches nothing. Whether a rule runs, and at which severity, is the rule's
option (see [Options](#options)), the one sanctioned way to change how a family tool behaves.

A missing file means no declarations: no suppressions, no Python tool or certificate
declarations, and the syntactic call-resolution layer only. A file that is present but malformed
is a hard error (lesson 31): unknown keys, wrong types, and empty mandatory fields are never
coerced, defaulted, or skipped.

## Options

Every rule is an option `strictcode:<rule id>`, filed as an entry in a subject document under
`.strictmetadata/options/` at the repository root and read through strictspec's options readers.
A rule declared at error severity ranks `error > warn > off`, and one declared at warning
severity ranks `warn > off`; the option's default is the rule's severity, so a repository without
entries runs every rule. `warn` reports the rule's findings at warning severity, which never fails
the run, and `off` does not run the rule.

The adopted rules (`lint`, `format`, `type-check`, and `strictspec-certificate`) rank
`error > warn > off` and default to `off`: a repository adopts one by filing an entry. `lint`,
`format`, and `type-check` take a path scope naming one workspace member's directory, so a
workspace adopts them member by member; every other option takes no scope. An entry without a
scope is the value for every member that has no entry of its own.

```toml
# .strictmetadata/options/code.toml
format_version = 1

[[entry]]
id = "strictcode:lint"
scope = "core"
current = "error"
ideal = "error"
reason = "core adopts ruff"
```

An entry that strictspec refuses (an unknown option, a value the option does not rank, a scope
the option does not take, a scope that names no member's path, an entry equal to the default) is
a hard error, and so is a document that fails its shape.

## Top level

:-: schema-fields path="schema/strictspec/config.schema.toml" type="Config"

## Analysis modes

:-: schema-fields path="schema/strictspec/config.schema.toml" type="Analysis"

A mode key's presence is the choice. A configured mode that cannot run is designed to be a hard
error, never a downgrade; see [call resolution](../call-resolution/). The type-checker mode is not
implemented: strictcode accepts `python_call_resolution = "type-checker"` and then ignores it,
which is an open defect recorded in [decisions](../decisions/).

## Rules

:-: schema-fields path="schema/strictspec/config.schema.toml" type="RuleConfig"

```toml
[rules.library-forbidden-imports.allow]
py = ["click"]
```

A rule table carries no switch: `enabled` and `severity` are refused, and so are group tables.

## Python tools

:-: schema-fields path="schema/strictspec/config.schema.toml" type="PythonTool"

```toml
[python_tools.lint]
paths = ["core", "tools/gen"]

[python_tools.type-check]
cwd = "core"
paths = ["src", "tests"]
```

A `[python_tools.<rule>]` declaration names which command `lint`, `format`, or `type-check` runs
over which paths: `ruff check`, `ruff format --check`, or `mypy`, each through
`uv run --frozen --no-sync`, in `cwd` over `paths`. Paths are canonical and relative to `cwd`, and `cwd` is relative to the workspace
root. A declaration runs nothing on its own: the rule's option decides for which members it runs,
and a member whose option is on but whose directory no declared path lies in is refused, naming
the member's path. See the [lint](../rules/lint/), [format](../rules/format/), and
[type-check](../rules/type-check/) rule pages.

## strictspec certificate

:-: schema-fields path="schema/strictspec/config.schema.toml" type="StrictspecCertificate"

```toml
[strictspec_certificate]
certificate = "migrations/config-v2.certificate.json"
adjudication = "migrations/config-v2.adjudication.toml"
```

The declaration names the files the [`strictspec-certificate`](../rules/strictspec-certificate/)
rule reads. Declaring it switches nothing; the rule runs while its option is on, and is refused
while on with no declaration.

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
- a suppression whose fields do not match its rule's shape;
- any suppression on a rule whose shape is `none`;
- a path in a Python tool or certificate declaration that is not canonical.

A suppression naming something that no longer exists on disk or in the workspace is not a load
error. It is a finding of [`stale-suppression`](../rules/stale-suppression/), reported during
analysis, when the workspace is known.

## Workspace inputs

strictcode also reads rlsbl's release declarations
(`.strictmetadata/releasables/releasables.toml`) and each member's manifest as committed inputs. They are
described on [check semantics](../check-semantics/).
