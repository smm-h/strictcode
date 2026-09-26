+++
title = "Installation"
description = "How to install strictcode from source: it needs Go, a C compiler, and CGo enabled, because it links the official tree-sitter runtime and grammars."
nav_order = 20
+++

# Installation

strictcode is distributed as a Go module, and releases publish no prebuilt binaries. Install it
with the Go toolchain:

```bash
go install github.com/smm-h/strictcode/cmd/strictcode@v0
```

Use `@v0` or an exact version such as `@v0.2.0`, never `@latest`: no strictcode release is 1.x
or above, so `@latest` has nothing real to prefer.

## Requirements

- **Go**, at the version `go.mod` declares.
- **A C compiler** (gcc or clang) on the `PATH`.
- **CGo enabled** (`CGO_ENABLED=1`). Go enables it by default when it finds a C compiler, and
  disables it when it does not.

The C compiler is needed because strictcode links the official tree-sitter runtime and its
grammars (Python, Go, TypeScript, and TSX), which are C source compiled at build time.
[Implementation](../implementation/) explains why strictcode uses the C runtime rather than a
pure-Go one.

## When the build fails with "build constraints exclude all Go files"

Without a C compiler, or with `CGO_ENABLED=0`, the build fails like this:

```
github.com/tree-sitter/tree-sitter-python/bindings/go: build constraints exclude all Go files in .../tree-sitter-python@v0.25.0/bindings/go
```

The message names a tree-sitter package, but the cause is the missing C toolchain: those
packages consist only of CGo files, so with CGo off there is nothing left to build. Install a C
compiler and make sure `CGO_ENABLED` is not set to `0`. Setting `CGO_ENABLED=0` never works
around it.

## Distribution plans

[Distribution](../distribution/) records why releases are source-only and when prebuilt binaries
become necessary.
