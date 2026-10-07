package extract

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/testctx"
	"github.com/smm-h/strictcode/internal/treesitter"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// --- resolution index (built across all members; see check-semantics.md) ---

// pyNorm applies PyPI name normalization: lowercase with -, _, and .
// unified (lesson 10: registry name and workspace name both match through
// normalization).
func pyNorm(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s
}

// pyDepMatches reports whether a pyproject dependency names the candidate
// member: its member name or its registry name, under PyPI normalization.
func pyDepMatches(dep string, cand *workspace.Member) bool {
	n := pyNorm(dep)
	return n == pyNorm(cand.Name) || (cand.RegistryName(vocab.LangPy) != "" && n == pyNorm(cand.RegistryName(vocab.LangPy)))
}

// pyMemberLayout is one member's discovered Python surface.
type pyMemberLayout struct {
	member *workspace.Member
	// files: member-relative paths of every walked .py file.
	files []string
	// packageDirs: member-relative dirs that contain __init__.py.
	packageDirs map[string]bool
	// roots: dotted prefix of each top-level package root -> true
	// (e.g. "core", "orxt.transport"). Used for the namespace map and for
	// logical-name derivation.
	roots map[string]bool
	// srcBase is true when roots live under src/.
	srcBase bool
	// rootPkg is non-empty when the member root itself is the package
	// (member path points at a directory containing __init__.py — found on
	// the real corpus); it is the package's importable name.
	rootPkg string
	// modules: logical name -> member-relative path.
	modules map[string]string
}

// pyResolutionIndex resolves absolute imports to workspace members.
type pyResolutionIndex struct {
	// byNorm: normalized workspace/registry name -> member.
	byNorm map[string]*workspace.Member
	// byImportName: exact import_name override -> member (step 3).
	byImportName map[string]*workspace.Member
	// nsEntries: dotted package-root prefixes -> member, longest first
	// (step 4).
	nsEntries []nsEntry
	// layouts: member name -> layout.
	layouts map[string]*pyMemberLayout
}

type nsEntry struct {
	prefix string
	member *workspace.Member
}

func buildPyResolutionIndex(list *sourceList, ws *workspace.Workspace) (*pyResolutionIndex, error) {
	idx := &pyResolutionIndex{
		byNorm:       map[string]*workspace.Member{},
		byImportName: map[string]*workspace.Member{},
		layouts:      map[string]*pyMemberLayout{},
	}
	for _, m := range ws.Members {
		layout, err := discoverPyLayout(list, ws, m)
		if err != nil {
			return nil, err
		}
		idx.layouts[m.Name] = layout

		if _, taken := idx.byNorm[pyNorm(m.Name)]; !taken {
			idx.byNorm[pyNorm(m.Name)] = m
		}
		if rn := m.RegistryName(vocab.LangPy); rn != "" {
			if _, taken := idx.byNorm[pyNorm(rn)]; !taken {
				idx.byNorm[pyNorm(rn)] = m
			}
		}
		if m.ImportName != "" {
			idx.byImportName[m.ImportName] = m
		}
		for root := range layout.roots {
			idx.nsEntries = append(idx.nsEntries, nsEntry{prefix: root, member: m})
		}
	}
	sort.Slice(idx.nsEntries, func(i, j int) bool {
		a, b := idx.nsEntries[i], idx.nsEntries[j]
		if la, lb := strings.Count(a.prefix, ".")+1, strings.Count(b.prefix, ".")+1; la != lb {
			return la > lb // longest prefix first
		}
		if a.prefix != b.prefix {
			return a.prefix < b.prefix
		}
		return a.member.Name < b.member.Name
	})
	return idx, nil
}

// resolveMember applies the import resolution order in stricttools/docs/check-semantics.md to an absolute
// dotted import. Returns nil for stdlib and external imports.
func (idx *pyResolutionIndex) resolveMember(dotted string) *workspace.Member {
	parts := strings.Split(dotted, ".")
	top := parts[0]
	// Step 1: stdlib never resolves to a member.
	if pyStdlib[top] {
		return nil
	}
	// Step 2: normalized top-level match against member/registry names.
	if m, ok := idx.byNorm[pyNorm(top)]; ok {
		return m
	}
	// Step 3: explicit import_name override.
	if m, ok := idx.byImportName[top]; ok {
		return m
	}
	// Step 4: namespace-map longest-prefix match.
	for _, e := range idx.nsEntries {
		if dotted == e.prefix || strings.HasPrefix(dotted, e.prefix+".") {
			return e.member
		}
	}
	// Step 5: any dotted sub-component normalizing to a member name.
	for _, p := range parts[1:] {
		if m, ok := idx.byNorm[pyNorm(p)]; ok {
			return m
		}
	}
	return nil
}

