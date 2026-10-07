+++
title = "lint"
description = "lint runs ruff check over the paths a [python_tools.lint] declaration names and reports each violation; a repository adopts it member by member through its option."
nav_group = "Rule reference"
nav_order = 150
+++

# `lint`

:-: rule-facts id="lint"

## Semantics

`uv run ruff check --output-format=json --quiet <paths>` runs in the declaration's `cwd` over its
`paths` (see [configuration](../../config/#python-tools)), and each violation in ruff's JSON output
is one finding, at the file and line ruff names, with ruff's code and message. The `uv run` flags
reach ruff where the project declares it: `--group <name>` for a dependency group other than
`dev`, `--extra <name>` for an optional-dependencies extra, and none otherwise or inside a uv
workspace.

The rule is adopted, not on by default: `strictcode:lint` defaults to `off` and takes a path scope
naming one workspace member's directory. A finding takes the value of the member owning its file,
so a member at `warn` reports warnings while another fails the run. A member whose option is on
but whose directory no declared path lies in is refused, naming the member's path, and so is an
option on with no declaration at all.

A member nested inside a declared path is left out of that run (`--extend-exclude`); its files
belong to its own declared path.

ruff older than 0.15.20 is refused before the run: lint reads the JSON fields that version
writes. A run whose output is not ruff's JSON, or that exits non-zero naming no violation, is an
error carrying ruff's own output, never a pass.

## Origin

Merged from rlsbl's `lint` check, which ran the same command over the paths of its `checks.lint`
block, and its `ruff-lint` check, which ran ruff on every Python project with ruff's JSON output
and a version floor. A repository that ran `ruff-lint` gets a `[python_tools.lint]` declaration
and a `strictcode:lint` entry for each member it covered from `rlsbl migrate records`.

## Suppressing

The rule accepts no suppressions: ruff's own `noqa` comments and configuration decide what it
reports.
