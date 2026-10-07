package checks

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
)

// fakeTools answers tool runs by the argv after "uv run" and any group
// flags, recording every run.
type fakeTools struct {
	t       *testing.T
	answers map[string]ToolResult
	runs    []ToolRun
}

func newFakeTools(t *testing.T) *fakeTools {
	return &fakeTools{t: t, answers: map[string]ToolResult{
		"ruff --version": {Stdout: "ruff 0.15.20\n"},
	}}
}

func (f *fakeTools) run(run ToolRun) (ToolResult, error) {
	f.runs = append(f.runs, run)
	if len(run.Argv) < 2 || run.Argv[0] != "uv" || run.Argv[1] != "run" {
		f.t.Fatalf("a tool must run through uv run: %v", run.Argv)
	}
	if run.Timeout != ToolTimeout {
		f.t.Errorf("tool run without the tool timeout: %v", run.Timeout)
	}
	rest := run.Argv[2:]
	for len(rest) >= 2 && (rest[0] == "--group" || rest[0] == "--extra") {
		rest = rest[2:]
	}
	key := rest[0]
	if len(rest) > 1 {
		key += " " + rest[1]
	}
	if res, ok := f.answers[key]; ok {
		return res, nil
	}
	return ToolResult{}, nil
}

// toolRun is the recorded run whose argv contains sub, or fails the test.
func (f *fakeTools) toolRun(sub string) ToolRun {
	for _, r := range f.runs {
		if strings.Contains(strings.Join(r.Argv, " "), sub) {
			return r
		}
	}
	f.t.Fatalf("no run of %q among %v", sub, f.runs)
	return ToolRun{}
}

// pyProject is a single-project Python fixture with the given strictcode.toml
// and options entries.
func pyProject(strictcodeToml string, entries map[string]string) map[string]string {
	files := map[string]string{
		"pyproject.toml":   "[project]\nname = \"p\"\nversion = \"0.1.0\"\n",
		"pkg/__init__.py":  "",
		"tests/test_a.py":  "import pkg\n",
		"strictcode.toml":  "format_version = 1\n" + strictcodeToml,
		"scripts/build.py": "",
	}
	for k, v := range entries {
		files[k] = v
	}
	return files
}

// Lesson 34: a Python tool rule is off by default and runs nothing.
func TestLesson34ToolRulesAreOffByDefault(t *testing.T) {
	tools := newFakeTools(t)
	fs, err := analyzeWith(t, pyProject("[python_tools.lint]\npaths = [\"pkg\"]\n", nil), tools.run)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.runs) != 0 {
		t.Fatalf("a declaration alone ran a tool: %v", tools.runs)
	}
	for _, rule := range []string{"lint", "format", "type-check", "lint-scope-guard"} {
		if got := byRule(fs, rule); len(got) != 0 {
			t.Errorf("%s reported while off: %+v", rule, got)
		}
	}
}

// Lesson 34: switched on without a declaration, a tool rule is refused,
// naming the declaration to add; declaring it clears the refusal.
func TestLesson34OnWithoutADeclarationIsRefusedUntilDeclared(t *testing.T) {
	for _, rule := range []string{"lint", "format", "type-check"} {
		t.Run(rule, func(t *testing.T) {
			files := pyProject("", optionEntry("code", rule, "", "error", "error"))
			_, err := analyzeWith(t, files, newFakeTools(t).run)
			if err == nil {
				t.Fatal("an option on without a declaration was accepted")
			}
			if !strings.Contains(err.Error(), "[python_tools."+rule+"]") {
				t.Errorf("refusal does not name the declaration: %v", err)
			}
			files["strictcode.toml"] = "format_version = 1\n[python_tools." + rule + "]\npaths = [\"pkg\"]\n"
			if _, err := analyzeWith(t, files, newFakeTools(t).run); err != nil {
				t.Fatalf("declaring the paths did not clear the refusal: %v", err)
			}
		})
	}
}

// twoPyMembers is a workspace of two Python members, core and tools.
func twoPyMembers(strictcodeToml string, entries map[string]string) map[string]string {
	files := map[string]string{
		fixture.DeclarationsPath:  fixture.Workspace("path = \"core\"\nname = \"core\"\n", "path = \"tools\"\nname = \"tools\"\n"),
		"core/pyproject.toml":     "[project]\nname = \"core\"\nversion = \"1.0.0\"\n",
		"core/core/__init__.py":   "",
		"tools/pyproject.toml":    "[project]\nname = \"tools\"\nversion = \"1.0.0\"\n",
		"tools/tools/__init__.py": "",
		"strictcode.toml":         "format_version = 1\n" + strictcodeToml,
	}
	for k, v := range entries {
		files[k] = v
	}
	return files
}