// discoverPyLayout walks a member's .py files and derives package roots and
// module logical names (stricttools/docs/node-identity.md, module segment: dotted path from the discovered
// package root; the same discovery feeds the namespace map).
func discoverPyLayout(list *sourceList, ws *workspace.Workspace, m *workspace.Member) (*pyMemberLayout, error) {
	files, err := walkMember(list, ws, m, func(name string) bool { return strings.HasSuffix(name, ".py") })
	if err != nil {
		return nil, err
	}
	layout := &pyMemberLayout{
		member:      m,
		files:       files,
		packageDirs: map[string]bool{},
		roots:       map[string]bool{},
		modules:     map[string]string{},
	}
	for _, f := range files {
		if strings.HasSuffix(f, "/__init__.py") {
			layout.packageDirs[strings.TrimSuffix(f, "/__init__.py")] = true
		}
		if f == "__init__.py" {
			// The member root IS the package: its importable name is the
			// directory's base name.
			layout.rootPkg = filepath.Base(filepath.Join(ws.Root, filepath.FromSlash(m.Path)))
			layout.roots[layout.rootPkg] = true
		}
	}

	// Top-level package roots under the member root or src/: a package dir
	// whose parent is the base, or one namespace level deep (ns/pkg where ns
	// itself is not a package).
	for dir := range layout.packageDirs {
		base, rest := "", dir
		if strings.HasPrefix(dir, "src/") {
			base, rest = "src", dir[len("src/"):]
		}
		parts := strings.Split(rest, "/")
		switch len(parts) {
		case 1:
			layout.roots[rest] = true
			if base == "src" {
				layout.srcBase = true
			}
		case 2:
			nsDir := parts[0]
			if base == "src" {
				nsDir = "src/" + nsDir
			}
			if !layout.packageDirs[nsDir] {
				layout.roots[parts[0]+"."+parts[1]] = true
				if base == "src" {
					layout.srcBase = true
				}
			}
		}
	}

	// Module logical names.
	for _, f := range files {
		layout.modules[layout.logicalName(f)] = f
	}
	return layout, nil
}

// logicalName derives a module's logical name from its member-relative
// path: for files under a discovered package root, the dotted path from the
// base (src/ stripped); a package's __init__.py is the package itself.
// Files outside any package root keep their full dotted relative path
// (scripts/build.py -> scripts.build).
func (l *pyMemberLayout) logicalName(file string) string {
	// Member-root-as-package: anchor at the directory's importable name.
	if l.rootPkg != "" {
		if file == "__init__.py" {
			return l.rootPkg
		}
		dotted := strings.ReplaceAll(strings.TrimSuffix(file, ".py"), "/", ".")
		dotted = strings.TrimSuffix(dotted, ".__init__")
		return l.rootPkg + "." + dotted
	}
	rest := file
	if strings.HasPrefix(file, "src/") {
		rest = file[len("src/"):]
	}
	dotted := strings.ReplaceAll(strings.TrimSuffix(rest, ".py"), "/", ".")
	dotted = strings.TrimSuffix(dotted, ".__init__")
	if l.underRoot(dotted) {
		return dotted
	}
	// Not under a package root: full member-relative dotted path.
	full := strings.ReplaceAll(strings.TrimSuffix(file, ".py"), "/", ".")
	return strings.TrimSuffix(full, ".__init__")
}

func (l *pyMemberLayout) underRoot(dotted string) bool {
	for root := range l.roots {
		if dotted == root || strings.HasPrefix(dotted, root+".") {
			return true
		}
	}
	return false
}

// --- tree-sitter queries --------------------------------------------------

var pyQueries = struct {
	imports *treesitter.Query
	all     *treesitter.Query
}{}

