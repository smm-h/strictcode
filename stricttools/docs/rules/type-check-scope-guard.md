+++
title = "type-check-scope-guard"
description = "type-check-scope-guard reports mypy configuration that competes with the paths a [python_tools.type-check] declaration names."
nav_group = "Rule reference"
nav_order = 200
+++

# `type-check-scope-guard`

:-: rule-facts id="type-check-scope-guard"

## Semantics

While [`type-check`](../type-check/) runs, this rule reads mypy's own configuration in the
declaration's `cwd` and reports `files`, `packages`, or `modules` in `pyproject.toml` `[tool.mypy]`, the `[mypy]` section of `mypy.ini` or `.mypy.ini`, or `setup.cfg` `[mypy]`, at the key's line: paths on mypy's command line silently override its files, packages, and modules settings, so a scope declared there is dead but reads as authoritative. Scope belongs in the declared `paths`
alone.

Keys that exclude (`exclude`, `extend-exclude`, `force-exclude`) are exempt: an explicit path
bypasses them, which over-includes loudly instead of under-scoping silently.

The rule's own option keeps its default, `error`; it reports nothing while `type-check` is off,
since there is no declared scope to compete with.

## Origin

Moved from rlsbl's `type-check-scope-guard` check.

## Suppressing

The rule accepts no suppressions: move the scope into the declaration's `paths`.
