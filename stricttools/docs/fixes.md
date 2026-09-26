+++
title = "Fixes"
description = "strictcode's three fix tiers, and how a tier-1 fix is planned, applied, re-extracted, verified by structural correspondence, and rolled back on any mismatch."
nav_order = 320
+++

# Fixes

Every finding offers a fix in one of three tiers.

## Tier 1: guaranteed behavior-preserving

Two independent layers make the guarantee:

1. **Proof by construction.** Only transforms on a hand-reviewed whitelist qualify.
2. **Mechanical verification.** After applying a fix, strictcode re-parses the edited files,
   re-extracts the graph, and checks it against the expected result. A mismatch rolls every edited
   file back and is reported as a tool bug, never silently accepted.

Running the project's own tests after a fix could be an optional extra layer, but it is not the
mechanism: it depends on the project having tests, and "the tests pass" is weaker than "behavior
is preserved".

The whitelist:

| Transform | Rule |
|---|---|
| Remove unreachable statements | [`unreachable-code`](../rules/unreachable-code/) |

### Verification by structural correspondence

Verification never compares raw IDs or locations, because a fix legitimately changes both.

- The **declared delta** is computed from the removal range: the rows sited inside it, the nodes
  whose `contains` rows are removed (transitively), and every row touching those nodes.
- The **expected** post-fix relation is the pre-fix relation with that delta applied.
- The **actual** post-fix relation is re-extracted from the edited files.
- They must be equal in [canonical form](../graph-model/). Kinds, IDs, and attributes are compared
  everywhere. Spans are compared in every file the fix did not edit, and ignored across every file
  it did.

Ignoring spans across the whole edited file, not only after the edit point, is deliberate. A rule
relative to the edit point is unsound: an enclosing definition's span can shrink to end just
before the edit, while its pre-fix span reached past it, and the two cannot be matched row by row.

A transform that consistently declares its own overreach would pass verification; catching that
is the whitelist review's job. Verification catches edits whose re-extracted reality differs from
what the removal range predicts: the test suite includes a sabotage transform that removes only a
function's header line, and verification rolls it back.

### Plan-time refusals

Some fixes are refused when planning, rather than being allowed to fail verification:

- a removal that would renumber same-name or same-hint siblings outside it (ordinal drift; see
  [node identity](../node-identity/)) stays a finding without a fix;
- nested unreachable regions merge into the outer removal;
- files with CRLF line endings are refused, because edits apply to LF-normalized bytes and
  rewriting would silently normalize the whole file.

Removal takes whole lines, from the first unreachable statement's line through the last one's,
including comments between them. Comments after the region are kept.

## Tier 2: behavior-changing, arguably improved

A fix that changes observable behavior in a way the rule argues is better, such as editing a
manifest's dependency declarations. It is never applied automatically: it requires explicit
consent per fix or per rule. Tier-2 fixes are planned for the dependency rules; the rule pages
list each rule's planned fixes. The consent flow is not built yet.

## Tier 3: suggestion only

For findings where auto-fixing is impossible or unwise, the finding explains the problem, the
evidence in the graph, and the recommended manual fix. Every rule offers at least this.
