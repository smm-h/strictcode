// Command strictcode is the strictcode CLI, built on strictcli (flag
// conventions enforced at registration; --dump-schema auto-injected).
//
// Foundation-build surface: the registry dump and the support matrix — the
// two committed artifacts CI diffs. Analysis commands land with the
// extractors round.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	strictcode "github.com/smm-h/strictcode"
	"github.com/smm-h/strictcode/internal/engine"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/registrydump"
	"github.com/smm-h/strictcli/go/strictcli"
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
		strictcli.WithArgs(
			strictcli.NewArg("dir", "Project or workspace root to analyze",
				strictcli.ArgRequired(false), strictcli.ArgDefault(".")),
		),
		strictcli.WithFlags(
			strictcli.StringFlag("format", "Output format: text or json",
				strictcli.Default("text"), strictcli.Choices("text", "json")),
			strictcli.StringFlag("config", "Config file name, resolved relative to the analyzed directory",
				strictcli.Default("strictcode.toml")),
		),
	)

	registry := app.Group("registry", "Rule registry artifacts (mint-once IDs, tombstones)")
	registry.Command("dump", "Write the committed registry dump (rules, groups, tombstones) as JSON",
		registryDumpHandler,
		strictcli.WithFlags(
			strictcli.StringFlag("out", "Output path for the registry dump",
				strictcli.Default("REGISTRY.json")),
		),
	)

	matrix := app.Group("matrix", "Language x feature support matrix")
	matrix.Command("gen", "Write the generated support matrix (rules and capabilities per language)",
		matrixGenHandler,
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
	format := strictcli.Get[string](kwargs, "format")
	cfgName := strictcli.Get[string](kwargs, "config")

	res, err := engine.Analyze(dir, cfgName)
	if err != nil {
		ctx.Error(err.Error())
		return strictcli.Exit(2)
	}
	switch format {
	case "json":
		out, err := findings.RenderJSON(strictcode.Version, res.WorkspaceRoot, res.Findings)
		if err != nil {
			ctx.Error(err.Error())
			return strictcli.Exit(2)
		}
		fmt.Print(string(out))
	default:
		fmt.Print(findings.RenderText(res.Findings))
	}
	if findings.FailRun(res.Findings) {
		return strictcli.Exit(1)
	}
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
