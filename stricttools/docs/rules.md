+++
title = "Rules"
description = "strictcode's rule registry: built-in rules with mint-once IDs, groups, the requires/uses capability model, and how rules are added and retired."
nav_order = 200
+++

# Rules

Rules are built in: Go code shipped with the tool, enabled, disabled, and tuned through
[configuration](../config/). There is no user rule language and no embedded query language.
Rules stay correct, testable, and fixable because they are code, and users cannot write broken
ones. The rule engine operates on graph nodes and rows, never on syntax trees, and every rule in
a run shares the one graph built for that run.

The registry lives in `internal/rules` and is dumped to `schema/registry.json` by
`strictcode registry dump`. A test compares the committed dump with the Go declarations, so the
dump, and every table on this site rendered from it, cannot fall out of step with the code.

## The rules

:-: rule-table

Each rule has its own page under the rule reference, with its registry facts, per-language
support, and semantics. The semantics shared across rules are on
[check semantics](../check-semantics/), and the regression requirements every rule must meet are
on [the lessons register](../lessons/).

## Rule identity

A rule ID is a flat, lowercase, hyphenated name that identifies one diagnosis and encodes
nothing else. Category, severity, fix tier, capabilities, and group membership are registry
metadata, so they can change without touching the ID.

The set of rule IDs is strictcode's API surface: IDs appear in findings, in configuration and
suppressions, and in rlsbl's check configuration. A registry has two lifecycle operations and no
others:

- **Adding a rule** is a minor release. It can make a previously passing build fail, because the
  tool got stricter and found real problems. That is intended: new rules ship enabled, because a
  rule shipped disabled by default is the soft guidance strictcode exists to replace.
- **Retiring a rule** is a breaking release. The ID is never reused and never deleted from the
  registry. Configuration or suppressions naming a retired ID are a hard error, and the error
  renders the retirement record: when the rule was retired, why, which rules replaced it, and what
  to do. One rule can be replaced by several.

There are no renames and no aliases: an old name that keeps working beside a new one would make
every reference ambiguous. Metadata changes (severity defaults, thresholds, capabilities, group
membership) are minor releases.

### Retired rules

:-: retired-rules

## Groups

A group is a convenience switch over several rules, written `group:<name>` so it can never be
mistaken for a rule ID. Configuration can enable, disable, or re-severity a group's members in one
entry. A finding never names a group, and a suppression never targets one.

:-: group-table

## Capabilities

Each rule declares the capabilities it requires and the ones it only uses. The model is described
on [capabilities and profiles](../capabilities-and-profiles/), and the result per language is on
the [support matrix](../support-matrix/).

## Future rules

Deep architectural rules are designed to arrive with full-semantic depth in more languages:
layering violations (edges crossing declared architectural boundaries in the wrong direction),
god classes and functions (fan-in, fan-out, and containment thresholds), unstable dependencies,
and interface-segregation violations. The identity scheme needs no change to accommodate them.
