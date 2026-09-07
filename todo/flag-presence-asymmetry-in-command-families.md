# Detect flag-presence asymmetry within a subcommand family

## Context

A recurring CLI-design irregularity: within one subcommand group, a large
majority of commands carry a certain flag (a scope selector, a consent
flag, a format option) while a few odd members do not. Sometimes the
asymmetry is deliberate (two commands are repository-scoped by nature
while nine siblings are per-target and rightly carry a `--target`-style
selector); sometimes it is an accident (a flag added family-wide except
where someone forgot). Either way the asymmetry accumulates silently:
each command is reviewed alone, and nobody looks at the family as a
distribution.

The motivating live case: a release-tooling CLI whose eleven-command
release group grew a scope selector on nine members over time, with two
deliberate exceptions — the asymmetry was only noticed by a human asking
"shouldn't all of them have this flag?", not by any tool.

## Problem

No check surfaces "this flag exists on most of a family but not all of
it" as a finding. The signal is cheap to compute wherever a
machine-readable CLI structure exists (a schema dump enumerating groups,
commands, and flags), and the question it raises — deliberate scope
difference, or omission? — is exactly the kind a maintainer should
answer once, explicitly, instead of never.

## Solution sketch

- Input: a machine-readable CLI description (a schema dump listing
  groups, their commands, and each command's flags), or introspection
  where strictcode already has structural access.
- For each command group, compute per-flag presence across siblings.
  A flag present on a clear majority (threshold, e.g. two-thirds) but
  absent from at least one sibling is a FINDING naming the flag, the
  carriers, and the exceptions.
- Findings are review-severity, not errors: exceptions are often
  deliberate. The suppression mechanism must be an explicit recorded
  declaration per (group, flag, exception) — "this command is
  deliberately without this flag because <reason>" — so a deliberate
  asymmetry becomes documentation instead of a re-flagged nag, and a NEW
  sibling missing the flag still fires.
- Consider the same distribution logic for other per-command properties
  (dry-run support, consent classification) once flags work.

## Affected

Wherever strictcode's checks and their input acquisition live; a new
check plus the exception-declaration format and docs.

## Effort

Small-medium: the distribution logic is trivial; the input plumbing and
the exception declaration design are the real work.
