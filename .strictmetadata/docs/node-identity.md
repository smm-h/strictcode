+++
title = "Node identity"
description = "How strictcode names nodes: qualified IDs from logical module names, with rules for anonymous units, overloads, receivers, escaping, and collisions."
nav_order = 110
+++

# Node identity

A node's public identity is a **hierarchical qualified name**. Location is an attribute, never
identity. Identity is reproducible from source alone, which statelessness requires, and does not
change when a node's body is edited.

IDs appear in the JSON findings output, in configuration suppressions, and in tier-1 fix
verification. [Decisions](../decisions/) records why content hashes and locations were rejected
as identity.

## Structure

Internally an ID is a sequence of segments. Its serialized form is:

```
<lang>:<member>:<module>:<container-chain>
```

- `<lang>`: `py`, `go`, or `ts` (TypeScript and JavaScript share `ts`).
- `<member>`: the workspace member's name, or `_` when scanning a single project that is not a
  workspace.
- `<module>`: the module's logical name (below).
- `<container-chain>`: names joined with `.` from module scope inward, such as
  `UserService.save` or `Outer.Inner.method`. It is empty for module nodes.

## Escaping

Inside a segment, `%`, `:`, `.`, `#`, `~`, and whitespace are percent-encoded. The `~` is
escaped so that a crafted name, such as a TypeScript computed method literally called
`cb~0~ab12cd34`, can never collide with the anonymous-unit form below.

Treat serialized IDs as opaque strings. Splitting them on separators is unsupported.

## Module segment: logical identity, never a file path

- **Python**: the dotted module path from the discovered package root (`pkg.sub.mod`). Package
  roots are discovered per workspace member at the member root and under `src/`, one namespace
  level deep. A member whose root is itself a package directory is rooted at that directory's
  importable name. Files outside any package root, such as scripts or a top-level
  `conftest.py`, get their full member-relative dotted path (`scripts/build.py` becomes
  `scripts.build`).
- **Go**: the package's import path relative to the member's module path
  (`internal/parser`). The root package is `.`.
- **TypeScript/JavaScript**: there is no logical module identity, so the ID is the
  member-relative file path with the extension removed and `index` collapsed to its directory
  (`src/util/strings`, and `src/util` for `src/util/index.ts`). A root-level index file is `.`.

A file move that preserves logical identity, such as a Python module moved with its package,
does not change IDs.

## Anonymous units

Closures, lambdas, and anonymous functions get a synthesized final segment:

```
<name-hint|anon>~<ordinal>~<fp8>
```

- `name-hint`: the assigned variable, property, or keyword-argument name when one can be derived
  from syntax; otherwise the literal `anon`.
- `ordinal`: the 0-based source-order index among anonymous siblings with the same hint in the
  same parent.
- `fp8`: the first 8 hex characters of SHA-256 over the unit's normalized signature text (its
  parameter list, LF-normalized and whitespace-collapsed).

The fingerprint makes ordinal drift detectable. If an edit renumbers anonymous siblings, a
reference whose ordinal now points at a unit with a different fingerprint is reported as stale,
never silently attached to the wrong unit.

## Overloads and redefinitions

Same-name siblings in one container, such as TypeScript overload signatures, a Python `def`
redefined under a condition, or getter and setter pairs, are told apart by `#<n>`, their 0-based
source order, with `#0` omitted. One counter covers every kind in the container, so a conditional
`def f` next to `class f` cannot collide. Parameter types are deliberately not part of the ID, so
editing a signature does not change identity; reordering same-name siblings does.

## Go receivers

Methods use the receiver type's name as their container (`Parser.Parse`). Whether the receiver
is a pointer is normalized away. Go forbids the same method name on both `T` and `*T`, so this
cannot collide, and changing a receiver between value and pointer does not change identity.

## Entry points

An entry point's ID has module segment `_` and a single chain segment `<form>/<declared-name>`,
such as `script/strictcode`.

## Collisions

Two distinct nodes producing the same serialized ID is a hard error at graph-build time. There
is no silent suffixing.

Module-level IDs that differ only by case are also a hard error, because on a case-insensitive
filesystem they name the same path. The case check applies only to module-level IDs: a class
`Outcome` and a function `outcome` in the same module are legal and distinct.
