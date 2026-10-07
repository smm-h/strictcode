package checks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/options"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
	"github.com/stricttools/strictspec/go/strictspec"
)

// ToolRun is one external tool invocation: Argv run in Dir, stopped after
// Timeout.
type ToolRun struct {
	// Dir is the absolute directory the command runs in.
	Dir     string
	Argv    []string
	Timeout time.Duration
}

// ToolResult is a finished tool invocation.
type ToolResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// ToolRunner runs one tool invocation. An error means the command did not
// run to completion (it could not start, or the timeout stopped it); a
// non-zero exit is a result, not an error.
type ToolRunner func(run ToolRun) (ToolResult, error)

// ToolTimeout bounds every tool invocation, the timeout rlsbl gave its
// checks when a project declared none.
const ToolTimeout = 900 * time.Second

// ruffMinimum is the oldest ruff whose JSON output (code, message, fix,
// location, filename) and default rule set lint is built against.
var ruffMinimum = [3]int{0, 15, 20}

// pythonTool is how one Python tool rule runs.
type pythonTool struct {
	// binary is the tool's executable, also the package name its project
	// declares it under.
	binary string
	// args follow the binary and come before the paths.
	args []string
	// guard is the rule checking the tool's own configuration for a
	// competing scope.
	guard string
	// family names whose configuration the guard reads: "ruff" or "mypy".
	family string
}

// pythonTools are the Python tool rules, by rule ID.
var pythonTools = map[string]pythonTool{
	"lint":       {binary: "ruff", args: []string{"check", "--output-format=json", "--quiet"}, guard: "lint-scope-guard", family: "ruff"},
	"format":     {binary: "ruff", args: []string{"format", "--check"}, guard: "format-scope-guard", family: "ruff"},
	"type-check": {binary: "mypy", args: nil, guard: "type-check-scope-guard", family: "mypy"},
}

// toolPlan is what one Python tool rule runs: the declaration, and the
// member paths whose option is on.
type toolPlan struct {
	rule string
	decl config.PythonTool
	// on are the member paths at which the rule's option is not off.
	on []string
}

// planPythonTool decides whether a Python tool rule runs. It does not run
// (nil, nil) while its option is off at every member. It is refused while
// on with no [python_tools.<rule>] declaration, or on for a member whose
// directory no declared path lies in.
func planPythonTool(ctx *Context, rule string) (*toolPlan, error) {
	ws := ctx.View.WS
	paths := make([]string, 0, len(ws.Members))
	for _, m := range ws.Members {
		paths = append(paths, m.Path)
	}
	on := ctx.Opts.OnPaths(rule, paths)
	if len(on) == 0 {
		return nil, nil
	}
	decl, ok := ctx.Cfg.PythonTools[rule]
	if !ok {
		return nil, fmt.Errorf("%s: strictcode:%s is on for the member path(s) %s, but %s declares no [python_tools.%s]: declare paths = [...] there, or switch the option off",
			rule, rule, quoteJoin(on), ctx.CfgPath, rule)
	}
	covered := map[string]bool{}
	for _, rp := range decl.RootPaths() {
		if owner := ws.Owner(rp); owner != nil {
			covered[owner.Path] = true
		}
	}
	var uncovered []string
	for _, p := range on {
		if !covered[p] {
			uncovered = append(uncovered, p)
		}
	}
	if len(uncovered) != 0 {
		return nil, fmt.Errorf("%s: strictcode:%s is on for the member path(s) %s, which no path of [python_tools.%s] in %s lies in: add a path inside each to the declaration, or switch the option off for it",
			rule, rule, quoteJoin(uncovered), rule, ctx.CfgPath)
	}
	return &toolPlan{rule: rule, decl: decl, on: on}, nil
}

func quoteJoin(items []string) string {
	q := make([]string, 0, len(items))
	for _, it := range items {
		q = append(q, strconv.Quote(it))
	}
	return strings.Join(q, ", ")
}

// runTool runs the plan's tool with extra arguments before the declared
// paths, through uv run in the declaration's directory.
func (ctx *Context) runTool(plan *toolPlan, extra []string) (ToolResult, error) {
	tool := pythonTools[plan.rule]
	dir := filepath.Join(ctx.View.WS.Root, filepath.FromSlash(plan.decl.Cwd))
	flags, err := uvGroupFlags(ctx.View.WS.Root, plan.decl.Cwd, tool.binary)
	if err != nil {
		return ToolResult{}, err
	}
	argv := append([]string{"uv", "run"}, flags...)
	argv = append(argv, tool.binary)
	argv = append(argv, tool.args...)
	argv = append(argv, extra...)
	argv = append(argv, plan.decl.Paths...)
	res, err := ctx.Runner(ToolRun{Dir: dir, Argv: argv, Timeout: ToolTimeout})
	if err != nil {
		return ToolResult{}, fmt.Errorf("%s: `%s` in %s did not run: %w", plan.rule, strings.Join(argv, " "), plan.decl.Cwd, err)
	}
	return res, nil
}

