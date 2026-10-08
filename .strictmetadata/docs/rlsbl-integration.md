+++
title = "rlsbl integration"
description = "How rlsbl runs strictcode: a binary on PATH whose rule list it checks by capability, then whose analysis it runs over committed files, keyed on the exit code and the findings document."
nav_order = 330
+++

# rlsbl integration

rlsbl runs strictcode as a binary on `PATH`, never as a library: rlsbl analyzes no source, and
strictcode owns all source analysis (see the boundary on [check semantics](../check-semantics/)).
rlsbl's `strictcode` check does two things, in order.

1. **It checks capabilities, not versions.** It runs `strictcode registry rules --json`, a
   read-only command whose payload lists every rule strictcode implements, by rule ID, each with
   its option. It refuses when a rule rlsbl requires is missing from the list, naming each one. No
   version is compared: a strictcode built from source reports the version its `VERSION` file held
   before its release bumped it, so a version floor would refuse the binaries a release produces.
2. **It runs the analysis.** It runs `strictcode analyze <repository root> --json` and fails on a
   non-zero exit or any error-severity finding, rendering each finding's rule, path, and message.

The `registry rules` payload:

```json
{
  "format_version": 1,
  "strictcode_version": "0.5.0",
  "rules": [
    {
      "id": "deps-unused",
      "severity": "error",
      "description": "...",
      "option": {
        "id": "strictcode:deps-unused",
        "subject": "dependencies",
        "values": "error > warn > off",
        "default": "error",
        "scope": "none"
      }
    }
  ]
}
```

The `analyze` payload is the findings document described on [the CLI](../cli-and-output/): exit 0
is clean or warnings only, 1 is at least one error-severity finding, and 2 is a tool or
configuration error (a malformed `strictcode.toml`, a refused options entry, a tool that could
not run).

strictcode reads everything it needs from committed files: the members from rlsbl's release
declarations (`.strictmetadata/releasables/releasables.toml`), its own declarations from
`strictcode.toml`, and every rule's value from its `strictcode:<rule id>` options entry under
`.strictmetadata/options/`. rlsbl passes nothing else. Rule IDs are stable identifiers whose
lifecycle is on [rules](../rules/).
