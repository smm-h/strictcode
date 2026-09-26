+++
title = "CLI and output"
description = "strictcode's commands, their arguments and flags, the human and JSON outputs, the findings document, and the exit codes CI and rlsbl key on."
nav_order = 310
+++

# CLI and output

strictcode's command line is built on strictcli, so flag conventions are enforced when the
commands are registered, and `--help`, `--json`, and `--dump-schema` come from the framework. The
tables on this page are rendered from `.strictcli/schema.json`, the schema strictcli dumps.

## Commands

:-: table-commands

## `analyze`

:-: cli-command name="analyze"

Exit codes:

| Code | Meaning |
|---|---|
| 0 | No findings, or warnings only. |
| 1 | At least one error-severity finding. This is what CI and rlsbl act on. |
| 2 | A tool or configuration error. |

Severity is the threshold: a rule configured at `warning` stops failing runs. See
[configuration](../config/).

## `fix`

:-: cli-command name="fix"

Writing files is never an implicit default: one of `--apply` or `--preview` is mandatory, and a
negated spelling such as `--no-apply` selects nothing and is refused. `fix` exits 0 on success,
including when there is nothing to fix, and 2 on a tool or configuration error or when a fix fails
verification and is rolled back. [Fixes](../fixes/) describes what happens.

## `registry dump`

:-: cli-command name="registry dump"

The dump is committed, and a test fails when it is stale. Rerun the command after changing a rule
declaration and commit the result.

## Human output

Without `--json`, `analyze` prints one line per finding, sorted, in the form
`file:line: severity: message [rule-id]`. A finding that offers an automatic fix is followed by an
indented `fix (tier N): description` line. A summary line gives the counts, or the output is
`no findings`:

```
pkg/__init__.py:1: warning: module pkg is not imported by any production module and not exported by any __init__.py [dead-modules]
pkg/a.py:1: warning: module pkg.a is not imported by any production module and not exported by any __init__.py [dead-modules]
pkg/a.py:3: error: unreachable code: statements after an unconditional terminator [unreachable-code]
    fix (tier 1): Remove the unreachable statements (whitelisted transform; verified by post-fix graph re-extraction).

3 finding(s): 1 error(s), 2 warning(s)
```

That is the output for a one-package Python project whose `pkg/a.py` holds a statement after a
`return`; the run exits 1 because one finding is an error.

## JSON output

Under `--json`, `analyze` prints one strictcli JSON document on standard output, and its
`payload` member is the findings document below. The command declares the document's JSON
Schema, so a document that deviates from it fails the run rather than reaching a consumer.

The findings document is also a strictspec document, validated by
`schema/strictspec/findings.schema.toml`:

:-: schema-fields path="schema/strictspec/findings.schema.toml" type="Findings"

Each finding:

:-: schema-fields path="schema/strictspec/findings.schema.toml" type="Finding"

Its target, the node the finding is about:

:-: schema-fields path="schema/strictspec/findings.schema.toml" type="Target"

The fix it offers, when it offers one:

:-: schema-fields path="schema/strictspec/findings.schema.toml" type="Fix"

The target's `kind` values are sourced from the vocabulary, so a vocabulary change changes this
document's accepted values; see [versioning](../versioning/). SARIF output is deferred.