// Lesson 34: an option on for a member whose directory no declared path
// lies in is refused, naming the member's path; adding a path inside it
// clears the refusal.
func TestLesson34OnForAnUncoveredMemberIsRefused(t *testing.T) {
	files := twoPyMembers("[python_tools.lint]\npaths = [\"core\"]\n", optionEntry("code", "lint", "tools", "error", "error"))
	_, err := analyzeWith(t, files, newFakeTools(t).run)
	if err == nil || !strings.Contains(err.Error(), `"tools"`) {
		t.Fatalf("uncovered member not refused by path: %v", err)
	}
	files["strictcode.toml"] = "format_version = 1\n[python_tools.lint]\npaths = [\"core\", \"tools/tools\"]\n"
	if _, err := analyzeWith(t, files, newFakeTools(t).run); err != nil {
		t.Fatalf("covering the member did not clear the refusal: %v", err)
	}
}

// Lesson 35: lint runs ruff check with JSON output through uv run in the
// declared directory over the declared paths, and reports each violation
// at the line ruff names, at the severity of the owning member's option.
func TestLesson35LintReportsEachRuffViolation(t *testing.T) {
	files := twoPyMembers("[python_tools.lint]\npaths = [\"core\", \"tools\"]\n", nil)
	files[".strictmetadata/options/manifest.toml"] = "owner = \"strictspec\"\n"
	files[".strictmetadata/options/code.toml"] = `format_version = 1

[[entry]]
id = "strictcode:lint"
scope = "core"
current = "error"
ideal = "error"
reason = "core adopts ruff"

[[entry]]
id = "strictcode:lint"
scope = "tools"
current = "warn"
ideal = "error"
reason = "tools is mid-cleanup"
`
	tools := newFakeTools(t)
	tools.answers["ruff check"] = ToolResult{ExitCode: 1, Stdout: `[
  {"code": "E501", "message": "Line too long (120 > 88)", "filename": "core/core/__init__.py", "location": {"row": 3, "column": 89}, "fix": null},
  {"code": "F401", "message": "os imported but unused", "filename": "tools/tools/__init__.py", "location": {"row": 1, "column": 8}, "fix": {"applicability": "safe"}}
]`}
	fs, err := analyzeWith(t, files, tools.run)
	if err != nil {
		t.Fatal(err)
	}
	got := byRule(fs, "lint")
	if len(got) != 2 {
		t.Fatalf("want one finding per ruff violation: %+v", got)
	}
	for _, f := range got {
		switch f.Target.File {
		case "core/core/__init__.py":
			if f.Severity != "error" || f.Target.Line != 3 || !strings.Contains(f.Message, "E501") {
				t.Errorf("core finding: %+v", f)
			}
		case "tools/tools/__init__.py":
			if f.Severity != "warning" || !strings.Contains(f.Message, "F401") {
				t.Errorf("tools finding at the tools member's value: %+v", f)
			}
		default:
			t.Errorf("finding at an unexpected file: %+v", f)
		}
	}
}

