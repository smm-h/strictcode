// Command strictcode is the strictcode CLI, built on strictcli (flag
// conventions enforced at registration; --dump-schema auto-injected).
//
// Surface: analyze (the batch pipeline — extract, check, report, exit
// code), registry dump, and matrix gen (the two committed artifacts CI
// diffs).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/smm-h/strictcli/go/strictcli"
	strictcode "github.com/smm-h/strictcode"
	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/engine"
	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/fix"
	"github.com/smm-h/strictcode/internal/registrydump"
	"github.com/smm-h/strictcode/internal/workspace"
)

func main() {
	newApp().Run()
}

// newApp builds the CLI. strictcli validates every flag and command at
// registration time, so building the app is itself a conformance check
// (exercised by the tests).
func newApp() *strictcli.App {
	app := strictcli.NewApp("strictcode", strictcode.Version,
		"strictcode: a deterministic linter for architecture, with tiered auto-fixes")

	app.Command("analyze", "Analyze a project or workspace directory and report findings",
		analyzeHandler,
		// read_only: analyze reads the workspace and its config and writes
		// only to stdout/stderr. It creates, modifies and deletes nothing.
		strictcli.WithEffect(strictcli.EffectReadOnly),
		strictcli.WithArgs(
			strictcli.NewArg("dir", "Project or workspace root to analyze",
				strictcli.ArgRequired(false), strictcli.ArgDefault(".")),
		),
		strictcli.WithFlags(
			strictcli.StringFlag("config", "Config file name, resolved relative to the analyzed directory",
				strictcli.Default("strictcode.toml")),
		),
		// Machine output is the framework's --json, and the findings document
		// is this command's payload. strictcode declares no output-format flag
		// of its own: the enum's other value was the human report, which is
		// what the command prints when it is not asked for a document.
		strictcli.PayloadSchema(findings.Schema),
	)

	app.Command("fix", "Apply tier-1 (guaranteed behavior-preserving) fixes with post-fix graph re-verification",
		fixHandler,
		// mutating: --apply rewrites source files in place.
		strictcli.WithEffect(strictcli.EffectMutating),
		// The rewrites go through the fix package's own apply-and-verify
		// path, not through the effects handle, so the framework has nothing
		// to record and a preview would show an empty plan while the real run
		// edits files. --preview is this command's honest preview.
		strictcli.WithDryRunUnsupported(
			"the file rewrites happen inside the fix package's apply-and-verify path, outside the effects handle; pass --preview to list the planned fixes without touching a file"),
		strictcli.WithArgs(
			strictcli.NewArg("dir", "Project or workspace root",
				strictcli.ArgRequired(false), strictcli.ArgDefault(".")),
		),
		// The apply-vs-preview choice is a required mutex: writing files is
		// never an implicit default.
		strictcli.WithMutex(strictcli.MutexGroup{Flags: []strictcli.Flag{
			strictcli.BoolFlag("apply", "Apply the planned fixes (files are rewritten, then verified; mismatches roll back)"),
			strictcli.BoolFlag("preview", "List the planned fixes without touching any file"),
		}}),
		strictcli.WithFlags(
			strictcli.StringFlag("config", "Config file name, resolved relative to the analyzed directory",
				strictcli.Default("strictcode.toml")),
		),
	)

	registry := app.Group("registry", "Rule registry artifacts (mint-once IDs, tombstones)")
	registry.Command("dump", "Write the committed registry dump (rules, groups, tombstones) as JSON",
		registryDumpHandler,
		// mutating: it writes the dump file named by --out.
		strictcli.WithEffect(strictcli.EffectMutating),
		strictcli.WithDryRunUnsupported(
			"the dump is written with a direct file write, outside the effects handle, so a preview would report nothing while the real run rewrites the file"),
		strictcli.WithFlags(
			strictcli.StringFlag("out", "Output path for the registry dump",
				strictcli.Default("REGISTRY.json")),
		),
	)

	matrix := app.Group("matrix", "Language x feature support matrix")
	matrix.Command("gen", "Write the generated support matrix (rules and capabilities per language)",
		matrixGenHandler,
		// mutating: it writes the matrix file named by --out.
		strictcli.WithEffect(strictcli.EffectMutating),
		strictcli.WithDryRunUnsupported(
			"the matrix is written with a direct file write, outside the effects handle, so a preview would report nothing while the real run rewrites the file"),
		strictcli.WithFlags(
			strictcli.StringFlag("out", "Output path for the matrix",
				strictcli.Default("docs/MATRIX.md")),
		),
	)

	return app
}

// analyzeHandler: exit 0 = clean or warnings only; exit 1 = at least one
// error-severity finding (the CI hard gate rlsbl keys on); exit 2 = tool or
// config error.
func analyzeHandler(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
	dir := strictcli.Get[string](kwargs, "dir")
	cfgName := strictcli.Get[string](kwargs, "config")

	res, err := engine.Analyze(dir, cfgName)
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(2)
	}
	// The document is built in both modes -- the framework decides what to do
	// with the payload -- and the text report is the human mode's rendering,
	// which is the one thing machine mode must not print: stdout carries the
	// envelope alone.
	doc, err := findings.Build(strictcode.Version, res.WorkspaceRoot, res.Findings)
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(2)
	}
	ctx.Payload(doc)
	if !ctx.JSON() {
		fmt.Print(findings.RenderText(res.Findings))
	}
	if findings.FailRun(res.Findings) {
		return strictcli.Exit(1)
	}
	return strictcli.Exit(0)
}

// fixHandler plans the whitelisted tier-1 transforms and either previews
// or applies+verifies them. Exit 0 = success (or nothing to fix); exit 2 =
// tool/config error, including a verification rollback.
func fixHandler(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
	dir := strictcli.Get[string](kwargs, "dir")
	cfgName := strictcli.Get[string](kwargs, "config")
	apply, _ := strictcli.GetOpt[bool](kwargs, "apply")

	ws, err := workspace.Load(dir)
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(2)
	}
	cfg, err := config.Load(filepath.Join(ws.Root, filepath.FromSlash(cfgName)))
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(2)
	}
	res, err := extract.Extract(ws)
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(2)
	}
	plans := fix.PlanUnreachableRemovals(res, cfg)
	if len(plans) == 0 {
		fmt.Println("no tier-1 fixes to apply")
		return strictcli.Exit(0)
	}
	for _, p := range plans {
		fmt.Printf("%s: %s [%s] (bytes %d..%d)\n", p.File, p.Description, p.Rule, p.Start, p.End)
	}
	if !apply {
		fmt.Printf("%d fix(es) planned (preview; pass --apply to write)\n", len(plans))
		return strictcli.Exit(0)
	}
	report, err := fix.Apply(ws, res, plans)
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(2)
	}
	fmt.Printf("applied %d fix(es) across %d file(s); post-fix graph verified\n", len(report.Applied), report.FilesEdited)
	return strictcli.Exit(0)
}

func registryDumpHandler(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
	out := strictcli.Get[string](kwargs, "out")
	data, err := registrydump.RegistryJSON()
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(1)
	}
	if err := writeFile(out, data); err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(1)
	}
	ctx.Info("wrote " + out)
	return strictcli.Exit(0)
}

func matrixGenHandler(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
	out := strictcli.Get[string](kwargs, "out")
	if err := writeFile(out, registrydump.MatrixMarkdown()); err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(1)
	}
	ctx.Info("wrote " + out)
	return strictcli.Exit(0)
}

func writeFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
