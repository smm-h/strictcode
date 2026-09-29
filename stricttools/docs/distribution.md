+++
title = "Distribution"
description = "Releases are source-only Go modules that build with Go alone; binaries become necessary at the rlsbl handoff, and cross-compile with GOOS and GOARCH."
nav_order = 410
+++

# Distribution

Releases publish the Go module and nothing else: the GitHub Releases carry no binaries, and the
publish workflow only confirms that the module reached the Go proxy. Installing therefore needs a
Go toolchain; see [installation](../installation/).

## Why source-only is enough for now

strictcode is pure Go (its tree-sitter runtime and grammars are the cgofree translations of the C
sources), so wherever Go exists, the build costs a few seconds, not minutes; the Go toolchain
requirement, not the build time, is the cost. [Decisions](../decisions/) records the measurement.
Until strictcode moved to cgofree, building also needed a C compiler, which was painful on
Windows and in slim containers; that requirement is gone.

## When binaries become necessary

The [rlsbl handoff](../rlsbl-integration/) is the point where source-only stops being acceptable.
rlsbl is a Python tool that manages projects in every ecosystem. If it runs strictcode as a
required check and strictcode is source-only, every project rlsbl manages, including pure Python
and pure npm projects, needs a Go toolchain on every developer machine and in CI. PyPI and npm
wrapper packages, which download a prebuilt binary from the GitHub Release, are impossible
without binaries.

## How binaries would be built

strictcode builds with `CGO_ENABLED=0`, so one machine cross-compiles every platform's binary by
setting `GOOS` and `GOARCH`, with goreleaser or plain `go build`, and the Linux binaries are
static. The earlier plans for a CGo build (`zig cc` per target, or native builds on per-platform
CI runners) are no longer needed. Unsigned macOS binaries still trigger the operating system's
unverified-developer warning when opened by hand, though not when a wrapper fetches and runs
them.

Building them in the release configuration unblocks the handoff and the wrapper packages. The
names `strictcode` on PyPI and npm are already held by placeholder packages.
