+++
title = "lint-scope-guard"
description = "lint-scope-guard reports ruff configuration that competes with the paths a [python_tools.lint] declaration names."
nav_group = "Rule reference"
nav_order = 160
+++

# `lint-scope-guard`

:-: rule-facts id="lint-scope-guard"

## Semantics

While [`lint`](../lint/) runs, this rule reads ruff's own configuration in the
declaration's `cwd` and reports `include` or `extend-include` in `pyproject.toml` `[tool.ruff]`, `ruff.toml`, or `.ruff.toml`, at the key's line: ruff's include and extend-include silently narrow the directories passed on its command line, so the declared scope would shrink without anyone seeing it. Scope belongs in the declared `paths`
alone.

Keys that exclude (`exclude`, `extend-exclude`, `force-exclude`) are exempt: an explicit path
bypasses them, which over-includes loudly instead of under-scoping silently.

The rule's own option keeps its default, `error`; it reports nothing while `lint` is off,
since there is no declared scope to compete with.

## Origin

Moved from rlsbl's `lint-scope-guard` check.

## Suppressing

The rule accepts no suppressions: move the scope into the declaration's `paths`.
