# Reversibility declaration needed: `fix` has no undo command

## Context

strictcli is gaining declared reversibility support: a mutating command will
declare which command undoes it (verified at registration in both
directions), a command with no recovery will declare irreversible with a
mandatory reason, a warn-severity check will flag destructive commands
declaring neither, and after a real run the framework will print a paste-able
recovery command and emit a machine-readable recovery member in the JSON
result document.

## Problem

`fix` applies code transforms to the working tree with no undo command. The
practical inverse is version control, which the tool neither owns nor
records.

## Solution

Most likely an honest irreversible-with-reason declaration ("the working
tree is the caller's; recovery is version control") rather than a new
command — but that should be a declared choice when the framework support
ships, not an implicit one.

## Effort

Trivial (a declaration), unless a pre-fix snapshot mechanism is wanted, which
would be medium and probably disproportionate.
