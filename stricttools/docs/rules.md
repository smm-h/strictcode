+++
title = "Rules"
description = "strictcode's rule registry: built-in rules with mint-once IDs, groups, the requires/uses capability model, and how rules are added and retired."
nav_order = 200
+++

# Rules

Rules are built in: Go code shipped with the tool, each switched and given its severity through
its `strictcode:<rule id>` option, with its suppressions and allow lists declared in
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
suppressions, in option IDs (`strictcode:<rule id>`), and in the list
`strictcode registry rules` prints, which rlsbl reads. A registry has two lifecycle operations and no
others:

- **Adding a rule** is a minor release. It can make a previously passing build fail, because the
  tool got stricter and found real problems. That is intended: a new rule's option defaults to its
  severity, because a rule shipped off by default is the soft guidance strictcode exists to
  replace. The exceptions are the adopted rules, which run a tool or read a file a repository
  must first declare.
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

A group classifies several rules, written `group:<name>` so it can never be mistaken for a rule
ID. It switches nothing: each rule is switched through its own option. A finding never names a
group, and a suppression never targets one.

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
