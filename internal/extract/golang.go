package extract

import (
	"path"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/testctx"
	"github.com/smm-h/strictcode/internal/treesitter"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

var goQueries = struct {
	imports  *treesitter.Query
	pkg      *treesitter.Query
	mainFunc *treesitter.Query
}{}

func initGoQueries() {
	if goQueries.imports != nil {
		return
	}
	mustCompile := func(pattern string) *treesitter.Query {
		q, err := treesitter.CompileQuery(treesitter.GrammarGo, pattern)
		if err != nil {
			panic(err)
		}
		return q
	}
	goQueries.imports = mustCompile(`(import_spec path: (interpreted_string_literal) @path)`)
	goQueries.pkg = mustCompile(`(package_clause (package_identifier) @name)`)
	goQueries.mainFunc = mustCompile(`(source_file (function_declaration name: (identifier) @fn (#eq? @fn "main")))`)
}

// goDepMatches matches a go.mod require path against a candidate member's
// module path.
func goDepMatches(dep string, cand *workspace.Member) bool {
	cmf := cand.Manifests[vocab.LangGo]
	return cmf != nil && cmf.GoModulePath != "" && dep == cmf.GoModulePath
}

// extractGo extracts a member's Go surface: package-granular modules
// (the import-addressable unit is the package directory), imports rows, and
// package-main entry points. Nested Go modules within the member (e.g.
// conformance harnesses with their own go.mod) contribute their requires as
// the member's declared deps, and their module paths scope intra-member
// package resolution for files beneath them.
func (ex *extraction) extractGo(m *workspace.Member) error {
	mf := m.Manifests[vocab.LangGo]
	all, err := walkMember(ex.ws, m, func(name string) bool {
		return strings.HasSuffix(name, ".go") || name == "go.mod"
	})
	if err != nil {
		return err
	}
	var files, nestedMods []string
	for _, f := range all {
		if strings.HasSuffix(f, "go.mod") {
			if f != "go.mod" { // the root manifest is loaded by workspace
				nestedMods = append(nestedMods, f)
			}
			continue
		}
		files = append(files, f)
	}
	if len(files) == 0 && mf == nil {
		return nil
	}
	initGoQueries()

	if _, err := ex.memberNodeID(vocab.LangGo, m); err != nil {
		return err
	}
	if err := ex.emitDeclaredDeps(vocab.LangGo, m, m.Manifests[vocab.LangGo], goDepMatches); err != nil {
		return err
	}

	// Nested modules: dir (member-relative) -> module path.
	nested := map[string]string{}
	for _, nm := range nestedMods {
		nmf, err := loadNestedGoMod(ex.ws, m, nm)
		if err != nil {
			return err
		}
		if err := ex.emitDeclaredDeps(vocab.LangGo, m, nmf, goDepMatches); err != nil {
			return err
		}
		nested[path.Dir(nm)] = nmf.GoModulePath
	}

	modPath := ""
	if mf != nil {
		modPath = mf.GoModulePath
	}
	// modPathFor returns the nearest enclosing module path for a package
	// dir, and the module-root dir it is anchored at ("" for the member
	// root module).
	modPathFor := func(pkgDir string) (string, string) {
		best, bestPath := "", modPath
		for dir, mp := range nested {
			if pkgDir == dir || strings.HasPrefix(pkgDir, dir+"/") {
				if len(dir) > len(best) {
					best, bestPath = dir, mp
				}
			}
		}
		return bestPath, best
	}

	// Package directories (member-relative; "." for the root package).
	pkgFiles := map[string][]string{}
	for _, f := range files {
		dir := path.Dir(f)
		pkgFiles[dir] = append(pkgFiles[dir], f)
	}
	pkgDirs := make([]string, 0, len(pkgFiles))
	for dir := range pkgFiles {
		pkgDirs = append(pkgDirs, dir)
	}
	sort.Strings(pkgDirs)

	// Module nodes: one per package directory. A package is test context
	// when its directory path is test context or every file is a _test.go
	// file (lesson 9: such packages never define dead-module candidates).
	for _, dir := range pkgDirs {
		allTest := true
		for _, f := range pkgFiles[dir] {
			if !strings.HasSuffix(f, "_test.go") {
				allTest = false
				break
			}
		}
		dirIsTest := dir != "." && testctx.IsTestContext(dir+"/")
		node := relation.Node{
			Kind: vocab.NodeKindModule,
			ID:   moduleNodeID(vocab.LangGo, m.Name, dir),
			Attrs: map[string]relation.Value{
				"logical_name": relation.StringValue(dir),
				"path":         relation.StringValue(wsRelPath(m, dir)),
				"test_context": relation.BoolValue(dirIsTest || allTest),
			},
		}
		if err := ex.builder.AddNode(node); err != nil {
			return err
		}
	}

	for _, dir := range pkgDirs {
		// resolveIntra maps an import path to a member-relative package dir
		// under the nearest enclosing module of this package's dir.
		mp, anchor := modPathFor(dir)
		resolveIntra := func(importPath string) (string, bool) {
			if mp == "" {
				return "", false
			}
			rel, ok := goRelPackage(mp, importPath)
			if !ok {
				return "", false
			}
			switch {
			case anchor == "":
				return rel, true
			case rel == ".":
				return anchor, true
			default:
				return anchor + "/" + rel, true
			}
		}
		for _, file := range pkgFiles[dir] {
			if err := ex.extractGoFile(m, resolveIntra, pkgFiles, dir, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ex *extraction) extractGoFile(m *workspace.Member, resolveIntra func(string) (string, bool), pkgFiles map[string][]string, pkgDir, file string) error {
	wsPath := wsRelPath(m, file)
	src, err := ex.readNormalized(wsPath)
	if err != nil {
		return err
	}
	tree, err := treesitter.Parse(treesitter.GrammarGo, src)
	if err != nil {
		return err
	}
	defer tree.Close()

	srcID := moduleNodeID(vocab.LangGo, m.Name, pkgDir)
	isTest := strings.HasSuffix(file, "_test.go") || testctx.IsTestContext(file)
	attrs := func() map[string]relation.Value {
		return map[string]relation.Value{
			"test_context":  relation.BoolValue(isTest),
			"guarded":       relation.BoolValue(false),
			"type_checking": relation.BoolValue(false),
		}
	}

	for _, match := range goQueries.imports.Matches(tree) {
		for _, cap := range match.Captures {
			lit := nodeText(&cap.Node, tree.Source)
			p := strings.Trim(lit, "`\"")
			span := spanOf(&cap.Node)
			resolvedToMember := false

			// Intra-member package resolution (nearest enclosing module).
			if rel, ok := resolveIntra(p); ok {
				if _, known := pkgFiles[rel]; known {
					row := relation.Row{
						Kind: vocab.RowKindImports,
						Src:  srcID,
						Dst:  moduleNodeID(vocab.LangGo, m.Name, rel),
						File: wsPath, Span: span, Attrs: attrs(),
					}
					if err := ex.builder.AddRow(row); err != nil {
						return err
					}
				}
			}
			// Member resolution: import path equals a member's module path
			// or is prefixed by it (DESIGN.md 6.3, Go).
			for _, other := range ex.ws.Members {
				omf := other.Manifests[vocab.LangGo]
				if omf == nil || omf.GoModulePath == "" {
					continue
				}
				if _, ok := goRelPackage(omf.GoModulePath, p); !ok {
					continue
				}
				dstID, err := ex.memberNodeID(vocab.LangGo, other)
				if err != nil {
					return err
				}
				row := relation.Row{
					Kind: vocab.RowKindImports,
					Src:  srcID,
					Dst:  dstID,
					File: wsPath, Span: span, Attrs: attrs(),
				}
				if err := ex.builder.AddRow(row); err != nil {
					return err
				}
				resolvedToMember = true
				break
			}
			if !resolvedToMember {
				ex.external = append(ex.external, ExternalImport{
					Lang:        vocab.LangGo,
					Member:      m.Name,
					SrcModule:   pkgDir,
					Specifier:   p,
					File:        wsPath,
					Span:        span,
					TestContext: isTest,
				})
			}
		}
	}

	// Entry point: package main with func main (not in test files).
	if !isTest && goPackageName(tree) == "main" && len(goQueries.mainFunc.Matches(tree)) > 0 {
		epID := entryPointNodeID(vocab.LangGo, m.Name, "main_package", pkgDir)
		node := relation.Node{
			Kind: vocab.NodeKindEntryPoint,
			ID:   epID,
			Attrs: map[string]relation.Value{
				"form":          relation.StringValue("main_package"),
				"declared_name": relation.StringValue(pkgDir),
			},
		}
		// The same package can declare main in several files; the node is
		// one. AddNode errors on the duplicate — tolerate exactly that case
		// by checking first.
		if !ex.hasEntryPoint(vocab.LangGo, m.Name, "main_package", pkgDir) {
			if err := ex.builder.AddNode(node); err != nil {
				return err
			}
			row := relation.Row{
				Kind: vocab.RowKindResolvesTo,
				Src:  epID,
				Dst:  moduleNodeID(vocab.LangGo, m.Name, pkgDir),
				File: wsPath,
				Span: relation.Span{},
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadNestedGoMod parses a nested go.mod within a member.
func loadNestedGoMod(ws *workspace.Workspace, m *workspace.Member, memberRel string) (*workspace.Manifest, error) {
	return workspace.ParseGoMod(ws.Root, wsRelPath(m, memberRel))
}

// goRelPackage returns the member-relative package dir for an import path
// under the module path: (modPath, modPath) -> ".", (modPath,
// modPath+"/internal/x") -> "internal/x".
func goRelPackage(modPath, importPath string) (string, bool) {
	if importPath == modPath {
		return ".", true
	}
	if strings.HasPrefix(importPath, modPath+"/") {
		return importPath[len(modPath)+1:], true
	}
	return "", false
}

func goPackageName(tree *treesitter.Tree) string {
	for _, match := range goQueries.pkg.Matches(tree) {
		for _, cap := range match.Captures {
			return nodeText(&cap.Node, tree.Source)
		}
	}
	return ""
}

// hasEntryPoint reports whether the entry point was already emitted, and
// marks it emitted otherwise (a Go package can declare main in several
// files; the node is one).
func (ex *extraction) hasEntryPoint(lang vocab.Lang, member, form, name string) bool {
	key := string(lang) + "\x00" + member + "\x00" + form + "\x00" + name
	if ex.entryPoints[key] {
		return true
	}
	ex.entryPoints[key] = true
	return false
}
