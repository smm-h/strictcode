+++
title = "library-entry-point"
description = "library-entry-point reports a library declaring a command-line entry point."
nav_group = "Rule reference"
nav_order = 130
+++

# `library-entry-point`

:-: rule-facts id="library-entry-point"

## Semantics

A library that declares an entry point (Python `[project.scripts]` or `[project.gui-scripts]`, Go
`func main()` in `package main`, or npm `bin`) is really an application. Runs only on members
marked `library = true`.

## Lineage

The donor's `library-lint` aggregate's entry-point check, now its own rule in `group:library`.

## Configuring

This rule takes no suppressions and has no exemption list.
