+++
title = "README.md"
+++
# strictcode

*A linter for architecture, with tiered auto-fixes.*

strictcode is a deterministic, non-LLM command-line tool. It scans a codebase with tree-sitter,
builds a graph of its modules, callables, and types, and enforces dependency-hygiene, dead-code,
import-cycle, and library-boundary rules across Python, Go, and TypeScript/JavaScript. Every
finding offers a fix in one of three tiers: guaranteed behavior-preserving (applied
automatically and verified by re-extracting the graph), behavior-changing with consent, or
suggestion only.

## Install

```bash
go install github.com/smm-h/strictcode/cmd/strictcode@v0
```

Building needs Go, a C compiler (gcc or clang), and CGo enabled (`CGO_ENABLED=1`), because
strictcode links the official tree-sitter runtime and grammars, which are C. Without a C compiler
the build fails with `build constraints exclude all Go files in .../tree-sitter-python@.../bindings/go`:
install a C compiler; setting `CGO_ENABLED=0` never works around it. Releases publish no prebuilt
binaries.

## Use

```bash
strictcode analyze .           # report findings; exit 1 if any is an error
strictcode analyze . --json    # the findings document as JSON
strictcode fix . --preview     # list the tier-1 fixes it would apply
strictcode fix . --apply       # apply them, verified against the re-extracted graph
```

:-: table-commands

Configuration lives in `strictcode.toml` at the analyzed directory: rule toggles, severities,
analysis modes, and suppressions, each with a mandatory reason.

## Documentation

The documentation site covers the graph model, every rule, configuration, the support matrix by
language, and the decisions behind the design, including the alternatives that were rejected. Its
source is in `.stricttools/docs/`.

## License

Apache 2.0.
