# Migrate the CLI to the strictcli effects regime

Filed 2026-08-09 from an external dependency survey. Read-only analysis of `go.mod` only; this
project's command surface was NOT investigated, so the inventory step below is real work, not
a formality.

## Context

`go.mod` requires `github.com/smm-h/strictcli/go v0.27.0`. The current release is **v0.30.0**,
and the intervening versions shipped a breaking "effects regime":

- **v0.28.0** — per-command `effect` classification (`read_only` / `mutating`) becomes
  mandatory at registration, side effects flow through a closed `ctx.Effects()` method set
  (Run, Spawn, Write, Mkdir, Remove, Rename, Chmod, HTTP), and `--dry-run` becomes
  framework-owned: in dry mode effect calls RECORD instead of performing, and the framework
  renders a would-do log. Handlers never branch on a dry-run flag.
- **v0.29.0** — `WithConsequential()`: a separate declaration that is the only thing which
  triggers a confirmation prompt. Classification answers "record or perform?"; consequential
  answers "worth interrupting a human?". A non-interactive stdin without
  `--approve-consequential` is refused rather than hanging.
- **v0.30.0** — `WithDryRunUnsupported(reason)` so a command that genuinely cannot preview
  refuses `--dry-run` honestly instead of rendering a preview that would lie.

Also breaking: the reserved flag quartet (`--dry-run`, `--approve-consequential`, `--quiet`,
`--verbose`) is framework-owned at every level and declaring one is a registration-time panic;
`--yes` is banned outright with an error pointing at `--approve-consequential`.

## Problem

Staying on v0.27.0 means this CLI has no framework preview, no consent gate, and no
`effects-bypass` lint. Under the ecosystem's always-latest rule for internal dependencies the
pin should not persist, and the upgrade is a hard break: `WithEffect` omission panics at
registration, so the bump cannot land without classifying every command in the same pass.

## Work

1. **Inventory first.** For every registered command, determine whether it writes to disk,
   runs subprocesses, or performs network mutation, and classify it `read_only` or `mutating`.
   Do not guess: a `read_only` command that calls a mutating effect is a call-time hard error,
   and a non-allowlisted subprocess from a `read_only` command is likewise refused.
2. Bump to go-strictcli v0.30.0 (floor, never a pin — never an upper bound).
3. Add `WithEffect(...)` at every registration site.
4. Delete any locally-declared flag whose name collides with the reserved quartet, and any
   `--yes`; replace their reads with the framework accessors (`ctx.DryRun()`,
   `ctx.ApproveConsequential()`, `ctx.Quiet()`, `ctx.Verbose()`).
5. Route every write / subprocess / network mutation in handler code through `ctx.Effects()`,
   so dry-run honoring is structural rather than a branch someone must remember.
6. Add `WithConsequential()` to the genuinely dangerous commands only — keep that set small;
   it exists to be worth interrupting a human for, not as a synonym for "mutating".
7. Where a command cannot honestly preview (its effects escape the seam, or a later step reads
   state an earlier recorded step would have written), declare
   `WithDryRunUnsupported(reason)` with a real reason rather than shipping a lying preview.
8. Enable the check system so the receiver-aware `effects-bypass` lint runs, and fix whatever
   it flags (direct ambient calls that bypass the seam). Note: enabling checks auto-registers
   a framework command named `check` — if this project already owns a command by that name,
   that collision has to be resolved first.

## Affected files

- `go.mod`, `go.sum`
- every command registration site
- every handler that writes files, runs subprocesses, or performs network mutation

## Effort estimate

Depends entirely on command count and how far writes sit from the CLI layer. For a small
surface with writes near the handlers this is a single-pass change measured in hours; a large
surface with writes buried in packages is longer, because each write must be reachable from
the effects handle. Do it as ONE pass (bump + classify + route + lint), never as sequential
partial sweeps.
