+++
title = "Non-goals"
description = "What strictcode deliberately does not do: no LLM, no security scanning, no user rule language, no daemon, no fallbacks, and no release duties."
nav_order = 40
+++

# Non-goals

- **No LLM in the analysis pipeline.** Determinism is the product.
- **No security scanning.** Joern, CodeQL, and Semgrep own that space. strictcode targets
  architecture, correctness, and discipline. [Prior art](../prior-art/) covers these tools.
- **No user-extensible rule language.** Rules are Go code maintained in the repository, with
  configuration limited to toggles, severities, thresholds, and suppressions.
- **No watch mode, daemon, or server.** strictcode is a batch command-line tool.
- **No silent fallbacks of any kind,** including no regular-expression parsing backend.
- **No speculative language support.** Dart, Java, Kotlin, and every other language wait for a
  demonstrated need.
- **No release orchestration, changelog, scaffolding, or registry duties.** Those belong to
  rlsbl. strictcode is the source-analysis specialist that rlsbl delegates to; see
  [rlsbl integration](../rlsbl-integration/).
