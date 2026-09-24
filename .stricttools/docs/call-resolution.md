+++
title = "Call resolution"
description = "How strictcode resolves Python calls: an always-on syntactic, import-aware layer that records unresolvable calls honestly, and a planned opt-in type-checker layer that is a hard error if unavailable."
nav_order = 160
+++

# Call resolution

Python is the first language with full-semantic depth. Its dynamic typing makes call resolution
the hardest problem in the space, so solving it first makes every later language easier.

## Two explicit layers

1. **Syntactic and import-aware** (always on). Calls are resolved through imports, module
   attributes, and class methods, using strictcode's own symbol tables: one per module plus a
   workspace-wide module index. Dynamic dispatch, such as duck typing, monkey-patching, or
   `getattr`, is recorded as unresolved, never guessed.
2. **Type-checker-backed** (explicit opt-in mode, planned). Consumes an external type checker's
   inferences to resolve method calls on inferred types. It is selected in
   [configuration](../config/) with `python_call_resolution = "type-checker"` and a
   `python_type_checker` of `pyright` or `ty`. If configured, it must work: a missing or failing
   type checker is a hard error, never a quiet downgrade to the syntactic layer. The mode is not
   built yet; the [support matrix](../support-matrix/) shows `call-resolution-type-informed` as
   planned.

## How the syntactic layer behaves

Resolution is conservative by construction:

- A local parameter or assignment shadowing an outer name makes the call unresolved.
- Instance method calls such as `obj.m()` are unresolved.
- `self` and `cls` dispatch follows base-class chains only as far as they resolve locally, and
  stops honestly otherwise.
- Star imports leave unknown names unresolved.
- Aliases are canonicalized: `import sys as s; s.stdout.write(...)` resolves to
  `sys.stdout.write`, which is how the library rules stay alias-proof.

Each call site is classified as **syntactic** (resolved to a local callable, and mirrored by a
`calls` row), **external** (the standard library, a builtin, or an external package), or
**unresolved**. External and unresolved sites live in a side table, not in the relation; see
[the graph model](../graph-model/). Module-level and class-level calls produce no `calls` rows,
because `calls` rows start at functions or closures, but they are still recorded as sites, so a
module-level `print` in a library is still reported.

Resolving a call to a local type emits an `instantiates` row. Resolvable local decorators emit
`decorates` rows.
