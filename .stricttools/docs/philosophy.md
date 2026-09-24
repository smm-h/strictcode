+++
title = "Philosophy"
description = "strictcode enforces discipline through hard constraints: deterministic, stateless, no silent degradation, hard errors, and justified suppressions."
nav_order = 30
+++

# Philosophy

strictcode is built for a world where AI agents are the primary readers and writers of code.
Agents take shortcuts, ignore warnings, and choose the path of least resistance, so the tool
enforces discipline through hard constraints rather than soft guidance.

- **Deterministic.** There is no LLM anywhere in the analysis pipeline. The same input always
  produces the same output.
- **Stateless.** The graph is rebuilt from scratch on every run. There is no cache and no
  persisted state, so there is no stale-state class of bug.
- **No silent degradation.** Analysis strategies are explicit modes selected in configuration,
  never runtime fallbacks. If a configured mode cannot run, that is a hard error, not a
  downgrade. [Call resolution](../call-resolution/) is the main example.
- **Hard errors, not warnings.** Findings at error severity fail the run. There is no
  `--skip-checks`, no `--ignore-warnings`, and no other bypass flag.
- **Honest about limits.** When the analysis cannot resolve something, such as a dynamic call,
  it records the site as unresolved rather than guessing. Every result is explainable.
- **Every suppression carries a reason.** Any configuration entry that silences a finding
  requires a non-empty `reason`, and a suppression that names something that no longer exists
  is itself an error (see [`stale-suppression`](../rules/stale-suppression/)).

The same principles govern strictcode's own code and files: rules are code rather than a user
language, schemas are validated documents rather than conventions, and generated artifacts are
checked for freshness by tests.
