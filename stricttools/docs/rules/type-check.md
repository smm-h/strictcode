+++
title = "type-check"
description = "type-check runs mypy over the paths a [python_tools.type-check] declaration names and reports each error line mypy prints."
nav_group = "Rule reference"
nav_order = 190
+++

# `type-check`

:-: rule-facts id="type-check"

## Semantics

`uv run --frozen --no-sync mypy <paths>` runs in the declaration's `cwd` over its `paths`, and each
`<file>:<line>: error: <message>` line mypy prints is one finding at that file and line. mypy
exits 1 when it found errors; any other non-zero exit, or an exit 1 with no error line, is an
error carrying mypy's output. Notes are not findings.

Adoption, path scopes, the `uv run` flags, the refusal of a missing environment, and the other
refusals work as for [`lint`](../lint/).

## Origin

Moved from rlsbl's `type-check` check, which ran the same command over the paths of its
`checks.type-check` block.

## Suppressing

The rule accepts no suppressions: mypy's own `type: ignore` comments and configuration decide
what it reports.