// toolFinding reports one problem a tool found in file (relative to the
// declaration's directory, or absolute) at line, attributed to the member
// owning the file, at that member's option value; ok is false when that
// member's option is off.
func (ctx *Context) toolFinding(plan *toolPlan, file string, line int, message string) (findings.Finding, bool) {
	ws := ctx.View.WS
	rel := file
	if filepath.IsAbs(file) {
		if r, err := filepath.Rel(ws.Root, file); err == nil {
			rel = filepath.ToSlash(r)
		}
	} else {
		rel = filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(plan.decl.Cwd), file)))
	}
	owner := ws.Owner(rel)
	if owner == nil {
		owner = ws.Members[0]
	}
	v := ctx.Opts.ValueFor(plan.rule, owner.Path)
	if v == options.Off {
		return findings.Finding{}, false
	}
	return ctx.findingAtLine(plan.rule, options.Severity(v),
		memberTargetID(ctx, vocab.LangPy, owner.Name), vocab.NodeKindWorkspaceMember,
		rel, line, message), true
}

// toolFailure is the error for a tool run whose output is not what the rule
// reads.
func toolFailure(rule string, res ToolResult, why string) error {
	detail := strings.TrimSpace(res.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(res.Stdout)
	}
	if len(detail) > 2000 {
		detail = detail[:2000] + "..."
	}
	return fmt.Errorf("%s: the tool exited %d and %s: %s", rule, res.ExitCode, why, detail)
}

// nestedExcludes lists, relative to the declaration's directory, the
// members nested inside a declared path, so ruff leaves their files to
// their own declared paths: a nested member whose option is on must have a
// declared path of its own (planPythonTool refuses it otherwise), and ruff
// does not apply an exclusion to a path given on its command line.
func (ctx *Context) nestedExcludes(plan *toolPlan) []string {
	ws := ctx.View.WS
	var out []string
	for _, rp := range plan.decl.RootPaths() {
		for _, m := range ws.Members {
			if m.Path == rp || m.Path == "." || !workspace.IsInside(m.Path, rp) {
				continue
			}
			rel, err := filepath.Rel(filepath.FromSlash(plan.decl.Cwd), filepath.FromSlash(m.Path))
			if err != nil {
				continue
			}
			out = append(out, filepath.ToSlash(rel))
		}
	}
	sort.Strings(out)
	return dedupe(out)
}

func dedupe(sorted []string) []string {
	var out []string
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}

// --- lint ------------------------------------------------------------------

// ruffViolation is one entry of ruff's JSON output.
type ruffViolation struct {
	Code     *string `json:"code"`
	Message  string  `json:"message"`
	Filename string  `json:"filename"`
	Location *struct {
		Row    int `json:"row"`
		Column int `json:"column"`
	} `json:"location"`
}

var versionTriple = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// checkLint runs ruff check over the declared paths and reports every
// violation ruff's JSON output names, at the option value of the member
// owning its file. ruff older than ruffMinimum is refused.
func checkLint(ctx *Context) []findings.Finding {
	plan, err := planPythonTool(ctx, "lint")
	if err != nil {
		ctx.fail(err)
		return nil
	}
	if plan == nil {
		return nil
	}
	if err := ctx.requireRuffMinimum(plan); err != nil {
		ctx.fail(err)
		return nil
	}
	var extra []string
	if ex := ctx.nestedExcludes(plan); len(ex) != 0 {
		extra = []string{"--extend-exclude", strings.Join(ex, ",")}
	}
	res, err := ctx.runTool(plan, extra)
	if err != nil {
		ctx.fail(err)
		return nil
	}
	if res.ExitCode == 0 {
		return nil
	}
	var violations []ruffViolation
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &violations); err != nil {
		ctx.fail(toolFailure("lint", res, "its output is not ruff's JSON"))
		return nil
	}
	if len(violations) == 0 {
		ctx.fail(toolFailure("lint", res, "reported no violation"))
		return nil
	}
	var out []findings.Finding
	for _, v := range violations {
		code := "?"
		if v.Code != nil && *v.Code != "" {
			code = *v.Code
		}
		line := 1
		if v.Location != nil {
			line = v.Location.Row
		}
		if f, ok := ctx.toolFinding(plan, v.Filename, line, fmt.Sprintf("ruff %s: %s", code, v.Message)); ok {
			out = append(out, f)
		}
	}
	return out
}

