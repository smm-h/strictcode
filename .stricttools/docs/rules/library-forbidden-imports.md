+++
title = "library-forbidden-imports"
description = "library-forbidden-imports reports a library importing an application-concern module, such as a CLI framework or a web server."
nav_group = "Rule reference"
nav_order = 100
+++

# `library-forbidden-imports`

:-: rule-facts id="library-forbidden-imports"

## Semantics

Runs only on members marked `library = true` (lesson 22), excluding test and example files by
default (lesson 23). The per-language default lists, and how the allow lists are subtracted from
them (lesson 26), are on [check semantics](../../check-semantics/).

## Lineage

The donor's `library-lint` aggregate's forbidden-imports check, now its own rule in
`group:library`.

## Configuring

This rule takes no suppressions. Exceptions are rule options: a per-language `allow` list, and a
per-language `forbidden` list that replaces the default.

```toml
[rules.library-forbidden-imports.allow]
py = ["click"]
```
