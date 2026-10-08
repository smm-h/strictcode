+++
title = "Installation"
description = "How to install strictcode from source: it needs only the Go toolchain, because its tree-sitter runtime and grammars are pure Go."
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

Nothing else: no C compiler, and CGo may be on or off (`CGO_ENABLED=0` works). strictcode parses
with tree-sitter's official runtime and grammars (Python, Go, TypeScript, and TSX) as translated
to pure Go by cgofree. [Implementation](../implementation/) has the details.

## Distribution plans

[Distribution](../distribution/) records why releases are source-only and when prebuilt binaries
become necessary.
