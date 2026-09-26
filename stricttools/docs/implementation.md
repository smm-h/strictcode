+++
title = "Implementation"
description = "How strictcode is built: Go, the official tree-sitter C runtime through CGo, strictspec readers, strictcli, the runtime model, and packages."
nav_order = 400
+++

# Implementation

## Language: Go

strictcode is written in Go, chosen over Rust and Zig. The deciding lens was that the codebase is
written and read mostly by AI agents, which inverts the usual valuation: verbosity helps and
expressiveness hurts. Go's deliberate minimalism (one way to do things, no macros, no operator
overloading, no implicit conversions, and limited generics) gives an agent less to invent and less
to get wrong. Go also brings fast compilation, a mature ecosystem, backward compatibility since
Go 1, built-in concurrency for parallel parsing, a garbage collector that handles cyclic graph
structures with no bookkeeping, and plenty of precedent among static analysis tools.

Go's lack of sum types is the accepted cost: graph nodes use a kind field with type switches,
behind a small, well-tested accessor layer. The node vocabulary is small and changes rarely, so
the missing exhaustiveness checking is a bounded risk.

[Decisions](../decisions/) records the comparison with Rust and Zig in full.

## Parsing: tree-sitter through the official C runtime

tree-sitter is the parsing foundation: MIT-licensed, with grammars for hundreds of languages,
error-tolerant, and very actively maintained. There is one parsing path, the graph extractor over
tree-sitter trees, and no regular-expression fallback anywhere.

strictcode uses the **official CGo bindings**, `github.com/tree-sitter/go-tree-sitter`, with the
official grammars for Python, Go, TypeScript, and TSX, at the versions `go.mod` pins. The
`internal/treesitter` package is the single integration layer. It:

- selects the grammar for each file (the TypeScript/JavaScript profile spans both the
  `typescript` and `tsx` grammars, because JSX conflicts with TypeScript type assertions);
- normalizes line endings to LF before parsing, so every byte span indexes the same bytes;
- owns the C resource lifecycle, closing every parser, tree, query, and cursor;
- exposes error-tolerant parses honestly, so extractors decide what a parse error means.

The pure-Go rewrite of the runtime, gotreesitter, was measured twice against pinned criteria and
rejected both times: it produces different trees from the official grammars by design, and parses
Python several times slower. Its misparses were reported upstream. [Decisions](../decisions/) has
the measurements and the reasoning, including why strictcode does not write its own runtime and
what would reopen the question.

The costs of CGo are that building needs a C compiler (see [installation](../installation/)),
that cross-compiling needs a C cross-toolchain (see [distribution](../distribution/)), and that
the race detector cannot see across the C boundary.

## Build versus depend

strictcode builds anything simple in-house and depends on an external project only where
rewriting it would be prohibitive.

- **Depended on**: tree-sitter for parsing; strictspec for every schema artifact and its
  generated Go readers (see [versioning](../versioning/)); and strictcli for the command line,
  which enforces the ecosystem's flag conventions when commands are registered.
- **Built in-house**: graph construction (tree-sitter queries to nodes and rows, the core of the
  tool), graph storage (in memory, with no graph database), the rule engine, the fix engine, and
  output rendering.
- **Candidates for future formatting-preserving transforms**: LibCST (Python, MIT) and Comby
  (language-agnostic structural rewriting, Apache 2.0).

## Runtime model

strictcode is a stateless batch tool: parse everything, build the graph in memory, run the rules,
report, and exit. Nothing persists between runs. A per-file cache keyed by content hash could be
considered if rebuilding ever hurts on very large repositories; it is deliberately not part of
the design.

## Go full-semantic depth

Go has a better option than tree-sitter for its own full-semantic depth: the standard library's
`go/parser` and `go/types`. They are pure Go, authoritative for Go, and provide real type
information, which would make Go's call resolution and implicit interface satisfaction far
simpler than rebuilding them over syntax trees. This is a design option for the Go extractor, not
a replacement for tree-sitter, which Python and TypeScript still need.

## Package layout

:-: list-modules path="internal/"
