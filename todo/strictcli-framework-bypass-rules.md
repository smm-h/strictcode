# Host the strictcli framework-bypass rules in strictcode, and remove source parsing from strictcli

## Context

strictcli promises its callers, most of them AI agents, several things that a
program built on it can silently break by going around the framework:

- Under `--json`, stdout carries one JSON document, written by the framework's
  exit step on every way out of a command.
- Messages written through the Context writers reach that document's
  `diagnostics`.
- The framework parses argv, so `--help` and the help document describe every
  input a command takes.
- Environment input is declared (a flag's environment binding, a handshake, a
  connection, a location root), so it too appears in `--help`.
- Side effects go through the effects handle, so `--dry-run` previews them
  truthfully and consent applies.

A survey of strictcli consumers found these guarantees bypassed on a large
scale: process exits inside command code (for example a `die()` helper calling
`os.Exit`, which leaves `--json` callers with empty output on every error),
direct stdout writes beside the JSON document, direct stderr writes that never
reach `diagnostics`, argv rewritten before the framework parses it, and raw
environment reads.

strictcli has since gained the runtime pieces that make every bypass
unnecessary: `ExitNow` (an early exit that still writes the JSON document), a
goroutine helper, `ctx.Out` for main output, `ctx.Document()` for commands that
own stdout, declared payload renderers, framework-owned signal handling, and a
runtime stdout guard under `--json`. What is missing is static enforcement that
no consumer can dodge.

strictcli shipped two source scanners of its own: the `--lint-framework-use`
reserved flag (contract §28 of `.stricttools/docs/history/_effects-contract.md`)
and the older `effects-bypass` check (contract §11). The owner has ruled that
strictcli must not parse source code at all: every static rule moves to
strictcode, and both scanners are deleted from strictcli.

## Rulings already made

- Rule IDs take the form `strictcli:<rule>`: `strictcli:process-exit`,
  `strictcli:stdout-write`, `strictcli:stderr-write`, `strictcli:argv-access`,
  `strictcli:environment-read`, `strictcli:exit-now-in-goroutine`, and
  `strictcli:effects-bypass`. strictcode's existing general rules stay bare.
- The rules scan every source file of a program built on strictcli, test files
  and other programs in the repository excluded. `strictcli:effects-bypass`
  scans whole programs too, with no handler roots or call following.
- Every strictcode rule carries a suppression level from 1 to 10, with
  cumulative thresholds:
  - 1 to 3: a suppression with a reason is accepted.
  - 4 and up: a suppression prints a warning.
  - 6 and up: a suppression needs human approval, given through
    `--approve-consequential` (an agent never passes it on its own judgment).
  - 8 and up: the suppression is listed where the maintainer sees it. Not in the
    release changelog, whose readers are the tool's users.
- strictcode reads only git-tracked files and refuses a source file that does
  not parse, for every rule, not only these.
- rlsbl runs strictcode in release validation, and a finding blocks the release.

## Open decisions

1. Where level-8+ suppressions are listed. Candidates: a generated, committed
   report (for example `SUPPRESSIONS.md`) whose freshness rlsbl checks; the
   release run's output; the consequential approval prompt; or dropping the tier.
2. The suppression level of each `strictcli:*` rule (all at 10, or
   `strictcli:stderr-write` lower).
3. How strictcode is distributed while its tree-sitter dependency (cgofree's
   pure-Go translation) stays unpublished and resolves only through a local,
   gitignored `go.work`: prebuilt binaries built on this machine and attached to
   each release (the documented `go install` stops working until cgofree is
   published), copying the generated packages into strictcode, or keeping
   strictcode local.
4. Whether the rules track single-assignment aliases such as
   `const p = process; p.exit()` and `p = sys; p.exit()`, which strictcli's own
   §28 scanners miss.

## Work in strictcode

strictcode today resolves calls for Python only, records no attribute reads
(`sys.argv`, `process.env`), assignments (`process.exitCode = 1`),
`raise SystemExit`, keyword arguments (`print(file=...)`), or literal arguments
(`os.write(1, ...)`), has no notion of a program built on strictcli, walks the
filesystem instead of reading git-tracked files, and analyzes files with parse
errors silently.

- Extraction: references, assignments, raise statements, and keyword and literal
  arguments in all three languages; call sites and import resolution for Go and
  TypeScript (TypeScript including tsconfig `paths` aliases and package
  self-imports).
- A predicate for "a program built on strictcli", per language, following
  §28.2 of strictcli's contract: in Go, every `package main` of the module that
  imports strictcli plus the same-module packages it imports; in Python, the
  package behind console entry points that construct a strictcli app; in
  TypeScript, the modules reachable from `bin` entries.
- The seven rules, carrying over §28.3's construct lists, tie-break rules, and
  message forms, and §11's effects-bypass name lists, from strictcli's contract
  (they remain readable in strictcli's git history after deletion).
- Suppression levels and their thresholds in the rule registry and config
  loader; git-tracked input and parse-error refusal globally.
- Distribution per open decision 3; the rlsbl adapter.

## Work in strictcli

- Delete `--lint-framework-use` and the `effects-bypass` scanner in all three
  ports: the scanners, the reserved-flag entries, the error templates, the
  conformance cases and parity entries, the TypeScript token scanner, and the
  docs. Both were released, so this is a breaking change. The check provider
  that registers `effects-bypass` also registers checks that read no source;
  those stay.
- Supersede contract §28, §11, and the related §12.17 rows and §18 items rather
  than rewriting them.
- Two owner rulings that are unbuilt:
  - In-process invocation (Go `Call`, Python `call`, TypeScript equivalent)
    returns a result record in all three ports, carrying the payload, the exit
    code, and the bytes a command that owns stdout wrote through
    `ctx.Document()` (and an owns-stdout child's stdout).
  - Killing a child the handler left running kills its whole process group
    (Windows: a job object), so a grandchild holding the captured output pipe
    cannot keep the command from exiting.

## Order

strictcode's rules and its release come first, then the rlsbl adapter, then the
deletions and the two rulings in strictcli with one strictcli release. Each
strictcli consumer is migrated when the new rules block its next release.

## Effort

strictcode: several days (extraction for three languages is the bulk).
strictcli: about a day for the deletions and the two rulings across three ports.
