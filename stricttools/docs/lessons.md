+++
title = "Lessons register"
description = "The regression requirements every strictcode rule must meet: each encodes a real false positive or false negative, and each is a red-green test in the suite."
nav_order = 220
+++

# Lessons register

Each item encodes a real false positive or false negative, found either in the donor
implementation inside rlsbl or in strictcode's own runs on real code. These are acceptance
criteria: none may regress. Each one is a red-green test in the suite: the test was written
first, seen failing, then made to pass.

The numbered items come from the donor, with items specific to dropped languages omitted. Their
numbers are stable, and tests and code comments cite them as "lesson N". The tests live in
`internal/checks/lessons_test.go`, with the test-context and resolution lessons also tested in
`internal/testctx` and `internal/extract`.

## From the donor

1. A workspace dependency imported inside `try/except ImportError` or `ModuleNotFoundError` MUST count as used for `deps-unused`.
2. A hard dependency (scope `runtime` or `explicit`) imported only under guards MUST still be flagged, with a message saying to declare it optional or import it unconditionally. Guards satisfy only `dev` and `peer` dependencies.
3. An import in an `except` body (a fallback import) MUST NOT be treated as guarded.
4. `deps-undeclared` MUST NOT flag guarded optional imports.
5. Imports under `if TYPE_CHECKING:`, bare and `typing.`-qualified, MUST be excluded from both `deps-undeclared` and `deps-unused`.
6. Test context MUST be classified by path relative to the member root, not by substring: a production `src/test/` MUST NOT be test code.
7. `testdata/` and `__tests__/` MUST be test context at any depth.
8. `integration_test/` and root-level `test`, `tests`, `example`, and `examples` MUST be test context as first path components.
9. Go packages under `testdata/` at any depth, or made only of `*_test.go` files, MUST NOT be reported dead.
10. Imports MUST resolve to the workspace member name even when the registry name differs, so the mismatch never produces a false `deps-undeclared`.
11. Namespace-package imports (`from ns.member import X`) MUST resolve to the member through the discovered namespace map.
12. A member's explicit `import_name` override MUST be honored.
13. Sibling workspace directories MUST be pruned from a member's scan, especially with `path = "."`, and a sibling's source MUST NOT trigger `deps-undeclared`.
14. For union-of-imports dead-module detection (Python and Go), a suppressed unit MUST be removed from the reference union too: its imports and exports MUST NOT keep other units alive.
15. Python files under a root `scripts/` directory MUST be excluded from dead-module candidates, and their imports MUST NOT keep other modules alive.
16. A Python module exported by any `__init__.py` (through `__all__` or a relative import) MUST NOT be flagged dead.
17. JavaScript and TypeScript files inside a Python package tree MUST NOT be treated as npm modules.
18. TypeScript/JavaScript relative-import resolution MUST probe extensions, map `.js` to `.ts` and `.jsx` to `.tsx`, and resolve a directory to `index.*`; otherwise reachability under-counts and dead code over-reports.
19. TypeScript/JavaScript entry points MUST be collected from `exports` (recursively), `main`, and `bin`.
20. Cycle detection MUST NOT run on Go, where the compiler rejects cycles; the matrix cell is n/a.
21. Cycle detection MUST report only strongly connected components of two or more modules (self-loops ignored).
22. The library-boundary rules MUST NOT run on members not marked `library = true`.
23. The library-boundary rules MUST apply the default per-language test and example excludes.
24. `unreachable-code` MUST NOT treat comment nodes as statements: no false positive on a trailing comment, and no false negative from a comment masking a real unreachable statement after it.
25. A terminator inside a nested function or class MUST NOT mark the enclosing block's following code unreachable.
26. Both the workspace-level allow list and the per-language `allow` list MUST be subtracted from the forbidden-imports set.
27. Direct `logging` use in a Python library MUST be a warning; `print` and `sys.stdout` writes MUST be errors.
28. `dead-workspace-packages` MUST skip dev-only, non-library, and published releasable members; MUST give test-only importers a different message from zero importers; and self-imports never count.
29. Asset, build, and vendor directories (the source-walk exclusions on [check semantics](../check-semantics/)) MUST be excluded from all source walks.
30. All rules in a run MUST share one graph build, with no per-rule re-scanning.
31. Malformed configuration MUST fail loudly, never be coerced or skipped.
32. A suppression path that does not exist on disk MUST be a hard error.

