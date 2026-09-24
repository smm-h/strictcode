+++
title = "strictcode"
description = "strictcode is a deterministic linter for architecture: it builds a graph of a codebase, enforces structural rules, and offers tiered auto-fixes."
nav_order = 10
+++

# strictcode

*A linter for architecture, with tiered auto-fixes.*

strictcode is a deterministic, non-LLM command-line tool. It scans a codebase with
tree-sitter, builds a semantic graph of its structure, and evaluates architectural rules
against that graph.

The premise: every modern programming language expresses programs through two basic
abstractions, callable units (functions, methods, and procedures) and type units (classes,
structs, interfaces, and enums). Any codebase can therefore be modeled as a graph whose nodes
are those units, plus the modules and projects that contain them, and whose edges are their
interactions: calls, imports, inheritance, references, and instantiation. The
[graph model](../graph-model/) describes how strictcode represents it.

## Fix tiers

Every finding comes with a fix in one of three tiers:

| Tier | Guarantee | How it is applied |
|---|---|---|
| 1 | Behavior-preserving, guaranteed | Auto-applicable. Only whitelisted, hand-proven transforms qualify, and after applying one, strictcode re-extracts the graph and verifies it against the expected result. |
| 2 | Behavior-changing, but arguably improved | Applied only with explicit consent. |
| 3 | Suggestion only | No auto-fix. The finding explains what is wrong and how to fix it by hand. |

[Fixes](../fixes/) describes the machinery.

## Languages

strictcode analyzes Python, Go, and TypeScript/JavaScript. What each rule supports in each
language is data, generated from the language profiles and the rule registry, and shown on the
[support matrix](../support-matrix/).

## Where to go next

- [Installation](../installation/) and [the CLI](../cli-and-output/) to run it.
- [Configuration](../config/) for `strictcode.toml`, suppressions, and analysis modes.
- [Rules](../rules/) for what strictcode reports, with one page per rule.
- [Philosophy](../philosophy/) and [non-goals](../non-goals/) for what it will and will not do.
- [Decisions](../decisions/) for why it is built the way it is, including the alternatives that were rejected.
