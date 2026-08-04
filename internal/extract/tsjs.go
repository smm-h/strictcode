package extract

import (
	"path"
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/testctx"
	"github.com/smm-h/strictcode/internal/treesitter"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// nodeBuiltins are the Node.js builtin module names (also importable with
// the node: prefix); they never resolve to workspace members.
var nodeBuiltins = map[string]bool{
	"assert": true, "async_hooks": true, "buffer": true, "child_process": true,
	"cluster": true, "console": true, "constants": true, "crypto": true,
	"dgram": true, "diagnostics_channel": true, "dns": true, "domain": true,
	"events": true, "fs": true, "http": true, "http2": true, "https": true,
	"inspector": true, "module": true, "net": true, "os": true, "path": true,
	"perf_hooks": true, "process": true, "punycode": true, "querystring": true,
	"readline": true, "repl": true, "stream": true, "string_decoder": true,
	"timers": true, "tls": true, "trace_events": true, "tty": true, "url": true,
	"util": true, "v8": true, "vm": true, "wasi": true, "worker_threads": true,
	"zlib": true, "test": true, "sea": true, "sqlite": true,
}

// tsExts are the probe extensions in probe order (DESIGN.md 6.2, lesson 18).
var tsExts = []string{".ts", ".tsx", ".js", ".mjs", ".cjs"}

var tsSourceExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts"}

var tsQueries = struct {
	imports *treesitter.Query
	tsx     *treesitter.Query
}{}

// tsImportQuery captures every specifier-carrying construct: import
// declarations, export-from declarations, require() calls, and dynamic
// import() calls.
const tsImportQuery = `[
  (import_statement source: (string) @import_src)
  (export_statement source: (string) @export_src)
  (call_expression function: (identifier) @fn arguments: (arguments (string) @require_src) (#eq? @fn "require"))
  (call_expression function: (import) arguments: (arguments (string) @dynamic_src))
]`

func initTSQueries() {
	if tsQueries.imports != nil {
		return
	}
	mustCompile := func(g treesitter.Grammar) *treesitter.Query {
		q, err := treesitter.CompileQuery(g, tsImportQuery)
		if err != nil {
			panic(err)
		}
		return q
	}
	tsQueries.imports = mustCompile(treesitter.GrammarTypeScript)
	tsQueries.tsx = mustCompile(treesitter.GrammarTSX)
}

// tsLayout is a member's TS/JS surface.
type tsLayout struct {
	// files: member-relative source files (Python-package-embedded files
	// excluded per lesson 17).
	files []string
	// modules: logical name -> member-relative file.
	modules map[string]string
	// fileSet for O(1) resolution probes.
	fileSet map[string]bool
}

// tsLogicalName strips the extension and collapses index files to their
// directory (SPEC.md 2.2). A root-level index file becomes ".".
func tsLogicalName(file string) string {
	ext := path.Ext(file)
	stem := strings.TrimSuffix(file, ext)
	if path.Base(stem) == "index" {
		dir := path.Dir(stem)
		return dir // "." for a root index
	}
	return stem
}

func (ex *extraction) extractTS(m *workspace.Member) error {
	mf := m.Manifests[vocab.LangTS]
	all, err := walkMember(ex.ws, m, func(name string) bool {
		if name == "__init__.py" {
			return true
		}
		for _, ext := range tsSourceExts {
			if strings.HasSuffix(name, ext) {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}

	// Lesson 17: files inside a Python package tree (a dir with __init__.py
	// between the file and the member root) are data resources, not modules.
	pyPkgDirs := map[string]bool{}
	var sources []string
	for _, f := range all {
		if strings.HasSuffix(f, "__init__.py") {
			pyPkgDirs[path.Dir(f)] = true
			continue
		}
		sources = append(sources, f)
	}
	layout := &tsLayout{modules: map[string]string{}, fileSet: map[string]bool{}}
	for _, f := range sources {
		inPyPkg := false
		for dir := path.Dir(f); dir != "."; dir = path.Dir(dir) {
			if pyPkgDirs[dir] {
				inPyPkg = true
				break
			}
		}
		if inPyPkg {
			continue
		}
		layout.files = append(layout.files, f)
		layout.fileSet[f] = true
	}
	if len(layout.files) == 0 && mf == nil {
		return nil
	}
	initTSQueries()

	if _, err := ex.memberNodeID(vocab.LangTS, m); err != nil {
		return err
	}
	if err := ex.emitDeclaredDeps(vocab.LangTS, m, m.Manifests[vocab.LangTS], func(dep string, cand *workspace.Member) bool {
		n := strings.ToLower(dep)
		if n == strings.ToLower(cand.Name) {
			return true
		}
		rn := cand.RegistryName(vocab.LangTS)
		return rn != "" && n == strings.ToLower(rn)
	}); err != nil {
		return err
	}

	for _, f := range layout.files {
		layout.modules[tsLogicalName(f)] = f
	}

	logicals := make([]string, 0, len(layout.modules))
	for logical := range layout.modules {
		logicals = append(logicals, logical)
	}
	sort.Strings(logicals)
	for _, logical := range logicals {
		file := layout.modules[logical]
		node := relation.Node{
			Kind: vocab.NodeKindModule,
			ID:   moduleNodeID(vocab.LangTS, m.Name, logical),
			Attrs: map[string]relation.Value{
				"logical_name": relation.StringValue(logical),
				"path":         relation.StringValue(wsRelPath(m, file)),
				"test_context": relation.BoolValue(testctx.IsTestContext(file)),
			},
		}
		if err := ex.builder.AddNode(node); err != nil {
			return err
		}
	}

	for _, file := range layout.files {
		if err := ex.extractTSFile(m, layout, file); err != nil {
			return err
		}
	}

	return ex.emitTSEntryPoints(m, layout)
}

func (ex *extraction) extractTSFile(m *workspace.Member, layout *tsLayout, file string) error {
	wsPath := wsRelPath(m, file)
	src, err := ex.readNormalized(wsPath)
	if err != nil {
		return err
	}
	grammar, ok := treesitter.GrammarForFile(file)
	if !ok {
		return nil
	}
	tree, err := treesitter.Parse(grammar, src)
	if err != nil {
		return err
	}
	defer tree.Close()

	query := tsQueries.imports
	if grammar == treesitter.GrammarTSX {
		query = tsQueries.tsx
	}

	srcID := moduleNodeID(vocab.LangTS, m.Name, tsLogicalName(file))
	isTest := testctx.IsTestContext(file)
	fromDir := path.Dir(file)

	for _, match := range query.Matches(tree) {
		var specNode *sitter.Node
		var capName string
		for i := range match.Captures {
			if match.Captures[i].Name != "fn" {
				specNode = &match.Captures[i].Node
				capName = match.Captures[i].Name
			}
		}
		if specNode == nil {
			continue
		}
		spec := tsStringContent(specNode, tree.Source)
		if spec == "" {
			continue
		}
		span := spanOf(specNode)
		stmt := statementAncestor(specNode)
		typeOnly := capName == "import_src" && tsIsTypeOnly(stmt)
		attrs := func() map[string]relation.Value {
			return map[string]relation.Value{
				"test_context":  relation.BoolValue(isTest),
				"guarded":       relation.BoolValue(false),
				"type_checking": relation.BoolValue(typeOnly),
			}
		}

		if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/") {
			// Relative: resolve to a member module (lesson 18).
			target, ok := resolveTSRelative(layout, fromDir, spec)
			if !ok {
				continue
			}
			dstID := moduleNodeID(vocab.LangTS, m.Name, tsLogicalName(target))
			row := relation.Row{
				Kind: vocab.RowKindImports,
				Src:  srcID, Dst: dstID,
				File: wsPath, Span: span, Attrs: attrs(),
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
			if capName == "export_src" {
				reexport := relation.Row{
					Kind: vocab.RowKindExports,
					Src:  srcID, Dst: dstID,
					File: wsPath, Span: span,
					Attrs: map[string]relation.Value{"form": relation.StringValue("reexport")},
				}
				if err := ex.builder.AddRow(reexport); err != nil {
					return err
				}
			}
			continue
		}

		// Bare specifier: reduce to the package name and match members
		// case-insensitively (DESIGN.md 6.3 TS/JS).
		resolvedToMember := false
		if pkg, ok := tsBarePackage(spec); ok {
			for _, other := range ex.ws.Members {
				if !tsMemberMatches(pkg, other) {
					continue
				}
				dstID, err := ex.memberNodeID(vocab.LangTS, other)
				if err != nil {
					return err
				}
				row := relation.Row{
					Kind: vocab.RowKindImports,
					Src:  srcID, Dst: dstID,
					File: wsPath, Span: span, Attrs: attrs(),
				}
				if err := ex.builder.AddRow(row); err != nil {
					return err
				}
				resolvedToMember = true
				break
			}
		}
		if !resolvedToMember {
			ex.external = append(ex.external, ExternalImport{
				Lang:        vocab.LangTS,
				Member:      m.Name,
				SrcModule:   srcID.Module,
				Specifier:   spec,
				File:        wsPath,
				Span:        span,
				TestContext: isTest,
			})
		}
	}
	return nil
}

func tsMemberMatches(pkg string, cand *workspace.Member) bool {
	n := strings.ToLower(pkg)
	if n == strings.ToLower(cand.Name) {
		return true
	}
	rn := cand.RegistryName(vocab.LangTS)
	return rn != "" && n == strings.ToLower(rn)
}

// tsBarePackage reduces a bare specifier to its package name: relative and
// builtin specifiers are dropped; @scope/pkg/... -> @scope/pkg; pkg/sub ->
// pkg.
func tsBarePackage(spec string) (string, bool) {
	if strings.HasPrefix(spec, "node:") {
		return "", false
	}
	if strings.HasPrefix(spec, "@") {
		parts := strings.SplitN(spec, "/", 3)
		if len(parts) < 2 {
			return "", false
		}
		return parts[0] + "/" + parts[1], true
	}
	pkg, _, _ := strings.Cut(spec, "/")
	if pkg == "" || nodeBuiltins[pkg] {
		return "", false
	}
	return pkg, true
}

// resolveTSRelative resolves a relative specifier from fromDir against the
// member's files: exact path, .js->.ts / .jsx->.tsx mapping, extension
// probing, and directory index resolution (lesson 18).
func resolveTSRelative(layout *tsLayout, fromDir, spec string) (string, bool) {
	target := path.Clean(path.Join(fromDir, spec))
	target = strings.TrimPrefix(target, "/")
	if target == "" || strings.HasPrefix(target, "..") {
		return "", false
	}

	if layout.fileSet[target] {
		return target, true
	}
	// .js -> .ts, .jsx -> .tsx (TS emits .js specifiers for .ts sources).
	if strings.HasSuffix(target, ".js") {
		if mapped := strings.TrimSuffix(target, ".js") + ".ts"; layout.fileSet[mapped] {
			return mapped, true
		}
	}
	if strings.HasSuffix(target, ".jsx") {
		if mapped := strings.TrimSuffix(target, ".jsx") + ".tsx"; layout.fileSet[mapped] {
			return mapped, true
		}
	}
	// Extension probing.
	for _, ext := range tsExts {
		if layout.fileSet[target+ext] {
			return target + ext, true
		}
	}
	// Directory -> index.*.
	for _, ext := range tsExts {
		if layout.fileSet[target+"/index"+ext] {
			return target + "/index" + ext, true
		}
	}
	return "", false
}

// emitTSEntryPoints adds entry_point nodes from package.json (exports
// recursive, main, bin — lesson 19) with resolves_to rows into the module
// set.
func (ex *extraction) emitTSEntryPoints(m *workspace.Member, layout *tsLayout) error {
	mf := m.Manifests[vocab.LangTS]
	if mf == nil {
		return nil
	}
	for _, ep := range mf.EntryPoints {
		epID := entryPointNodeID(vocab.LangTS, m.Name, ep.Form, ep.Name)
		if ex.hasEntryPoint(vocab.LangTS, m.Name, ep.Form, ep.Name) {
			continue
		}
		node := relation.Node{
			Kind: vocab.NodeKindEntryPoint,
			ID:   epID,
			Attrs: map[string]relation.Value{
				"form":          relation.StringValue(ep.Form),
				"declared_name": relation.StringValue(ep.Name),
			},
		}
		if err := ex.builder.AddNode(node); err != nil {
			return err
		}
		target, ok := resolveTSRelative(layout, ".", "./"+strings.TrimPrefix(ep.Target, "./"))
		if !ok {
			continue
		}
		row := relation.Row{
			Kind: vocab.RowKindResolvesTo,
			Src:  epID,
			Dst:  moduleNodeID(vocab.LangTS, m.Name, tsLogicalName(target)),
			File: mf.Path,
			Span: ex.locate(mf.Path, ep.Target),
		}
		if err := ex.builder.AddRow(row); err != nil {
			return err
		}
	}
	return nil
}

// --- node helpers ---------------------------------------------------------

// tsStringContent extracts the content of a string literal node.
func tsStringContent(n *sitter.Node, src []byte) string {
	count := n.NamedChildCount()
	for i := uint(0); i < count; i++ {
		c := n.NamedChild(i)
		if c.Kind() == "string_fragment" {
			return nodeText(c, src)
		}
	}
	return ""
}

// statementAncestor ascends from a captured string to its statement node.
func statementAncestor(n *sitter.Node) *sitter.Node {
	for cur := n; cur != nil; cur = cur.Parent() {
		switch cur.Kind() {
		case "import_statement", "export_statement", "expression_statement",
			"lexical_declaration", "variable_declaration":
			return cur
		}
	}
	return n
}

// tsIsTypeOnly reports whether an import statement is a type-only import
// (`import type ... from "..."`).
func tsIsTypeOnly(stmt *sitter.Node) bool {
	if stmt == nil || stmt.Kind() != "import_statement" {
		return false
	}
	count := stmt.ChildCount()
	for i := uint(0); i < count; i++ {
		if stmt.Child(i).Kind() == "type" {
			return true
		}
	}
	return false
}