// Lesson 35: ruff names files absolutely; a finding is placed at the file
// relative to the workspace root.
func TestLesson35AbsoluteToolPathsAreMadeRootRelative(t *testing.T) {
	files := pyProject("[python_tools.lint]
paths = ["pkg"]
", optionEntry("code", "lint", "", "error", "error"))
	tools := newFakeTools(t)
	runner := func(run ToolRun) (ToolResult, error) {
		if strings.Contains(strings.Join(run.Argv, " "), "ruff check") {
			abs := filepath.Join(run.Dir, "pkg", "__init__.py")
			return ToolResult{ExitCode: 1, Stdout: `[{"code": "E711", "message": "Comparison to None", "filename": ` + strconvQuote(abs) + `, "location": {"row": 2, "column": 4}}]`}, nil
		}
		return tools.run(run)
	}
	fs, err := analyzeWith(t, files, runner)
	if err != nil {
		t.Fatal(err)
	}
	got := byRule(fs, "lint")
	if len(got) != 1 || got[0].Target.File != "pkg/__init__.py" || got[0].Target.Line != 2 {
		t.Fatalf("absolute path not made root-relative: %+v", got)
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Lesson 35: the composed command lines, verbatim, and the directory each
// runs in.
func TestLesson35ToolCommandLines(t *testing.T) {
	cases := map[string][]string{
		"lint":       {"uv", "run", "ruff", "check", "--output-format=json", "--quiet", "pkg", "tests"},
		"format":     {"uv", "run", "ruff", "format", "--check", "pkg", "tests"},
		"type-check": {"uv", "run", "mypy", "pkg", "tests"},
	}
	for rule, want := range cases {
		t.Run(rule, func(t *testing.T) {
			files := pyProject("[python_tools."+rule+"]\npaths = [\"pkg\", \"tests\"]\n", optionEntry("code", rule, "", "error", "error"))
			tools := newFakeTools(t)
			if _, err := analyzeWith(t, files, tools.run); err != nil {
				t.Fatal(err)
			}
			run := tools.toolRun(strings.Join(want[2:4], " "))
			if !reflect.DeepEqual(run.Argv, want) {
				t.Errorf("argv %v, want %v", run.Argv, want)
			}
		})
	}
}

func TestLesson35CwdIsRelativeToTheWorkspaceRoot(t *testing.T) {
	files := pyProject("[python_tools.format]\ncwd = \"pkg\"\npaths = [\".\"]\n", optionEntry("code", "format", "", "error", "error"))
	tools := newFakeTools(t)
	if _, err := analyzeWith(t, files, tools.run); err != nil {
		t.Fatal(err)
	}
	run := tools.toolRun("ruff format")
	if filepath.Base(run.Dir) != "pkg" {
		t.Errorf("format ran in %s, want the declared pkg directory", run.Dir)
	}
}

// Lesson 35: ruff older than the floor lint's output parsing is built
// against is refused before lint runs.
func TestLesson35OldRuffIsRefused(t *testing.T) {
	files := pyProject("[python_tools.lint]\npaths = [\"pkg\"]\n", optionEntry("code", "lint", "", "error", "error"))
	tools := newFakeTools(t)
	tools.answers["ruff --version"] = ToolResult{Stdout: "ruff 0.15.19\n"}
	_, err := analyzeWith(t, files, tools.run)
	if err == nil || !strings.Contains(err.Error(), "0.15.19") || !strings.Contains(err.Error(), "0.15.20") {
		t.Fatalf("old ruff not refused with both versions named: %v", err)
	}
	for _, r := range tools.runs {
		if strings.Contains(strings.Join(r.Argv, " "), "ruff check") {
			t.Fatal("lint ran with a ruff below the floor")
		}
	}
}

// Lesson 35: a failing tool whose output the rule cannot read is an error
// carrying the tool's own output, never a pass.
func TestLesson35UnreadableToolOutputIsAnError(t *testing.T) {
	cases := map[string]ToolResult{
		"ruff check":  {ExitCode: 2, Stderr: "ruff failed: invalid configuration"},
		"ruff format": {ExitCode: 2, Stderr: "error: Failed to parse pyproject.toml"},
		"mypy pkg":    {ExitCode: 2, Stderr: "mypy: can't read file 'pkg': No such file"},
	}
	rules := map[string]string{"ruff check": "lint", "ruff format": "format", "mypy pkg": "type-check"}
	for key, res := range cases {
		rule := rules[key]
		t.Run(rule, func(t *testing.T) {
			files := pyProject("[python_tools."+rule+"]\npaths = [\"pkg\"]\n", optionEntry("code", rule, "", "error", "error"))
			tools := newFakeTools(t)
			tools.answers[key] = res
			_, err := analyzeWith(t, files, tools.run)
			if err == nil {
				t.Fatal("a failed tool run passed")
			}
			if !strings.Contains(err.Error(), strings.SplitN(res.Stderr, ":", 2)[0]) {
				t.Errorf("error does not carry the tool's output: %v", err)
			}
		})
	}
}

func TestLesson35FormatAndTypeCheckReportEachProblem(t *testing.T) {
	files := pyProject("[python_tools.format]\npaths = [\"pkg\"]\n\n[python_tools.type-check]\npaths = [\"pkg\"]\n", nil)
	files[".strictmetadata/options/manifest.toml"] = "owner = \"strictspec\"\n"
	files[".strictmetadata/options/code.toml"] = "format_version = 1\n\n" +
		"[[entry]]\nid = \"strictcode:format\"\ncurrent = \"error\"\nideal = \"error\"\nreason = \"adopted\"\n\n" +
		"[[entry]]\nid = \"strictcode:type-check\"\ncurrent = \"error\"\nideal = \"error\"\nreason = \"adopted\"\n"
	tools := newFakeTools(t)
	tools.answers["ruff format"] = ToolResult{ExitCode: 1, Stdout: "Would reformat: pkg/__init__.py\nWould reformat: pkg/b.py\n2 files would be reformatted\n"}
	tools.answers["mypy pkg"] = ToolResult{ExitCode: 1, Stdout: "pkg/__init__.py:7: error: Incompatible return value type (got \"int\", expected \"str\")  [return-value]\npkg/__init__.py:7: note: see https://mypy.readthedocs.io\nFound 1 error in 1 file (checked 1 source file)\n"}
	fs, err := analyzeWith(t, files, tools.run)
	if err != nil {
		t.Fatal(err)
	}
	if got := byRule(fs, "format"); len(got) != 2 || got[0].Target.File != "pkg/__init__.py" {
		t.Errorf("format findings: %+v", got)
	}
	got := byRule(fs, "type-check")
	if len(got) != 1 || got[0].Target.Line != 7 || !strings.Contains(got[0].Message, "return-value") {
		t.Errorf("type-check findings: %+v", got)
	}
}

// Lesson 36: the uv run flags reach the tool where the project declares it.
func TestLesson36GroupFlags(t *testing.T) {
	cases := []struct {
		pyproject string
		want      []string
	}{
		{"[project]\nname = \"p\"\n\n[dependency-groups]\nlint = [\"ruff>=0.15.20\"]\n", []string{"--group", "lint"}},
		{"[project]\nname = \"p\"\n\n[dependency-groups]\ndev = [\"ruff\"]\n", nil},
		{"[project]\nname = \"p\"\n\n[project.optional-dependencies]\nqa = [\"ruff[extra]\"]\n", []string{"--extra", "qa"}},
		{"[project]\nname = \"p\"\n\n[dependency-groups]\nlint = [\"ruffian\"]\n", nil},
		{"[project]\nname = \"p\"\n\n[tool.uv.workspace]\nmembers = [\"x\"]\n\n[dependency-groups]\nlint = [\"ruff\"]\n", nil},
	}
	for _, c := range cases {
		root := fixture.Write(t, map[string]string{"pyproject.toml": c.pyproject})
		got, err := uvGroupFlags(root, ".", "ruff")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: flags %v, want %v", c.pyproject, got, c.want)
		}
	}
}

// Lesson 37: a nested member is left out of an enclosing member's ruff run,
// since its files belong to its own declared path.
func TestLesson37NestedMembersAreLeftOutOfTheEnclosingRun(t *testing.T) {
	files := map[string]string{
		fixture.DeclarationsPath:       fixture.Workspace("path = \"sdk\"\nname = \"sdk\"\n", "path = \"sdk/python\"\nname = \"sdk-python\"\n", "path = \"sdk/npm\"\nname = \"sdk-npm\"\n"),
		"sdk/pyproject.toml":           "[project]\nname = \"sdk\"\n",
		"sdk/sdk/__init__.py":          "",
		"sdk/python/pyproject.toml":    "[project]\nname = \"sdk-python\"\n",
		"sdk/python/sdkpy/__init__.py": "",
		"sdk/npm/package.json":         `{"name": "sdk-npm", "main": "./index.js"}`,
		"sdk/npm/index.js":             "export const x = 1;\n",
		"strictcode.toml":              "format_version = 1\n[python_tools.lint]\npaths = [\"sdk\"]\n",
	}
	for k, v := range optionEntry("code", "lint", "sdk", "error", "error") {
		files[k] = v
	}
	tools := newFakeTools(t)
	if _, err := analyzeWith(t, files, tools.run); err != nil {
		t.Fatal(err)
	}
	argv := tools.toolRun("ruff check").Argv
	i := indexOf(argv, "--extend-exclude")
	if i < 0 || argv[i+1] != "sdk/npm,sdk/python" {
		t.Fatalf("nested members not excluded: %v", argv)
	}
}

func indexOf(items []string, want string) int {
	for i, it := range items {
		if it == want {
			return i
		}
	}
	return -1
}

// Lesson 38: the scope guards report a tool's own configuration competing
// with the declared paths, run only while their tool rule runs, and leave
// exclusion keys alone.
func TestLesson38ScopeGuards(t *testing.T) {
	cases := []struct {
		rule, guard, file, content, key string
	}{
		{"lint", "lint-scope-guard", "pyproject.toml", "[project]\nname = \"p\"\n\n[tool.ruff]\ninclude = [\"src/**\"]\n", "include"},
		{"format", "format-scope-guard", "ruff.toml", "extend-include = [\"*.pyi\"]\n", "extend-include"},
		{"type-check", "type-check-scope-guard", "pyproject.toml", "[project]\nname = \"p\"\n\n[tool.mypy]\nfiles = \"src\"\n", "files"},
		{"type-check", "type-check-scope-guard", "mypy.ini", "[mypy]\nstrict = True\npackages = pkg\n", "packages"},
		{"type-check", "type-check-scope-guard", "setup.cfg", "[metadata]\nname = p\n\n[mypy]\nmodules: pkg.a\n", "modules"},
	}
	for _, c := range cases {
		t.Run(c.guard+"/"+c.file, func(t *testing.T) {
			files := pyProject("[python_tools."+c.rule+"]\npaths = [\"pkg\"]\n", optionEntry("code", c.rule, "", "error", "error"))
			files[c.file] = c.content
			fs, err := analyzeWith(t, files, newFakeTools(t).run)
			if err != nil {
				t.Fatal(err)
			}
			got := byRule(fs, c.guard)
			if len(got) != 1 || !strings.Contains(got[0].Message, "'"+c.key+"'") || got[0].Target.File != c.file {
				t.Fatalf("guard findings: %+v", got)
			}
			if got[0].Target.Line < 2 {
				t.Errorf("guard finding does not point at the key's line: %+v", got[0])
			}

			// With the tool rule off, the guard has no declared scope to
			// protect and reports nothing.
			delete(files, ".strictmetadata/options/code.toml")
			fs, err = analyzeWith(t, files, newFakeTools(t).run)
			if err != nil {
				t.Fatal(err)
			}
			if got := byRule(fs, c.guard); len(got) != 0 {
				t.Fatalf("guard reported while its tool rule is off: %+v", got)
			}
		})
	}
}

func TestLesson38ExclusionKeysAreExempt(t *testing.T) {
	files := pyProject("[python_tools.lint]\npaths = [\"pkg\"]\n", optionEntry("code", "lint", "", "error", "error"))
	files["pyproject.toml"] = "[project]\nname = \"p\"\n\n[tool.ruff]\nexclude = [\"build\"]\nextend-exclude = [\"gen\"]\nforce-exclude = true\n"
	fs, err := analyzeWith(t, files, newFakeTools(t).run)
	if err != nil {
		t.Fatal(err)
	}
	if got := byRule(fs, "lint-scope-guard"); len(got) != 0 {
		t.Fatalf("an exclusion key was reported: %+v", got)
	}
}

// Lesson 39: strictspec-certificate is off by default; switched on, it is
// refused without a [strictspec_certificate] declaration, and once declared
// it reports each reason the certificate blocks at the error severity.
func TestLesson39StrictspecCertificate(t *testing.T) {
	certJSON := `{"certificate_format_version": 1, "claims": [{"kind": "flip-scan", "grade": "violated", "statement": "narrowing without a bump"}]}`
	files := pyProject("", map[string]string{"migrations/cert.json": certJSON})
	fs, err := analyzeWith(t, files, newFakeTools(t).run)
	if err != nil {
		t.Fatal(err)
	}
	if got := byRule(fs, "strictspec-certificate"); len(got) != 0 {
		t.Fatalf("the rule ran while off: %+v", got)
	}

	files = withFiles(files, optionEntry("release", "strictspec-certificate", "", "error", "error"))
	_, err = analyzeWith(t, files, newFakeTools(t).run)
	if err == nil || !strings.Contains(err.Error(), "[strictspec_certificate]") {
		t.Fatalf("on without a declaration was not refused: %v", err)
	}

	files["strictcode.toml"] = "format_version = 1\n[strictspec_certificate]\ncertificate = \"migrations/cert.json\"\n"
	fs, err = analyzeWith(t, files, newFakeTools(t).run)
	if err != nil {
		t.Fatalf("declaring the certificate did not clear the refusal: %v", err)
	}
	got := byRule(fs, "strictspec-certificate")
	if len(got) != 1 || got[0].Severity != "error" || got[0].Target.File != "migrations/cert.json" || !strings.Contains(got[0].Message, "violated") {
		t.Fatalf("certificate findings: %+v", got)
	}
}
