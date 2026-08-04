package checks

import (
	"fmt"
	"strings"

	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
)

// defaultForbidden holds the per-language application-concern lists
// (DESIGN.md 6.5), replaceable via config.
var defaultForbidden = map[vocab.Lang][]string{
	vocab.LangPy: {"argparse", "click", "typer", "flask", "fastapi", "django",
		"uvicorn", "granian", "starlette", "tornado", "bottle"},
	vocab.LangGo: {"net/http", "github.com/spf13/cobra", "github.com/urfave/cli"},
	vocab.LangTS: {"express", "koa", "hono", "commander", "yargs"},
}

// checkLibraryForbiddenImports: a library importing an application-concern
// module. Runs only on library = true members (lesson 22); test/example
// files are excluded via the shared predicate (lesson 23); the workspace
// lint_allow list and the per-language config allow list are both
// subtracted (lesson 26). Matching: Python by top-level module, Go and
// TS/JS by full specifier.
func checkLibraryForbiddenImports(ctx *Context) []findings.Finding {
	setting := ctx.Cfg.Setting("library-forbidden-imports")
	var out []findings.Finding
	for _, m := range ctx.View.WS.Members {
		if !m.Library {
			continue
		}
		for _, lang := range vocab.Langs {
			forbidden := map[string]bool{}
			list := defaultForbidden[lang]
			if replacement, ok := setting.Forbidden[string(lang)]; ok {
				list = replacement
			}
			for _, f := range list {
				forbidden[f] = true
			}
			for _, allowed := range m.LintAllow {
				delete(forbidden, allowed)
			}
			for _, allowed := range setting.Allow[string(lang)] {
				delete(forbidden, allowed)
			}
			if len(forbidden) == 0 {
				continue
			}
			for _, ext := range ctx.View.Res.ExternalImports {
				if ext.Lang != lang || ext.Member != m.Name || ext.TestContext {
					continue
				}
				specifier := ext.Specifier
				if lang == vocab.LangPy {
					specifier, _, _ = strings.Cut(specifier, ".")
				}
				if !forbidden[specifier] {
					continue
				}
				srcID := relation.NodeID{Lang: string(lang), Member: m.Name, Module: ext.SrcModule}
				out = append(out, ctx.finding("library-forbidden-imports",
					srcID.String(), vocab.NodeKindModule,
					ext.File, ext.Span.Start,
					fmt.Sprintf("library '%s' imports application-concern module '%s'", m.Name, specifier)))
			}
		}
	}
	return out
}

// cliEntryForms are the entry-point forms that constitute a CLI surface.
// npm export/main entries are the normal library surface and never count.
var cliEntryForms = map[string]bool{
	"script":       true,
	"gui_script":   true,
	"bin":          true,
	"main_package": true,
}

// checkLibraryEntryPoint: a library declaring a CLI entry point
// ([project.scripts], func main in package main, npm bin). Library-only
// (lesson 22).
func checkLibraryEntryPoint(ctx *Context) []findings.Finding {
	var out []findings.Finding
	for _, m := range ctx.View.WS.Members {
		if !m.Library {
			continue
		}
		for _, lang := range vocab.Langs {
			for _, ep := range ctx.View.EntryPoints[langMember{lang, m.Name}] {
				if !cliEntryForms[ep.Form] {
					continue
				}
				file := ep.File
				if file == "" {
					file = m.Path
				}
				out = append(out, ctx.finding("library-entry-point",
					ep.ID.String(), vocab.NodeKindEntryPoint,
					file, ep.Span.Start,
					fmt.Sprintf("library '%s' declares CLI entry point '%s' (%s)", m.Name, ep.Name, ep.Form)))
			}
		}
	}
	return out
}
