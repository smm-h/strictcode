# A VS Code extension that surfaces every strictcode finding in the editor

## Context

strictcode is a deterministic linter for architecture: it scans a workspace
with tree-sitter (pure-Go grammars), builds one graph of modules, callables,
and types, and runs built-in rules over it across Python, Go, and
TypeScript/JavaScript: dependency hygiene (`deps-undeclared`, `deps-unused`,
`deps-dev-in-production`, and the rest), dead code (`dead-modules`,
`dead-workspace-packages`, `unreachable-code`), `import-cycles`, library
boundaries (`library-stdout`, `library-direct-logging`,
`library-forbidden-imports`, `library-entry-point`), and
`stale-suppression`. The rule set is `internal/rules` and its committed dump
is `schema/registry.json`. Every finding offers a fix in one of three tiers,
and tier-1 fixes are applied by `strictcode fix --apply` and verified by
re-extracting the graph. Configuration is `strictcode.toml`: rule and group
toggles, severities, analysis modes, and suppressions that each carry a
mandatory reason.

The findings are reachable only from the command line, as text lines or as
the `--json` findings document. The people and agents who would act on a
finding are usually in an editor looking at the file it names.

## Problem

- A finding is seen only after someone runs `strictcode analyze`, reads the
  output, and navigates to `file:line` by hand.
- Architectural findings are not local to one line: an import cycle spans
  several modules, `deps-unused` points at a manifest, `dead-modules` points
  at a whole file. A terminal list does not show these relations.
- Adding a suppression means hand-writing a TOML entry in the shape the rule
  declares (`path`, `project-dep`, `member-set`, `member`), with a reason; a
  wrong shape is a load error found on the next run.
- Tier-1 fixes are applied workspace-wide by `fix --apply`; there is no way
  to apply the fix for the one finding in front of you.
- `strictcode.toml` errors (unknown rule ID, retired rule, unknown group,
  wrong suppression shape) surface as exit code 2 from the next run rather
  than where they are typed.

## Proposal

A VS Code extension, the name to be chosen, that runs strictcode and presents
every kind of finding it produces.

### Features

