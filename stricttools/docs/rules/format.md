+++
title = "format"
description = "format runs ruff format --check over the paths a [python_tools.format] declaration names and reports each file ruff would reformat."
nav_group = "Rule reference"
nav_order = 170
+++

# `format`

:-: rule-facts id="format"

## Semantics

`uv run --frozen --no-sync ruff format --check <paths>` runs in the declaration's `cwd` over its
`paths`, and each `Would reformat: <file>` line is one finding at that file. ruff exits 1 when it
would reformat a file; any other non-zero exit, or an exit 1 naming no file, is an error carrying
ruff's output.

Adoption, path scopes, the `uv run` flags, the refusal of a missing environment, the other
refusals, and nested members work as for [`lint`](../lint/).

## Origin

Moved from rlsbl's `format` check, which ran the same command over the paths of its
`checks.format` block.

## Suppressing

The rule accepts no suppressions: ruff's own configuration decides what it formats.
