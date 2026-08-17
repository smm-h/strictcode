// Command strictcode is the strictcode CLI, built on strictcli (flag
// conventions enforced at registration; --dump-schema auto-injected).
//
// Surface: analyze (the batch pipeline — extract, check, report, exit
// code), fix (the tier-1 transforms, under a required apply/preview
// selector), registry dump, and matrix gen (the two committed artifacts
// CI diffs).
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

// Defaults the mutating commands apply in their handlers rather than in their
// declarations. On a mutating command strictcli refuses a value default --
// absence must never resolve to a value the invocation did not state -- so each
// of these is declared Optional() and substituted here, and every one of them
// names a DESTINATION or a SEARCH ROOT, never a value that gets written into
// anything. Each flag's help states the fallback.
const (
	defaultConfigName  = "strictcode.toml"
	defaultRoot        = "."
	defaultRegistryOut = "REGISTRY.json"
	defaultMatrixOut   = "docs/MATRIX.md"
)

// The `fix` command's write decision is a member-spelled selector: the operator
// still types `--apply` or `--preview` exactly as before, and the selector -- not
// a hand-written guard -- is what makes exactly one of them mandatory. "Neither
// elected" is unrepresentable rather than a state the handler has to refuse, and
// `--no-apply` can no longer be read as "preview".
var (
	fixApplyChoice = strictcli.MemberChoice(
		strictcli.BoolFlag("apply",
			"Apply the planned fixes (files are rewritten, then verified; mismatches roll back)",
			strictcli.Required()),
		"rewrite the files and verify the result against the declared delta")
	fixPreviewChoice = strictcli.MemberChoice(
		strictcli.BoolFlag("preview",
			"List the planned fixes without touching any file",
			strictcli.Required()),
		"list the planned fixes and touch nothing")
)

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
				strictcli.ArgDefault(defaultRoot)),
		),
		strictcli.WithFlags(
			strictcli.StringFlag("config", "Config file name, resolved relative to the analyzed directory",
				strictcli.Default(defaultConfigName)),
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
		// No update_of: `fix` names no properties. The edits are computed by
		// the tier-1 planner from the extracted graph, not carried by flags, so
		// there is no property set to declare and no sparse-vs-full-replace
		// question to answer (contract §27 requires at least one property).
		strictcli.WithArgs(
			// Optional, not defaulted: this command is mutating, so the
			// fallback is applied in the handler and stated here.
			strictcli.NewArg("dir", "Project or workspace root; omitted means the current directory",
				strictcli.ArgOptional()),
		),
		strictcli.WithFlags(
			strictcli.StringFlag("config",
				"Config file name, resolved relative to the analyzed directory; omitted means "+defaultConfigName,
				strictcli.Optional()),
			// The apply-vs-preview choice is a required member-spelled
			// selector: writing files is never an implicit default, and
			// exactly one member is elected by construction.
			strictcli.MemberChoiceFlag("disposition", "What to do with the planned fixes",
				strictcli.Required(), fixApplyChoice, fixPreviewChoice),
		),
	)

	registry := app.Group("registry", "Rule registry artifacts (mint-once IDs, tombstones)")
	registry.Command("dump", "Write the committed registry dump (rules, groups, tombstones) as JSON",
		registryDumpHandler,
		// mutating: it writes the dump file named by --out.
		strictcli.WithEffect(strictcli.EffectMutating),
		strictcli.WithDryRunUnsupported(
			"the dump is written with a direct file write, outside the effects handle, so a preview would report nothing while the real run rewrites the file"),
		// No update_of: the dump is regenerated whole from the built-in
		// registry, and --out names where it lands, not what it contains.
		strictcli.WithFlags(
			// Optional, not defaulted: mutating commands may not declare a
			// value default, and --out is a destination the handler falls back
			// on, never a value written into the artifact.
			strictcli.StringFlag("out", "Output path for the registry dump; omitted means "+defaultRegistryOut,
				strictcli.Optional()),
		),
	)

	matrix := app.Group("matrix", "Language x feature support matrix")
	matrix.Command("gen", "Write the generated support matrix (rules and capabilities per language)",
		matrixGenHandler,
		// mutating: it writes the matrix file named by --out.
		strictcli.WithEffect(strictcli.EffectMutating),
		strictcli.WithDryRunUnsupported(
			"the matrix is written with a direct file write, outside the effects handle, so a preview would report nothing while the real run rewrites the file"),
		// No update_of: the matrix is regenerated whole from the registry and
		// the capability profiles; --out names where it lands.
		strictcli.WithFlags(
			strictcli.StringFlag("out", "Output path for the matrix; omitted means "+defaultMatrixOut,
				strictcli.Optional()),
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
	dir := optOr(kwargs, "dir", defaultRoot)
	cfgName := optOr(kwargs, "config", defaultConfigName)
	// The selector is required and elects exactly one member, so "neither
	// elected" is unrepresentable rather than a state this handler refuses.
	apply := strictcli.GetElected(kwargs, "disposition").Is(fixApplyChoice)

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
	out := optOr(kwargs, "out", defaultRegistryOut)
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
	out := optOr(kwargs, "out", defaultMatrixOut)
	if err := writeFile(out, registrydump.MatrixMarkdown()); err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(1)
	}
	ctx.Info("wrote " + out)
	return strictcli.Exit(0)
}

// optOr reads an optional string flag or arg, substituting the handler-side
// fallback when the invocation did not state one. This is the third remedy the
// mutating-default ban names, and it is legal here because every value that
// travels through it is a path -- where a command reads or writes -- never a
// value the command writes into an artifact.
func optOr(kwargs map[string]interface{}, name, fallback string) string {
	if v, ok := strictcli.GetOpt[string](kwargs, name); ok {
		return v
	}
	return fallback
}

func writeFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
