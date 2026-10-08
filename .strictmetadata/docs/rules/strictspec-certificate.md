+++
title = "strictspec-certificate"
description = "strictspec-certificate reports each reason a declared strictspec diff certificate blocks a schema's format_version change: a violated claim, an unsupported claim no adjudication discharges, or a dangling adjudication entry."
nav_group = "Rule reference"
nav_order = 210
+++

# `strictspec-certificate`

:-: rule-facts id="strictspec-certificate"

## Semantics

`strictspec diff` writes a certificate for a schema's move from one `format_version` to the next:
a JSON document whose `claims` each carry a grade. This rule reads the certificate the
`[strictspec_certificate]` declaration names (see [configuration](../../config/#strictspec-certificate))
and reports, at the error severity, each reason it blocks:

- a claim graded `violated`, naming its counterexample documents;
- a claim with any grade other than `violated`, `corpus-supported`, or `proven` (an unsupported
  claim) that no entry of the declared adjudication file discharges; an entry discharges a claim
  when its `claim_kind` equals the claim's `kind` and its `scope` equals the claim's `statement`;
- an adjudication entry that discharges no unsupported claim (a dangling entry).

The adjudication file is a strictspec document, validated against the adjudication schema
strictcode embeds. A missing or unreadable certificate or adjudication file, a certificate that is
not a JSON object with a `claims` array, and an adjudication file that fails its schema are
errors, never a pass.

The rule is adopted: `strictcode:strictspec-certificate` defaults to `off`, and switched on with
no `[strictspec_certificate]` declaration it is refused. The declaration switches nothing on its
own.

This rule engages no language capability, so it applies to every project.

## Origin

Moved from rlsbl's `strictspec-certificate-gate` check, whose `strictspec_gate` section named the
same two files.

## Suppressing

The rule accepts no suppressions: discharge an unsupported claim with an adjudication entry.
