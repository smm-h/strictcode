package checks

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/fixture"
)

// The cases of rlsbl's library-lint tests (Python, Go, and npm libraries)
// that the numbered lessons do not already pin.

// libraryMember is a workspace whose one member, m, is a library made of
// the given files (paths relative to m), with an optional strictcode.toml.
func libraryMember(files map[string]string, strictcodeToml string) map[string]string {
	out := map[string]string{
		fixture.DeclarationsPath: fixture.Workspace("path = \"m\"\nname = \"m\"\nlibrary = true\n"),
	}
	for k, v := range files {
		out["m/"+k] = v
	}
	if strictcodeToml != "" {
		out["strictcode.toml"] = "format_version = 1\n" + strictcodeToml
	}
	return out
}

func goLibrary(files map[string]string, strictcodeToml string) map[string]string {
	all := map[string]string{"go.mod": "module example.com/m\n\ngo 1.22\n"}
	for k, v := range files {
		all[k] = v
	}
	return libraryMember(all, strictcodeToml)
}

func npmLibrary(files map[string]string, strictcodeToml string) map[string]string {
	all := map[string]string{"package.json": `{"name": "m", "main": "./index.js"}`}
	for k, v := range files {
		all[k] = v
	}
	return libraryMember(all, strictcodeToml)
}

func pyLibrary(files map[string]string, strictcodeToml string) map[string]string {
	all := map[string]string{"pyproject.toml": "[project]\nname = \"m\"\n", "pkg/__init__.py": ""}
	for k, v := range files {
		all[k] = v
	}
	return libraryMember(all, strictcodeToml)
}

// specifiers lists the module each finding of the rule names after
// "module '".
func forbiddenModules(fs []findings.Finding) []string {
	var out []string
	for _, f := range byRule(fs, "library-forbidden-imports") {
		if i := strings.Index(f.Message, "module '"); i >= 0 {
			rest := f.Message[i+len("module '"):]
			out = append(out, rest[:strings.Index(rest, "'")])
		}
	}
	return out
}

func TestPortedPythonForbiddenImports(t *testing.T) {
	fs := analyze(t, pyLibrary(map[string]string{
		"pkg/cli.py":  "import argparse\n",
		"pkg/web.py":  "from flask import Flask\n",
		"pkg/fine.py": "import os\nimport json\nfrom click_helpers import x\n",
	}, ""))
	got := strings.Join(forbiddenModules(fs), ",")
	if got != "argparse,flask" && got != "flask,argparse" {
		t.Fatalf("forbidden imports %q, want argparse and flask only", got)
	}
}

func TestPortedGoLibraryBoundary(t *testing.T) {
	fs := analyze(t, goLibrary(map[string]string{
		"single.go":  "package m\n\nimport \"net/http\"\n\nvar _ = http.StatusOK\n",
		"grouped.go": "package m\n\nimport (\n\t\"fmt\"\n\t\"github.com/spf13/cobra\"\n\t\"strings\"\n)\n\nvar _ = cobra.Command{}\nvar _ = strings.ToUpper\n\nfunc g() { fmt.Println(\"x\") }\n",
		"print.go":   "package m\n\nimport (\n\t\"fmt\"\n\t\"io\"\n\t\"os\"\n)\n\nfunc p(w io.Writer) {\n\tfmt.Printf(\"%d\", 1)\n\tfmt.Print(\"y\")\n\tos.Stdout.Write(nil)\n\tfmt.Fprintln(os.Stderr, \"z\")\n\tfmt.Fprintln(w, \"to the caller\")\n\t_ = fmt.Sprintf(\"not a write\")\n}\n",
		"m_test.go":  "package m\n\nimport (\n\t\"fmt\"\n\t\"net/http\"\n\t\"testing\"\n)\n\nfunc TestX(t *testing.T) { fmt.Println(http.StatusOK) }\n",
	}, ""))
	mods := strings.Join(forbiddenModules(fs), ",")
	if !strings.Contains(mods, "net/http") || !strings.Contains(mods, "github.com/spf13/cobra") || strings.Count(mods, "net/http") != 1 {
		t.Errorf("forbidden imports %q, want net/http once (not from the test file) and cobra", mods)
	}
	var callees []string
	for _, f := range byRule(fs, "library-stdout") {
		callees = append(callees, f.Message[strings.LastIndex(f.Message, " via ")+5:])
	}
	want := map[string]bool{"fmt.Println": true, "fmt.Printf": true, "fmt.Print": true, "os.Stdout.Write": true, "fmt.Fprintln(os.Stderr)": true}
	if len(callees) != len(want) {
		t.Fatalf("stdout callees %v, want %v", callees, want)
	}
	for _, c := range callees {
		if !want[c] {
			t.Errorf("unexpected stdout finding via %s", c)
		}
	}
	if got := byRule(fs, "library-entry-point"); len(got) != 0 {
		t.Errorf("a library without package main reported an entry point: %+v", got)
	}
}