// requireRuffMinimum refuses a ruff older than ruffMinimum.
func (ctx *Context) requireRuffMinimum(plan *toolPlan) error {
	tool := pythonTools[plan.rule]
	flags, err := uvGroupFlags(ctx.View.WS.Root, plan.decl.Cwd, tool.binary)
	if err != nil {
		return err
	}
	argv := append(append([]string{"uv", "run"}, flags...), "ruff", "--version")
	dir := filepath.Join(ctx.View.WS.Root, filepath.FromSlash(plan.decl.Cwd))
	res, err := ctx.Runner(ToolRun{Dir: dir, Argv: argv, Timeout: ToolTimeout})
	if err != nil {
		return fmt.Errorf("%s: `%s` did not run: %w", plan.rule, strings.Join(argv, " "), err)
	}
	m := versionTriple.FindStringSubmatch(res.Stdout + " " + res.Stderr)
	if res.ExitCode != 0 || m == nil {
		return toolFailure(plan.rule, res, "its output names no ruff version")
	}
	var got [3]int
	for i := range got {
		got[i], _ = strconv.Atoi(m[i+1])
	}
	for i := range got {
		if got[i] != ruffMinimum[i] {
			if got[i] < ruffMinimum[i] {
				return fmt.Errorf("%s: ruff %d.%d.%d is older than %d.%d.%d, the oldest whose output lint reads: upgrade ruff in the project's environment",
					plan.rule, got[0], got[1], got[2], ruffMinimum[0], ruffMinimum[1], ruffMinimum[2])
			}
			break
		}
	}
	return nil
}

// --- format ----------------------------------------------------------------

var wouldReformat = regexp.MustCompile(`(?m)^Would reformat: (.+)$`)

// checkFormat runs ruff format --check over the declared paths and reports
// every file it would reformat. ruff exits 1 when it would reformat a file;
// any other non-zero exit is a failure to check.
func checkFormat(ctx *Context) []findings.Finding {
	plan, err := planPythonTool(ctx, "format")
	if err != nil {
		ctx.fail(err)
		return nil
	}
	if plan == nil {
		return nil
	}
	var extra []string
	if ex := ctx.nestedExcludes(plan); len(ex) != 0 {
		extra = []string{"--extend-exclude", strings.Join(ex, ",")}
	}
	res, err := ctx.runTool(plan, extra)
	if err != nil {
		ctx.fail(err)
		return nil
	}
	if res.ExitCode == 0 {
		return nil
	}
	matches := wouldReformat.FindAllStringSubmatch(res.Stdout+"\n"+res.Stderr, -1)
	if res.ExitCode != 1 || len(matches) == 0 {
		ctx.fail(toolFailure("format", res, "named no file it would reformat"))
		return nil
	}
	var out []findings.Finding
	for _, m := range matches {
		file := strings.TrimSpace(m[1])
		if f, ok := ctx.toolFinding(plan, file, 1, "ruff format would reformat this file"); ok {
			out = append(out, f)
		}
	}
	return out
}

// --- type-check ------------------------------------------------------------

var mypyError = regexp.MustCompile(`(?m)^(.+?):(\d+)(?::\d+)?: error: (.*)$`)

// checkTypeCheck runs mypy over the declared paths and reports every error
// line it prints. mypy exits 1 when it found errors; any other non-zero exit
// is a failure to check.
func checkTypeCheck(ctx *Context) []findings.Finding {
	plan, err := planPythonTool(ctx, "type-check")
	if err != nil {
		ctx.fail(err)
		return nil
	}
	if plan == nil {
		return nil
	}
	res, err := ctx.runTool(plan, nil)
	if err != nil {
		ctx.fail(err)
		return nil
	}
	if res.ExitCode == 0 {
		return nil
	}
	matches := mypyError.FindAllStringSubmatch(res.Stdout, -1)
	if res.ExitCode != 1 || len(matches) == 0 {
		ctx.fail(toolFailure("type-check", res, "printed no error line"))
		return nil
	}
	var out []findings.Finding
	for _, m := range matches {
		line, _ := strconv.Atoi(m[2])
		if f, ok := ctx.toolFinding(plan, m[1], line, "mypy: "+m[3]); ok {
			out = append(out, f)
		}
	}
	return out
}

// --- uv group flags --------------------------------------------------------

