+++
title = "Distribution"
description = "Releases are source-only Go modules; binaries become necessary at the rlsbl handoff, built with zig cc under goreleaser or on native CI runners."
nav_order = 410
+++

# Distribution

Releases publish the Go module and nothing else: the GitHub Releases carry no binaries, and the
publish workflow only confirms that the module reached the Go proxy. Installing therefore needs a
Go toolchain and a C compiler; see [installation](../installation/).

## Why source-only is enough for now

Wherever Go and a C compiler already exist, the build costs a few seconds, not minutes; the
toolchain requirement, not the build time, is the cost. [Decisions](../decisions/) records the
measurement. By consumer:

| Consumer | Cost of building from source |
|---|---|
| A development machine with Go and gcc | None. |
| GitHub Actions Ubuntu runners | Low: gcc is preinstalled, and `setup-go` is one step. |
| macOS machines | The Xcode Command Line Tools, for the C compiler. |
| Windows machines | mingw, which is painful. |
| Slim containers | A C toolchain added to the image. |

## When binaries become necessary

The [rlsbl handoff](../rlsbl-integration/) is the point where source-only stops being acceptable.
rlsbl is a Python tool that manages projects in every ecosystem. If it runs strictcode as a
required check and strictcode is source-only, every project rlsbl manages, including pure Python
and pure npm projects, needs a Go toolchain and a C compiler on every developer machine and in
CI. PyPI and npm wrapper packages, which download a prebuilt binary from the GitHub Release, are
impossible without binaries.

## How binaries would be built

Cross-compiling a CGo program needs a C compiler for each target, and the system compiler only
builds for the machine it runs on. There are two ways to solve that:

1. **`zig cc` under goreleaser.** Zig's compiler bundles clang plus the C libraries and headers
   for many targets (glibc and musl for Linux, mingw for Windows, and macOS system headers and
   library stubs). Setting `CC="zig cc -target <triple>"` for each goreleaser build lets one Linux
   CI runner build every platform's binary, and musl makes the Linux binaries fully static. The
   risks: macOS linking is the fragile target, because Go passes linker flags zig has not always
   accepted and zig can only stub Apple's system libraries; zig is pre-1.0, so its version would be
   pinned in CI; and unsigned macOS binaries trigger the operating system's unverified-developer
   warning when opened by hand, though not when a wrapper fetches and runs them.
2. **Native builds on per-platform CI runners.** Linux, macOS, and Windows runners each build
   their own binary, with no cross-compiling at all. This avoids the macOS linking risk, at the
   cost of more runners.

Either one, chosen explicitly in the release configuration, unblocks the handoff and the wrapper
packages. The names `strictcode` on PyPI and npm are already held by placeholder packages.