func TestPortedGoLibraryEntryPointAndConfiguration(t *testing.T) {
	fs := analyze(t, goLibrary(map[string]string{
		"lib.go":          "package m\n\nimport \"net/http\"\n\nvar _ = http.StatusOK\n",
		"cmd/m/main.go":   "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n",
		"internal/x/x.go": "package x\n\nimport \"github.com/x/forbidden\"\n\nvar _ = forbidden.X\n",
	}, "[rules.library-forbidden-imports.forbidden]\ngo = [\"github.com/x/forbidden\"]\n\n[rules.library-stdout.allow]\ngo = [\"fmt.Println\"]\n"))
	if got := byRule(fs, "library-entry-point"); len(got) != 1 {
		t.Errorf("a library's func main must be one entry-point finding: %+v", got)
	}
	if got := forbiddenModules(fs); len(got) != 1 || got[0] != "github.com/x/forbidden" {
		t.Errorf("a replaced forbidden list must judge only its own entries: %v", got)
	}
	if got := byRule(fs, "library-stdout"); len(got) != 0 {
		t.Errorf("an allowed callee was reported: %+v", got)
	}

	fs = analyze(t, goLibrary(map[string]string{"lib.go": "package m\n\nimport \"net/http\"\n\nvar _ = http.StatusOK\n"},
		"[rules.library-forbidden-imports.forbidden]\ngo = []\n"))
	if got := forbiddenModules(fs); len(got) != 0 {
		t.Errorf("an empty forbidden list must forbid nothing: %v", got)
	}
}

func TestPortedNpmLibraryBoundary(t *testing.T) {
	fs := analyze(t, npmLibrary(map[string]string{
		"index.js":       "import express from 'express';\nimport { Command } from 'commander';\nexport const x = 1;\nconsole.log('a');\n",
		"req.cjs":        "const koa = require('koa');\nconsole.warn('b');\n",
		"dyn.mjs":        "const h = await import('hono');\nconsole.error('c');\n",
		"reexport.ts":    "export { y } from 'yargs';\nconsole.info('d');\n",
		"view.tsx":       "import express from 'express';\nexport const V = () => <div />;\nprocess.stdout.write('e');\n",
		"fine.ts":        "import fs from 'node:fs';\nimport lodash from 'lodash';\nconsole.table([]);\n",
		"index.test.ts":  "import express from 'express';\nconsole.log('tests may');\n",
		"__tests__/a.js": "import koa from 'koa';\n",
	}, ""))
	mods := forbiddenModules(fs)
	counts := map[string]int{}
	for _, m := range mods {
		counts[m]++
	}
	want := map[string]int{"express": 2, "commander": 1, "koa": 1, "hono": 1, "yargs": 1}
	for m, n := range want {
		if counts[m] != n {
			t.Errorf("%s reported %d times, want %d (all: %v)", m, counts[m], n, mods)
		}
	}
	if len(mods) != 6 {
		t.Errorf("forbidden imports %v, want 6", mods)
	}
	if got := byRule(fs, "library-stdout"); len(got) != 5 {
		t.Errorf("want one stdout finding each for console.log, warn, error, info, and process.stdout.write: %+v", got)
	}
}

func TestPortedNpmLibraryEntryPoints(t *testing.T) {
	for name, pkg := range map[string]string{
		"bin string": `{"name": "m", "main": "./index.js", "bin": "./cli.js"}`,
		"bin map":    `{"name": "m", "main": "./index.js", "bin": {"m": "./cli.js", "m2": "./cli.js"}}`,
	} {
		fs := analyze(t, npmLibrary(map[string]string{"package.json": pkg, "index.js": "export const x = 1;\n", "cli.js": "import './index.js';\n"}, ""))
		if got := byRule(fs, "library-entry-point"); len(got) == 0 {
			t.Errorf("%s: a library's bin must be reported", name)
		}
	}
	fs := analyze(t, npmLibrary(map[string]string{"index.js": "export const x = 1;\n"}, "[rules.library-forbidden-imports.forbidden]\nts = []\n"))
	if got := byRule(fs, "library-entry-point"); len(got) != 0 {
		t.Errorf("a library without bin reported an entry point: %+v", got)
	}
}

// Both allow lists merge: the workspace lint_allow list and the per-language
// allow list are each subtracted, and allowing a module that is not
// forbidden changes nothing.
func TestPortedAllowListsMerge(t *testing.T) {
	files := map[string]string{
		fixture.DeclarationsPath: fixture.Workspace("path = \"m\"\nname = \"m\"\nlibrary = true\nlint_allow = [\"click\", \"requests\"]\n"),
		"m/pyproject.toml":       "[project]\nname = \"m\"\n",
		"m/pkg/__init__.py":      "import click\nimport flask\nimport django\nimport requests\n",
		"strictcode.toml":        "format_version = 1\n[rules.library-forbidden-imports.allow]\npy = [\"flask\", \"numpy\"]\n",
	}
	if got := forbiddenModules(analyze(t, files)); len(got) != 1 || got[0] != "django" {
		t.Fatalf("want only django forbidden: %v", got)
	}
}