// uvGroupFlags are the uv run flags reaching binary from the project in
// cwd (workspace-root-relative): none when cwd lies in a uv workspace, when
// the project declares the tool in its "dev" dependency group or in uv's
// dev-dependencies, or when it does not declare it; --group <name> for
// another dependency group; --extra <name> for an optional-dependencies
// extra. So the common case is a bare `uv run <tool>`.
func uvGroupFlags(root, cwd, binary string) ([]string, error) {
	inWorkspace, err := inUVWorkspace(root, cwd)
	if err != nil || inWorkspace {
		return nil, err
	}
	doc, ok, err := readTOML(filepath.Join(root, filepath.FromSlash(cwd), "pyproject.toml"))
	if err != nil || !ok {
		return nil, err
	}
	declares := regexp.MustCompile(`^` + regexp.QuoteMeta(binary) + `(\[|[<>=!~; ]|$)`)
	declaresIn := func(list strictspec.Value) bool {
		for _, item := range list.Items() {
			if s, isStr := item.AsString(); isStr && declares.MatchString(strings.TrimSpace(s)) {
				return true
			}
		}
		return false
	}
	if groups, ok := doc.Field("dependency-groups"); ok {
		for _, kv := range groups.Entries() {
			if declaresIn(kv.Value) {
				if kv.Key == "dev" {
					return nil, nil
				}
				return []string{"--group", kv.Key}, nil
			}
		}
	}
	if project, ok := doc.Field("project"); ok {
		if extras, ok := project.Field("optional-dependencies"); ok {
			for _, kv := range extras.Entries() {
				if declaresIn(kv.Value) {
					return []string{"--extra", kv.Key}, nil
				}
			}
		}
	}
	return nil, nil
}

// inUVWorkspace reports whether cwd, or a directory above it up to the
// workspace root, holds a pyproject.toml declaring [tool.uv.workspace].
func inUVWorkspace(root, cwd string) (bool, error) {
	dir := cwd
	for {
		doc, ok, err := readTOML(filepath.Join(root, filepath.FromSlash(dir), "pyproject.toml"))
		if err != nil {
			return false, err
		}
		if ok {
			if tool, has := doc.Field("tool"); has {
				if uv, has := tool.Field("uv"); has {
					if _, has := uv.Field("workspace"); has {
						return true, nil
					}
				}
			}
		}
		if dir == "." {
			return false, nil
		}
		dir = filepath.ToSlash(filepath.Dir(filepath.FromSlash(dir)))
	}
}

