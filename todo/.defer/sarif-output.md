# SARIF output format

Filed 2026-08-03 at repo scaffolding time, deferred by design decision
(DESIGN.md §10: "SARIF — deferred").

## Context

strictcode's output formats are human-readable CLI text, machine-readable JSON
(the strictspec-checked findings format in `schema/strictspec/findings.schema.toml`),
and exit codes. SARIF (Static Analysis Results Interchange Format, OASIS
standard) is the interchange format consumed by GitHub code scanning, VS Code
SARIF viewers, and other CI/editor integrations.

## Problem

Without SARIF, strictcode findings cannot surface in GitHub's code-scanning UI
or standard SARIF tooling. The native JSON format covers scripting and agent
consumption but is strictcode-specific.

## Solution sketch

Add a `--format sarif` (or equivalent explicit output-mode selection) that
projects the findings feed into SARIF 2.1.0: one `run` per invocation, rules
from the registry (ID, severity mapping error→error, warning→warning, help text
from rule descriptions), results with `physicalLocation` (file + region from
byte spans converted to line/col), and fix data for tier-1/2 fixes as SARIF
`fix` objects where representable.

- Pros: free GitHub code-scanning integration; standard tooling.
- Cons: SARIF is large and fiddly; mapping tiered fixes onto SARIF's fix model
  needs care; not needed by any current consumer (rlsbl keys on exit code +
  native JSON).

## Affected files

- Output layer (findings projection) — new SARIF serializer.
- CLI surface — explicit format selection.
- Docs and the findings-format documentation.

## Effort

Small-to-medium: the findings feed already carries everything SARIF needs;
this is a serializer plus tests against the SARIF schema.

## Trigger for revisiting

A concrete consumer wanting GitHub code scanning or SARIF-based tooling.
