+++
title = "rlsbl integration"
description = "How rlsbl consumes strictcode as an external check: a subprocess reading only committed files, keyed on the exit code, with stable rule IDs and JSON output."
nav_order = 330
+++

# rlsbl integration

strictcode is designed to be consumed by rlsbl through rlsbl's external-check protocol. From
rlsbl's side it is an ordinary external check that happens to own all source analysis (see the
boundary on [check semantics](../check-semantics/)). The contract strictcode guarantees:

- It runs as a subprocess in a project or workspace directory. Everything it needs is on disk and
  committed; rlsbl passes no parsed data at runtime.
- The **exit code** is what rlsbl acts on: 0 passes, nonzero fails, and there is no bypass. See
  [the CLI](../cli-and-output/).
- Human-readable text goes to standard output for logs, and `--json` gives the machine-readable
  findings document.
- Rule IDs are stable identifiers (lowercase and hyphenated), so rlsbl configuration can reference
  them and order checks with `depends_on`. Their lifecycle is on [rules](../rules/).

The preferred long-term shape is a first-class `structured` adapter (`tool = "strictcode"`) in
rlsbl's adapter table, which gives argument composition, budgeted timeouts, and dependency
ordering; a `freeform` command entry works without it. Which one, and when, is rlsbl's decision.

The handoff has not happened: rlsbl still runs its own source scanners. Two things stand between
them: the adapter entry on rlsbl's side, and prebuilt strictcode binaries, since rlsbl is a Python
tool and a source-only strictcode would force a Go toolchain on every project
rlsbl manages. See [distribution](../distribution/).