// readTOML parses the TOML file at path; ok is false when it does not exist.
func readTOML(path string) (strictspec.Value, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return strictspec.Value{}, false, nil
	}
	if err != nil {
		return strictspec.Value{}, false, err
	}
	doc, err := strictspec.LoadValue(raw, "toml")
	if err != nil {
		return strictspec.Value{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return doc, true, nil
}

// --- scope guards ----------------------------------------------------------

// scopeConflict is one key of a tool's own configuration competing with the
// declared paths.
type scopeConflict struct {
	// file is workspace-root-relative.
	file string
	line int
	// source names the table: "pyproject.toml [tool.ruff]".
	source string
	key    string
}

// guardExplanation says why a competing key is a problem, per family.
var guardExplanation = map[string]string{
	"ruff": "ruff's include and extend-include silently narrow the paths passed on its command line, so the declared scope would shrink unseen",
	"mypy": "paths on mypy's command line silently override its files, packages, and modules settings, so this scope is dead but reads as authoritative",
}

// checkScopeGuard reports the keys of a Python tool's own configuration, in
// the declaration's directory, that compete with the declared paths. It
// runs only while its paired tool rule runs; exclude-style keys are exempt,
// because an explicit path bypasses them.
func checkScopeGuard(ctx *Context, rule string) []findings.Finding {
	plan, err := planPythonTool(ctx, rule)
	if err != nil || plan == nil {
		// The paired rule reports a refused plan.
		return nil
	}
	tool := pythonTools[rule]
	dir := plan.decl.Cwd
	var conflicts []scopeConflict
	if tool.family == "ruff" {
		conflicts, err = ruffScopeConflicts(ctx.View.WS.Root, dir)
	} else {
		conflicts, err = mypyScopeConflicts(ctx.View.WS.Root, dir)
	}
	if err != nil {
		ctx.fail(fmt.Errorf("%s: %w", tool.guard, err))
		return nil
	}
	owner := ctx.View.WS.Owner(dir)
	if owner == nil {
		owner = ctx.View.WS.Members[0]
	}
	var out []findings.Finding
	for _, c := range conflicts {
		out = append(out, ctx.findingAtLine(tool.guard, options.Severity(ctx.Opts.Value(tool.guard)),
			memberTargetID(ctx, vocab.LangPy, owner.Name), vocab.NodeKindWorkspaceMember,
			c.file, c.line,
			fmt.Sprintf("%s: '%s' competes with the paths [python_tools.%s] declares (%s)",
				c.source, c.key, rule, guardExplanation[tool.family])))
	}
	return out
}

func checkLintScopeGuard(ctx *Context) []findings.Finding { return checkScopeGuard(ctx, "lint") }

func checkFormatScopeGuard(ctx *Context) []findings.Finding { return checkScopeGuard(ctx, "format") }

func checkTypeCheckScopeGuard(ctx *Context) []findings.Finding {
	return checkScopeGuard(ctx, "type-check")
}

// ruffScopeConflicts finds include and extend-include in pyproject.toml's
// [tool.ruff], ruff.toml, and .ruff.toml.
func ruffScopeConflicts(root, dir string) ([]scopeConflict, error) {
	keys := []string{"include", "extend-include"}
	var out []scopeConflict
	pyproject := joinRel(dir, "pyproject.toml")
	doc, ok, err := readTOML(filepath.Join(root, filepath.FromSlash(pyproject)))
	if err != nil {
		return nil, err
	}
	if ok {
		if tool, has := doc.Field("tool"); has {
			if ruff, has := tool.Field("ruff"); has {
				for _, k := range keys {
					if _, has := ruff.Field(k); has {
						out = append(out, scopeConflict{pyproject, keyLine(root, pyproject, k), "pyproject.toml [tool.ruff]", k})
					}
				}
			}
		}
	}
	for _, name := range []string{"ruff.toml", ".ruff.toml"} {
		file := joinRel(dir, name)
		doc, ok, err := readTOML(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for _, k := range keys {
			if _, has := doc.Field(k); has {
				out = append(out, scopeConflict{file, keyLine(root, file, k), name, k})
			}
		}
	}
	return out, nil
}

// mypyScopeConflicts finds files, packages, and modules in pyproject.toml's
// [tool.mypy], in the [mypy] section of mypy.ini and .mypy.ini, and in
// setup.cfg's [mypy] section.
func mypyScopeConflicts(root, dir string) ([]scopeConflict, error) {
	keys := []string{"files", "packages", "modules"}
	var out []scopeConflict
	pyproject := joinRel(dir, "pyproject.toml")
	doc, ok, err := readTOML(filepath.Join(root, filepath.FromSlash(pyproject)))
	if err != nil {
		return nil, err
	}
	if ok {
		if tool, has := doc.Field("tool"); has {
			if mypy, has := tool.Field("mypy"); has {
				for _, k := range keys {
					if _, has := mypy.Field(k); has {
						out = append(out, scopeConflict{pyproject, keyLine(root, pyproject, k), "pyproject.toml [tool.mypy]", k})
					}
				}
			}
		}
	}
	for _, name := range []string{"mypy.ini", ".mypy.ini", "setup.cfg"} {
		file := joinRel(dir, name)
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if line := iniKeyLine(string(raw), "mypy", k); line > 0 {
				out = append(out, scopeConflict{file, line, name + " [mypy]", k})
			}
		}
	}
	return out, nil
}

// iniKeyLine is the 1-based line of key in an INI file's [section], or 0.
// A key is written "key = value" or "key: value", case-insensitively, as
// Python's configparser reads it.
func iniKeyLine(text, section, key string) int {
	in := false
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			in = strings.TrimSpace(line[1:len(line)-1]) == section
			continue
		}
		if !in || line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || raw != strings.TrimLeft(raw, " \t") {
			continue
		}
		name := line
		if j := strings.IndexAny(line, "=:"); j >= 0 {
			name = line[:j]
		}
		if strings.EqualFold(strings.TrimSpace(name), key) {
			return i + 1
		}
	}
	return 0
}

// keyLine is the 1-based line where a TOML key is first assigned in the
// workspace-root-relative file, or 1 when no line starts with it.
func keyLine(root, file, key string) int {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return 1
	}
	for i, line := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, key) {
			rest := strings.TrimSpace(t[len(key):])
			if strings.HasPrefix(rest, "=") {
				return i + 1
			}
		}
	}
	return 1
}

// joinRel joins a workspace-root-relative directory and a name.
func joinRel(dir, name string) string {
	if dir == "." {
		return name
	}
	return dir + "/" + name
}