## From the rlsbl port

These came with the checks that moved from rlsbl into strictcode: rlsbl's test cases and the false
positives fixed there, each a red-green test in `internal/checks`.

33. `deps-stale` MUST report a registry-sourced constraint that the dependency member's declared version does not satisfy, and MUST NOT judge path sources, workspace-protocol sources, constraints in forms it does not evaluate, or a dependency without a static version.
34. A Python tool rule (`lint`, `format`, `type-check`) MUST run nothing while its option is off, and MUST be refused, naming the fix, while on with no `[python_tools.<rule>]` declaration or for a member whose directory no declared path lies in.
35. A Python tool rule MUST run its tool through `uv run` in the declared directory over the declared paths, report each problem the tool names at its file and line and at the owning member's option value, refuse ruff older than the version whose JSON `lint` reads, and treat output it cannot read as an error, never a pass.
36. The `uv run` flags MUST reach the tool where the project declares it: `--group` for a dependency group other than `dev`, `--extra` for an optional-dependencies extra, none inside a uv workspace.
37. A member nested inside a declared path MUST be left out of that path's ruff run, its files belonging to its own declared path.
38. The scope guards MUST report a tool's own configuration that narrows or overrides the declared paths (ruff `include` and `extend-include`; mypy `files`, `packages`, and `modules`), MUST leave exclusion keys alone, and MUST report nothing while their tool rule is off.
39. `strictspec-certificate` MUST block on a violated claim, an unsupported claim no adjudication entry discharges, and a dangling adjudication entry; MUST pass `corpus-supported` and `proven` claims; and MUST treat a missing or unreadable certificate or adjudication file as an error, never a pass.
40. A directory named like a build artifact (`build`, `dist`, `static`, and the others on [check semantics](../check-semantics/)) MUST be left out of a source walk only directly under a member's root; deeper it is an ordinary package directory, such as a Go project's `internal/build`.
41. A source walk MUST read only what git lists (tracked files and untracked files that are not ignored), so a gitignored file is never read; a workspace root outside a git work tree MUST be refused.
42. A Go internal package imported only by other packages' `_test.go` files MUST be alive, and a main package MUST never be a candidate; a package's own tests and files under `testdata/` MUST NOT keep it alive.
43. A member nested in another member's directory MUST be left out of the enclosing member's walk.
44. A TypeScript entry point inside `tsconfig.json`'s `outDir` MUST resolve to its source under `rootDir`, files under `outDir` MUST NOT be scanned as source, and with no `rootDir` known reachability MUST abstain rather than report the sources dead.
45. A src-layout Python package's modules MUST be named by their import path, not by their file path, so a module imported only from inside its package is alive.
46. A member whose import name differs from its distribution name (a `core` distribution shipping `portal_core`, a `cloudflare` shipping `src/cf`) MUST be matched by the package directory it ships, so its dependents' imports are neither unused nor undeclared and the library is not dead.

## From real-code runs

These came from running strictcode on real repositories, each after a finding that was wrong:

- **`__main__.py` is an implicit entry point.** rlsbl's `python -m rlsbl` runner was reported dead.
- **TypeScript reachability abstains without a resolved entry point.** Exports pointing at built
  `dist/` output would otherwise make every source file dead.
- **Nested Go modules count.** A member with its own nested `go.mod` (a conformance harness, for
  example) produced a false error until nested requirements and module paths were honored.
- **The case-only ID clash applies to module-level IDs only.** strictcli's legal `class Outcome`
  beside `def outcome` stopped the build.
- **A member whose root is itself a package directory** (its root contains `__init__.py`) needs
  its package rooted at the directory's importable name. Without that, relative imports never
  resolved in selfdoc and dozens of modules were falsely reported dead.