func initPyQueries() {
	if pyQueries.imports != nil {
		return
	}
	mustCompile := func(pattern string) *treesitter.Query {
		q, err := treesitter.CompileQuery(treesitter.GrammarPython, pattern)
		if err != nil {
			panic(err)
		}
		return q
	}
	pyQueries.imports = mustCompile(`[(import_statement) (import_from_statement)] @imp`)
	pyQueries.all = mustCompile(`(assignment left: (identifier) @lhs right: (list) @list (#eq? @lhs "__all__"))`)
}

// --- extraction -----------------------------------------------------------

// pyImport is one extracted import site.
type pyImport struct {
	span     relation.Span
	guarded  bool
	typeOnly bool
	// relative marks a relative import: its candidates are pre-resolved to
	// absolute names from the file's package position for intra-member
	// module resolution, but relative imports never participate in member
	// resolution (step 1 of the import resolution order drops them).
	relative bool
	// dotted are the absolute dotted-name candidates this site references,
	// most specific first (e.g. from pkg import mod -> [pkg.mod, pkg]).
	dotted []string
}

func (ex *extraction) extractPython(m *workspace.Member) error {
	layout := ex.pyIndex.layouts[m.Name]
	if len(layout.files) == 0 && m.Manifests[vocab.LangPy] == nil {
		return nil
	}
	initPyQueries()

	if _, err := ex.memberNodeID(vocab.LangPy, m); err != nil {
		return err
	}
	if err := ex.emitDeclaredDeps(vocab.LangPy, m, m.Manifests[vocab.LangPy], pyDepMatches); err != nil {
		return err
	}

	// Module nodes first (import rows may target any of them).
	logicals := make([]string, 0, len(layout.modules))
	for logical := range layout.modules {
		logicals = append(logicals, logical)
	}
	sort.Strings(logicals)
	for _, logical := range logicals {
		file := layout.modules[logical]
		node := relation.Node{
			Kind: vocab.NodeKindModule,
			ID:   moduleNodeID(vocab.LangPy, m.Name, logical),
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

	// Parse each file and emit rows.
	for _, file := range layout.files {
		if err := ex.extractPyFile(m, layout, file); err != nil {
			return err
		}
	}

	// Entry points from the manifest.
	return ex.emitPyEntryPoints(m, layout)
}

func (ex *extraction) extractPyFile(m *workspace.Member, layout *pyMemberLayout, file string) error {
	wsPath := wsRelPath(m, file)
	src, err := ex.readNormalized(wsPath)
	if err != nil {
		return err
	}
	tree, err := treesitter.Parse(treesitter.GrammarPython, src)
	if err != nil {
		return err
	}

	logical := layout.logicalName(file)
	srcID := moduleNodeID(vocab.LangPy, m.Name, logical)
	isTest := testctx.IsTestContext(file)

	for _, match := range pyQueries.imports.Matches(tree) {
		for _, cap := range match.Captures {
			imp := analyzePyImport(cap.Node, logical, layout)
			if err := ex.emitPyImportRows(m, layout, srcID, wsPath, isTest, imp); err != nil {
				return err
			}
		}
	}

	// Export rows from __init__.py (lesson 16: a module exported by any
	// __init__.py via __all__ or a relative import must not be flagged dead).
	if strings.HasSuffix(file, "__init__.py") {
		if err := ex.emitPyExports(m, layout, srcID, wsPath, logical, tree); err != nil {
			return err
		}
	}

	// Full-semantic pass 1 (same parse): callables, types, containment,
	// and the site records pass 2 resolves after every member is walked.
	sem, err := ex.extractPySemantics(m, layout, file, wsPath, &pyTree{
		root: tree.Root(), isTest: isTest,
	})
	if err != nil {
		return err
	}
	if ex.pySem.mods[m.Name] == nil {
		ex.pySem.mods[m.Name] = map[string]*pyModSem{}
	}
	ex.pySem.mods[m.Name][logical] = sem
	return nil
}

// emitPyImportRows emits the module->module row for the most specific
// resolving candidate and the module->member row per the resolution order.
func (ex *extraction) emitPyImportRows(m *workspace.Member, layout *pyMemberLayout, srcID relation.NodeID, wsPath string, isTest bool, imp pyImport) error {
	attrs := func() map[string]relation.Value {
		return map[string]relation.Value{
			"test_context":  relation.BoolValue(isTest),
			"guarded":       relation.BoolValue(imp.guarded),
			"type_checking": relation.BoolValue(imp.typeOnly),
		}
	}
	// Intra-member module resolution: most specific candidate that is one
	// of this member's modules.
	for _, dotted := range imp.dotted {
		if _, ok := layout.modules[dotted]; ok {
			row := relation.Row{
				Kind: vocab.RowKindImports,
				Src:  srcID,
				Dst:  moduleNodeID(vocab.LangPy, m.Name, dotted),
				File: wsPath, Span: imp.span, Attrs: attrs(),
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
			break
		}
	}
	// Member resolution (workspace-internal deps): first candidate that
	// resolves. Self-resolution is emitted too; checks exempt it. Relative
	// imports are dropped from member resolution (6.3 step 1).
	if imp.relative {
		return nil
	}
	resolvedToMember := false
	for _, dotted := range imp.dotted {
		if target := ex.pyIndex.resolveMember(dotted); target != nil {
			resolvedToMember = true
			dstID, err := ex.memberNodeID(vocab.LangPy, target)
			if err != nil {
				return err
			}
			row := relation.Row{
				Kind: vocab.RowKindImports,
				Src:  srcID,
				Dst:  dstID,
				File: wsPath, Span: imp.span, Attrs: attrs(),
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
			break
		}
	}
	// External import: record the site for specifier-based checks
	// (library-forbidden-imports). The most specific candidate is the raw
	// dotted name as written.
	if !resolvedToMember && len(imp.dotted) > 0 {
		ex.external = append(ex.external, ExternalImport{
			Lang:        vocab.LangPy,
			Member:      m.Name,
			SrcModule:   srcID.Module,
			Specifier:   imp.dotted[0],
			File:        wsPath,
			Span:        imp.span,
			TestContext: isTest,
		})
	}
	return nil
}

// emitPyExports emits exports rows for __all__ entries and
// from-dot-import names that match sibling submodules.
func (ex *extraction) emitPyExports(m *workspace.Member, layout *pyMemberLayout, srcID relation.NodeID, wsPath, pkgLogical string, tree *treesitter.Tree) error {
	emit := func(name, form string, span relation.Span) error {
		submodule := pkgLogical + "." + name
		if _, ok := layout.modules[submodule]; !ok {
			return nil
		}
		row := relation.Row{
			Kind: vocab.RowKindExports,
			Src:  srcID,
			Dst:  moduleNodeID(vocab.LangPy, m.Name, submodule),
			File: wsPath, Span: span,
			Attrs: map[string]relation.Value{"form": relation.StringValue(form)},
		}
		return ex.builder.AddRow(row)
	}

	for _, match := range pyQueries.all.Matches(tree) {
		for _, cap := range match.Captures {
			if cap.Name != "list" {
				continue
			}
			n := cap.Node
			count := n.NamedChildCount()
			for i := uint32(0); i < count; i++ {
				item := namedChildAt(n, i)
				if item.Type() != "string" {
					continue
				}
				name := pyStringContent(item)
				if name == "" {
					continue
				}
				if err := emit(name, "declared_all", spanOf(item)); err != nil {
					return err
				}
			}
		}
	}

	// "from . import name" exports name.
	for _, match := range pyQueries.imports.Matches(tree) {
		for _, cap := range match.Captures {
			n := cap.Node
			if n.Type() != "import_from_statement" {
				continue
			}
			mod, ok := n.ChildByFieldName("module_name")
			if !ok || mod.Type() != "relative_import" || mod.Text() != "." {
				continue
			}
			for _, nameNode := range fieldChildren(n, "name") {
				name := importedName(nameNode)
				if name == "" {
					continue
				}
				if err := emit(name, "relative_import", spanOf(nameNode)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// emitPyEntryPoints adds entry_point nodes for [project.scripts] /
// [project.gui-scripts] and resolves_to rows to the target module.
func (ex *extraction) emitPyEntryPoints(m *workspace.Member, layout *pyMemberLayout) error {
	mf := m.Manifests[vocab.LangPy]
	if mf == nil {
		return nil
	}
	for _, ep := range mf.EntryPoints {
		epID := entryPointNodeID(vocab.LangPy, m.Name, ep.Form, ep.Name)
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
		target, _, _ := strings.Cut(ep.Target, ":")
		if _, ok := layout.modules[target]; !ok {
			continue
		}
		row := relation.Row{
			Kind: vocab.RowKindResolvesTo,
			Src:  epID,
			Dst:  moduleNodeID(vocab.LangPy, m.Name, target),
			File: mf.Path,
			Span: ex.locate(mf.Path, ep.Name),
		}
		if err := ex.builder.AddRow(row); err != nil {
			return err
		}
	}
	return nil
}

// --- import-site analysis -------------------------------------------------

// analyzePyImport derives the dotted-name candidates and the
// guarded/type_checking classification for one import statement.
func analyzePyImport(n treesitter.Node, moduleLogical string, layout *pyMemberLayout) pyImport {
	imp := pyImport{span: spanOf(n)}
	imp.guarded = pyIsGuarded(n)
	imp.typeOnly = pyIsTypeChecking(n)

	switch n.Type() {
	case "import_statement":
		for _, nameNode := range fieldChildren(n, "name") {
			dotted := importedName(nameNode)
			if dotted != "" {
				imp.dotted = append(imp.dotted, splitPrefixes(dotted)...)
			}
		}
	case "import_from_statement":
		mod, ok := n.ChildByFieldName("module_name")
		if !ok {
			break
		}
		var base string
		if mod.Type() == "relative_import" {
			imp.relative = true
			base = resolveRelativeBase(mod.Text(), moduleLogical, layout)
			if base == "" {
				break // beyond the package root: no resolvable candidates
			}
		} else {
			base = mod.Text()
		}
		// Candidates: base.name for each imported name (most specific), then
		// the base itself and its prefixes.
		var specific []string
		for _, nameNode := range fieldChildren(n, "name") {
			if name := importedName(nameNode); name != "" {
				specific = append(specific, base+"."+name)
			}
		}
		imp.dotted = append(imp.dotted, specific...)
		imp.dotted = append(imp.dotted, splitPrefixes(base)...)
	}
	return imp
}

// splitPrefixes returns dotted and its ancestor prefixes, most specific
// first: a.b.c -> [a.b.c, a.b, a].
func splitPrefixes(dotted string) []string {
	var out []string
	for {
		out = append(out, dotted)
		i := strings.LastIndexByte(dotted, '.')
		if i < 0 {
			return out
		}
		dotted = dotted[:i]
	}
}

// resolveRelativeBase converts a relative-import prefix (".", "..",
// ".sub") into an absolute dotted base from the importing module's package
// position. Returns "" when the relative level escapes the package root.
func resolveRelativeBase(relText, moduleLogical string, layout *pyMemberLayout) string {
	dots := 0
	for dots < len(relText) && relText[dots] == '.' {
		dots++
	}
	suffix := relText[dots:]

	// The importing module's package: for pkg.sub.mod the package is
	// pkg.sub; for a package's own __init__ (logical pkg.sub) it is itself.
	pkg := moduleLogical
	if _, isPkg := layout.modules[pkg]; !isPkg || !layout.packageIsDir(pkg) {
		if i := strings.LastIndexByte(pkg, '.'); i >= 0 {
			pkg = pkg[:i]
		} else {
			return ""
		}
	}
	// Each dot beyond the first ascends one package level.
	for i := 1; i < dots; i++ {
		j := strings.LastIndexByte(pkg, '.')
		if j < 0 {
			return ""
		}
		pkg = pkg[:j]
	}
	if suffix != "" {
		return pkg + "." + suffix
	}
	return pkg
}

// packageIsDir reports whether a logical name denotes a package (its file
// is an __init__.py).
func (l *pyMemberLayout) packageIsDir(logical string) bool {
	file, ok := l.modules[logical]
	return ok && strings.HasSuffix(file, "__init__.py")
}

// pyIsGuarded reports whether the import sits in the TRY BODY of a
// try/except catching ImportError or ModuleNotFoundError (lessons 1-3: the
// except body — a fallback import — is NOT guarded).
func pyIsGuarded(n treesitter.Node) bool {
	for child, parent, ok := n, n, false; ; child = parent {
		if parent, ok = child.Parent(); !ok {
			return false
		}
		if parent.Type() != "try_statement" {
			continue
		}
		body, ok := parent.ChildByFieldName("body")
		if !ok || child != body {
			continue // inside an except/else/finally clause, keep ascending
		}
		count := parent.ChildCount()
		for i := uint32(0); i < count; i++ {
			clause := childAt(parent, i)
			if clause.Type() == "except_clause" && exceptCatchesImportError(clause) {
				return true
			}
		}
	}
}

// exceptCatchesImportError matches `except ImportError`, tuple forms, and
// as-bound forms; the matched identifiers may be dotted (builtins.ImportError).
func exceptCatchesImportError(clause treesitter.Node) bool {
	count := clause.NamedChildCount()
	for i := uint32(0); i < count; i++ {
		c := namedChildAt(clause, i)
		if c.Type() == "block" {
			continue
		}
		if exprNamesImportError(c) {
			return true
		}
	}
	return false
}

func exprNamesImportError(n treesitter.Node) bool {
	switch n.Type() {
	case "identifier":
		t := n.Text()
		return t == "ImportError" || t == "ModuleNotFoundError"
	case "attribute":
		attr, ok := n.ChildByFieldName("attribute")
		return ok && exprNamesImportError(attr)
	case "tuple", "parenthesized_expression", "as_pattern", "expression_list":
		count := n.NamedChildCount()
		for i := uint32(0); i < count; i++ {
			if exprNamesImportError(namedChildAt(n, i)) {
				return true
			}
		}
	}
	return false
}

// pyIsTypeChecking reports whether the import sits in the consequence of an
// `if TYPE_CHECKING:` (bare or typing-qualified) block (lesson 5).
func pyIsTypeChecking(n treesitter.Node) bool {
	for child, parent, ok := n, n, false; ; child = parent {
		if parent, ok = child.Parent(); !ok {
			return false
		}
		if parent.Type() != "if_statement" {
			continue
		}
		consequence, ok := parent.ChildByFieldName("consequence")
		if !ok || child != consequence {
			continue
		}
		if cond, ok := parent.ChildByFieldName("condition"); ok && condIsTypeChecking(cond) {
			return true
		}
	}
}

func condIsTypeChecking(n treesitter.Node) bool {
	switch n.Type() {
	case "identifier":
		return n.Text() == "TYPE_CHECKING"
	case "attribute":
		attr, ok := n.ChildByFieldName("attribute")
		return ok && attr.Text() == "TYPE_CHECKING"
	case "parenthesized_expression":
		if n.NamedChildCount() == 1 {
			return condIsTypeChecking(namedChildAt(n, 0))
		}
	}
	return false
}

// --- node helpers ---------------------------------------------------------

func spanOf(n treesitter.Node) relation.Span {
	return relation.Span{Start: n.StartByte(), End: n.EndByte()}
}

// fieldChildren returns every child of n assigned to the given field.
func fieldChildren(n treesitter.Node, field string) []treesitter.Node {
	var out []treesitter.Node
	count := n.ChildCount()
	for i := uint32(0); i < count; i++ {
		if name, ok := n.FieldNameForChild(i); ok && name == field {
			out = append(out, childAt(n, i))
		}
	}
	return out
}

// importedName extracts the dotted name from a dotted_name or
// aliased_import node.
func importedName(n treesitter.Node) string {
	switch n.Type() {
	case "dotted_name", "identifier":
		return n.Text()
	case "aliased_import":
		if name, ok := n.ChildByFieldName("name"); ok {
			return name.Text()
		}
	}
	return ""
}

// pyStringContent returns the content of a plain string literal node.
func pyStringContent(n treesitter.Node) string {
	count := n.NamedChildCount()
	for i := uint32(0); i < count; i++ {
		c := namedChildAt(n, i)
		if c.Type() == "string_content" {
			return c.Text()
		}
	}
	return ""
}

// childAt returns n's i-th child; i must be below n.ChildCount().
func childAt(n treesitter.Node, i uint32) treesitter.Node {
	c, _ := n.Child(i)
	return c
}

// namedChildAt returns n's i-th named child; i must be below
// n.NamedChildCount().
func namedChildAt(n treesitter.Node, i uint32) treesitter.Node {
	c, _ := n.NamedChild(i)
	return c
}
