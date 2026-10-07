+++
title = "library-stdout"
description = "library-stdout reports a library writing to standard output or standard error."
nav_group = "Rule reference"
nav_order = 110
+++

# `library-stdout`

:-: rule-facts id="library-stdout"

## Semantics

A library should report through its return values and a caller-supplied logger, never by writing
to the standard streams. The stream-writing calls the rule matches per language are on
[check semantics](../../check-semantics/). In Python, matching uses the canonicalized callee from
[call resolution](../../call-resolution/), so aliases such as `import sys as s` do not hide a
write, and module-level calls count too; in Go and TypeScript/JavaScript a call is matched by the
qualified name it is written with.

Runs only on members marked `library = true`, excluding test and example files by default.

## Origin

The donor's `library-lint` aggregate's standard-stream check, now its own rule in `group:library`.
Direct root-logger use, which the donor reported from the same check at lower severity, is the
separate [`library-direct-logging`](../library-direct-logging/).

## Configuring

This rule takes no suppressions. Individual callees can be exempted through the rule's
per-language `allow` lists, matched against the callee name as the rule reports it (a Go
`fmt.Fprint*` call as `fmt.Fprintln(os.Stderr)`):

```toml
[rules.library-stdout.allow]
py = ["sys.stderr.write"]
go = ["fmt.Fprintln(os.Stderr)"]
ts = ["console.error"]
```