- **Diagnostics for every rule.** Each finding at its file and line, with the
  rule ID as the diagnostic code (linked to that rule's documentation page),
  the severity mapped from the finding's severity, and the message. Findings
  with no meaningful line (a whole module, a manifest dependency) go on the
  line the finding names.
- **Related information for multi-site findings.** An import cycle carries
  every member module as `relatedInformation`, so the editor shows the whole
  cycle from any member.
- **Code actions.**
  - For a finding with a tier-1 fix: apply that fix, through strictcode's own
    fix path, which verifies it by graph re-extraction and rolls back on a
    mismatch.
  - For tier-2 fixes (behavior-changing, with consent per fix or per rule):
    once strictcode's consent flow exists, an explicit per-fix action is the
    editor's form of that consent; until then the extension shows no tier-2
    action. Tier-3 findings show their suggested manual fix in the hover.
  - For every finding whose rule accepts suppressions: add a suppression to
    `strictcode.toml` in the shape the rule declares, prompting for the
    reason and refusing an empty one, since the reason is mandatory.
- **`strictcode.toml` diagnostics.** The load errors the configuration can
  produce, positioned at the offending entry, including a retired rule's
  retirement record and its replacements. Completion of rule IDs and group
  names from the registry, and hover on a rule ID showing its registry facts
  and per-language support.
- **Tree view.** Findings grouped by rule and by workspace member, with
  counts per severity, and an import-cycles view listing each cycle's
  members.
- **Status bar.** Whether the last run passed (exit 0) or failed (exit 1),
  and a tool or configuration error (exit 2) shown as an error, never as
  "no findings".

### Running strictcode

strictcode's documented non-goals include "no watch mode, daemon, or
server": it is a batch command-line tool. That shapes the design:

- **Batch runs from the extension (fits the non-goal).** The extension runs
  `strictcode analyze <workspace> --json` on save (debounced) and on demand,
  reads the findings document, and maps it to diagnostics. Fixes run
  `strictcode fix` with a selector for the one finding, and suppressions are
  edits to `strictcode.toml`.
- **A language server (requires revisiting the non-goal).** A long-running
  Go server keeping the graph warm and re-extracting only the changed files,
  speaking the Language Server Protocol. This is faster on large workspaces
  but is a server, which the non-goals rule out; adopting it means changing
  that decision in `stricttools/docs/non-goals.md` and
  `stricttools/docs/decisions.md` first.

## Solutions considered

### Extension-only, driving the batch CLI (recommended first)

- Pros: respects the batch-tool non-goal; the findings document is already a
  schema-validated machine interface, so the extension adds no second
  authority; small.
- Cons: every run re-extracts the whole workspace, so on a large workspace
  diagnostics lag behind edits; unsaved buffers are not analyzed, because
  strictcode reads files from disk.

### A Go language server reusing `internal/engine`, `internal/extract`, and `internal/rules`

- Pros: incremental re-extraction per changed file; unsaved buffers can be
  analyzed from editor overlays; usable from any LSP client.
- Cons: contradicts a recorded non-goal; the graph's determinism and canonical
  hash are defined over a whole-workspace extraction, and incremental updates
  must be proven to produce the same graph as a full run (a test comparing the
  two over the fixtures would be the precondition).

### SARIF output plus a generic SARIF viewer

- Pros: SARIF output is already listed as deferred; a SARIF file is readable
  by existing viewers and CI tooling.
- Cons: a viewer shows results but offers no fix or suppression actions and
  no configuration support.

## What strictcode would need to change

- **Columns and ranges.** A finding's target carries a file and a 1-based
  line, derived at output time from a byte span. Editors underline ranges;
  adding the span (as line and column of start and end, in the findings
  document) lets a diagnostic cover the construct rather than a whole line.
  This is a findings-document change and a versioning decision
  (`stricttools/docs/versioning.md`).
- **Fixing one finding.** `fix` applies all tier-1 fixes; applying one needs
  a selector (a finding's rule and target ID), which is a new flag and
  subject to the CLI's conventions.
- **Configuration positions.** Load errors need the position of the
  offending entry in `strictcode.toml`. The config is loaded through
  strictspec (`internal/config/config.go`), so whether a position can be
  reported depends on strictspec exposing source positions for the document
  it validates; if it does not, that is a strictspec change first.

## Affected files and new components

- New: the TypeScript extension (its own directory with `package.json`,
  diagnostics mapping, code actions, tree views, and settings for the path
  to the `strictcode` binary).
- Changed, per the section above: `internal/findings/findings.go` and
  `schema/strictspec/findings.schema.toml` for ranges; `cmd/strictcode` and
  `internal/fix/fix.go` for a single-finding selector; `internal/config` for
  load-error positions.
- Documentation: `stricttools/docs/cli-and-output.md` for any new flag or
  field, and a page for the extension.
- If the server route is chosen: `stricttools/docs/non-goals.md` and
  `stricttools/docs/decisions.md`, plus a new server package.

## Distribution

- Publish the extension to both the VS Code Marketplace and Open VSX, the open
  registry VSCodium, Cursor, and other VS Code derivatives install from. Each
  needs its own publisher account and token; `vsce` publishes to the first and
  `ovsx` to the second, from the same `.vsix`.
- The extension is TypeScript/JavaScript; strictcode stays a Go binary.
  strictcode releases publish no prebuilt binaries and install with
  `go install`, so the extension finds `strictcode` on PATH or through a
  setting, and refuses with an error naming the setting when it is missing.
  Bundling binaries in per-platform `.vsix` packages would mean building
  release binaries, which reverses the no-prebuilt-binaries choice.
- strictcode releases with rlsbl, which has no VS Code publishing target; the
  extension's publish step would be added to rlsbl or run from a workflow of
  its own.

## Effort

- Extension driving the batch CLI, with diagnostics, related information,
  the tree view, and the status bar: three to four days.
- Suppression code actions and `strictcode.toml` completion and hovers: two
  to three days, plus the loader positions.
- Ranges in the findings document and the single-finding fix selector: two to
  three days including tests and documentation.
- A language server, if the non-goal is revisited: one to two weeks, most of
  it in proving incremental extraction equal to a full run.
- Packaging and dual-registry publishing: a day or two.
