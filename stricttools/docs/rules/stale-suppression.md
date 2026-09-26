+++
title = "stale-suppression"
description = "stale-suppression reports a suppression in strictcode.toml that names a path, rule, dependency, or member that no longer exists."
nav_group = "Rule reference"
nav_order = 90
+++

# `stale-suppression`

:-: rule-facts id="stale-suppression"

## Semantics

Configuration rot is a defect, not noise (lessons 31 and 32). A suppression naming a file or
package that is gone from disk, or a `(project, dep)` pair or member that no longer exists, fails
the run. The finding points at the configuration file.

A suppression naming a rule the registry does not know is not this rule's finding: it is a hard
error when the configuration loads, and a retired rule's error shows its retirement record (see
[rules](../../rules/)).

This rule engages no language capability, so it applies to every project.

## Lineage

Renamed and widened from the donor's `dead-modules-stale`. The old name filed it under dead code,
but the diagnosis is configuration rot, and it covers every kind of suppression, not only
dead-module paths.

## Suppressing

It cannot be suppressed: suppressing the staleness check would defeat it.
