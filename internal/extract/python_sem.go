package extract

// Python full-semantic extraction (round 3): callables, types, containment,
// decorates, declared conformance, instantiation, and syntactic call
// resolution via a two-pass symbol table (DESIGN.md section 8 layer 1).
//
// Pass 1 (per file, same parse as the import extractor): emit
// function/closure/type nodes with SPEC 2.3-2.5 identity (anonymous
// hint~ordinal~fp8, overload #n, receiver-free Python chains), contains
// rows, and record bindings, call sites, decorator sites, base references,
// and unreachable regions.
//
// Pass 2 (after every member's pass 1): resolve call/decorator/base sites
// against the global symbol index. The relation receives calls rows only
// for resolved local-to-local calls (both endpoints must exist as nodes);
// external and unresolved call sites are honest side data (Result.Calls),
// never guessed rows.

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictcode/internal/workspace"
)

// CallResolution classifies a call site in the side table.
type CallResolution string

const (
	// CallSyntactic: resolved to a workspace-local callable or type; the
	// relation carries the corresponding calls/instantiates row.
	CallSyntactic CallResolution = "syntactic"
	// CallExternal: the callee is a builtin, stdlib, or external-package
	// name (library-stdout matches these).
	CallExternal CallResolution = "external"
	// CallUnresolved: dynamic dispatch, local rebinding, star-import
	// ambiguity — honestly unknown, never guessed.
	CallUnresolved CallResolution = "unresolved"
)

// CallSite is one call site (side data beside the relation).
type CallSite struct {
	Lang   vocab.Lang
	Member string
	// Module is the containing module's logical name.
	Module string
	// Container is the serialized ID of the innermost enclosing container
	// node (module, type, function, or closure).
	Container string
	// Callee is the alias-expanded dotted callee text ("print",
	// "sys.stdout.write", "pkg.mod.f"); empty when the callee expression is
	// not a dotted name.
	Callee     string
	Resolution CallResolution
	// Target is the serialized resolved node ID (syntactic only).
	Target      string
	File        string
	Span        relation.Span
	TestContext bool
}

// UnreachableRegion is one dead statement region (side data; the
// unreachable-code rule and the tier-1 removal transform consume it).
type UnreachableRegion struct {
	Lang   vocab.Lang
	Member string
	Module string
	// Container is the serialized ID of the node owning the block.
	Container string
	File      string
	// Span covers the first through last unreachable statement.
	Span relation.Span
	// FirstLineSpan is the first unreachable statement (the finding site).
	FirstLineSpan relation.Span
	TestContext   bool
}

// --- pass-1 per-module record ---------------------------------------------

type pyBindKind int

const (
	bindModule pyBindKind = iota // name -> absolute dotted module
	bindName                     // name -> (source module, original name)
)

type pyBinding struct {
	kind   pyBindKind
	module string // bindModule: absolute dotted target
	source string // bindName: absolute dotted source module
	orig   string // bindName: original name
}

type pyClassInfo struct {
	// chain is the container chain of the class node.
	chain []relation.Segment
	// methods: name -> number of same-name defs (last one wins at runtime).
	methods map[string]int
	// bases: dotted base expressions in source order.
	bases []pyBaseRef
}

type pyBaseRef struct {
	expr string
	span relation.Span
}

type pyCallRec struct {
	container  []relation.Segment // enclosing container chain (nil = module level)
	inCallable bool               // container innermost is function/closure
	callee     string             // raw dotted callee text ("" = complex)
	firstArg   string             // first positional argument when a bare identifier
	span       relation.Span
}

type pyDecoRec struct {
	expr        string
	targetChain []relation.Segment
	span        relation.Span
}

type pyModSem struct {
	member  *workspace.Member
	logical string
	wsPath  string
	isTest  bool

	bindings map[string]pyBinding
	star     bool // a wildcard import is present
	// classes: dotted chain key ("A", "Outer.Inner") -> info.
	classes map[string]*pyClassInfo
	// funcs: dotted chain key -> count of same-name defs.
	funcs map[string]int
	// callableLocals: chain key of a callable -> set of locally bound
	// names (params, assignments) that shadow outer bindings.
	callableLocals map[string]map[string]bool
	// nestedDefs: chain key of a callable -> nested def names -> count.
	nestedDefs map[string]map[string]int

	calls       []pyCallRec
	decos       []pyDecoRec
	unreachable []UnreachableRegion
}

// pySemIndex is the workspace-global symbol index for pass 2.
type pySemIndex struct {
	// modules: (member, logical) -> sem record.
	mods map[string]map[string]*pyModSem
}

func (ix *pySemIndex) get(member, logical string) *pyModSem {
	return ix.mods[member][logical]
}

// --- pass 1 ---------------------------------------------------------------

var testNameRe = regexp.MustCompile(`^test_`)

// extractPySemantics walks one parsed file and returns its semantic record.
// Nodes and contains rows are emitted immediately; sites wait for pass 2.
func (ex *extraction) extractPySemantics(m *workspace.Member, layout *pyMemberLayout, file, wsPath string, tree *pyTree) (*pyModSem, error) {
	sem := &pyModSem{
		member:         m,
		logical:        layout.logicalName(file),
		wsPath:         wsPath,
		isTest:         tree.isTest,
		bindings:       map[string]pyBinding{},
		classes:        map[string]*pyClassInfo{},
		funcs:          map[string]int{},
		callableLocals: map[string]map[string]bool{},
		nestedDefs:     map[string]map[string]int{},
	}
	w := &pySemWalker{
		ex: ex, m: m, layout: layout, sem: sem, src: tree.src,
		nameCounters: map[string]int{},
		anonCounters: map[string]int{},
	}
	root := tree.root
	if err := w.walkBlock(root, nil, inModule, nil); err != nil {
		return nil, err
	}
	// Module-level unreachable analysis plus every nested block was handled
	// during the walk.
	return sem, nil
}

// pyTree bundles the parse results extractPyFile already has.
type pyTree struct {
	src    []byte
	root   *sitter.Node
	isTest bool
}

// containerKind tracks what the current walk container is.
type containerKind int

const (
	inModule containerKind = iota
	inClass
	inCallable
)

type pySemWalker struct {
	ex     *extraction
	m      *workspace.Member
	layout *pyMemberLayout
	sem    *pyModSem
	src    []byte

	// nameCounters: containerKey + "\x00" + name -> next overload index.
	nameCounters map[string]int
	// anonCounters: containerKey + "\x00" + hint -> next ordinal.
	anonCounters map[string]int
}

func chainKey(chain []relation.Segment) string {
	parts := make([]string, len(chain))
	for i, s := range chain {
		parts[i] = s.Name
		if s.Anonymous {
			parts[i] = fmt.Sprintf("%s~%d~%s", s.Name, s.Ordinal, s.Fingerprint)
		}
		if s.Overload > 0 {
			parts[i] += fmt.Sprintf("#%d", s.Overload)
		}
	}
	return strings.Join(parts, ".")
}

func (w *pySemWalker) nodeID(chain []relation.Segment) relation.NodeID {
	return relation.NodeID{
		Lang:   "py",
		Member: w.m.Name,
		Module: w.sem.logical,
		Chain:  chain,
	}
}

// containerNodeID returns the ID of the innermost container usable as a
// contains-row src (module, type, function — never closure, per the
// vocabulary's src kinds).
func (w *pySemWalker) containsSrc(chain []relation.Segment) relation.NodeID {
	for i := len(chain); i > 0; i-- {
		if !chain[i-1].Anonymous {
			return w.nodeID(chain[:i])
		}
	}
	return w.nodeID(nil)
}

// walkBlock walks the statements of a block (or the module root) with the
// given container chain. localScope is the innermost callable's local-name
// set (nil at module/class level).
func (w *pySemWalker) walkBlock(block *sitter.Node, chain []relation.Segment, kind containerKind, localScope map[string]bool) error {
	count := block.ChildCount()
	for i := uint(0); i < count; i++ {
		child := block.Child(i)
		if err := w.walkStatement(child, chain, kind, localScope); err != nil {
			return err
		}
	}
	w.analyzeUnreachable(block, chain)
	return nil
}

func (w *pySemWalker) walkStatement(n *sitter.Node, chain []relation.Segment, kind containerKind, localScope map[string]bool) error {
	switch n.Kind() {
	case "comment":
		return nil
	case "function_definition":
		return w.walkFunctionDef(n, chain, kind, nil)
	case "class_definition":
		return w.walkClassDef(n, chain, kind, nil)
	case "decorated_definition":
		var decos []*sitter.Node
		var def *sitter.Node
		cc := n.ChildCount()
		for i := uint(0); i < cc; i++ {
			c := n.Child(i)
			switch c.Kind() {
			case "decorator":
				decos = append(decos, c)
			case "function_definition":
				def = c
			case "class_definition":
				def = c
			}
		}
		if def == nil {
			return nil
		}
		if def.Kind() == "function_definition" {
			return w.walkFunctionDef(def, chain, kind, decos)
		}
		return w.walkClassDef(def, chain, kind, decos)
	case "import_statement", "import_from_statement":
		w.recordImportBindings(n)
		return nil
	default:
		// Generic walk: record calls, lambdas, and local assignments; then
		// recurse into nested blocks with the same container.
		return w.walkExprTree(n, chain, kind, localScope)
	}
}

// walkExprTree recursively visits a non-definition statement/expression,
// recording call sites and lambdas, and descending into nested blocks.
func (w *pySemWalker) walkExprTree(n *sitter.Node, chain []relation.Segment, kind containerKind, localScope map[string]bool) error {
	switch n.Kind() {
	case "comment":
		return nil
	case "function_definition":
		return w.walkFunctionDef(n, chain, kind, nil)
	case "class_definition":
		return w.walkClassDef(n, chain, kind, nil)
	case "decorated_definition":
		return w.walkStatement(n, chain, kind, localScope)
	case "lambda":
		return w.walkLambda(n, chain, kind, localScope)
	case "call":
		w.recordCall(n, chain, kind)
		// Recurse into arguments (nested calls/lambdas).
	case "assignment", "augmented_assignment":
		if localScope != nil {
			if left := n.ChildByFieldName("left"); left != nil && left.Kind() == "identifier" {
				localScope[nodeText(left, w.src)] = true
			}
		}
	}
	count := n.ChildCount()
	for i := uint(0); i < count; i++ {
		if err := w.walkExprTree(n.Child(i), chain, kind, localScope); err != nil {
			return err
		}
	}
	return nil
}

func (w *pySemWalker) walkFunctionDef(n *sitter.Node, chain []relation.Segment, parentKind containerKind, decos []*sitter.Node) error {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nodeText(nameNode, w.src)
	ck := chainKey(chain)
	overload := w.nameCounters[ck+"\x00"+name]
	w.nameCounters[ck+"\x00"+name]++

	seg := relation.Segment{Name: name, Overload: overload}
	funcChain := append(append([]relation.Segment{}, chain...), seg)

	isMethod := parentKind == inClass
	isAsync := false
	cc := n.ChildCount()
	for i := uint(0); i < cc; i++ {
		if n.Child(i).Kind() == "async" {
			isAsync = true
		}
	}
	node := relation.Node{
		Kind: vocab.NodeKindFunction,
		ID:   w.nodeID(funcChain),
		Attrs: map[string]relation.Value{
			"visibility": relation.StringValue(pyVisibility(name)),
			"is_method":  relation.BoolValue(isMethod),
			"is_async":   relation.BoolValue(isAsync),
			"is_test":    relation.BoolValue(w.sem.isTest || testNameRe.MatchString(name)),
		},
	}
	if err := w.ex.builder.AddNode(node); err != nil {
		return err
	}
	if err := w.ex.builder.AddRow(relation.Row{
		Kind: vocab.RowKindContains,
		Src:  w.containsSrc(chain),
		Dst:  w.nodeID(funcChain),
		File: w.sem.wsPath,
		Span: spanOf(n),
	}); err != nil {
		return err
	}

	// Registries for resolution.
	fk := chainKey(funcChain)
	w.sem.funcs[chainKey(append(append([]relation.Segment{}, chain...), relation.Segment{Name: name}))] = overload + 1
	if isMethod {
		w.sem.classes[ck].methods[name] = overload + 1
	}
	if len(chain) > 0 {
		parentKey := ck
		if w.sem.nestedDefs[parentKey] == nil {
			w.sem.nestedDefs[parentKey] = map[string]int{}
		}
		w.sem.nestedDefs[parentKey][name] = overload + 1
	}

	// Decorators.
	for _, d := range decos {
		if expr := decoratorExpr(d, w.src); expr != "" {
			w.sem.decos = append(w.sem.decos, pyDecoRec{expr: expr, targetChain: funcChain, span: spanOf(d)})
		}
	}

	// Local scope: parameters + assignments.
	locals := map[string]bool{}
	if params := n.ChildByFieldName("parameters"); params != nil {
		collectParamNames(params, w.src, locals)
	}
	w.sem.callableLocals[fk] = locals

	if body := n.ChildByFieldName("body"); body != nil {
		return w.walkBlock(body, funcChain, inCallable, locals)
	}
	return nil
}

func (w *pySemWalker) walkClassDef(n *sitter.Node, chain []relation.Segment, parentKind containerKind, decos []*sitter.Node) error {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nodeText(nameNode, w.src)
	ck := chainKey(chain)
	overload := w.nameCounters[ck+"\x00"+name]
	w.nameCounters[ck+"\x00"+name]++

	seg := relation.Segment{Name: name, Overload: overload}
	classChain := append(append([]relation.Segment{}, chain...), seg)

	info := &pyClassInfo{chain: classChain, methods: map[string]int{}}
	var baseExprs []pyBaseRef
	form := "class"
	if sup := n.ChildByFieldName("superclasses"); sup != nil {
		sc := sup.NamedChildCount()
		for i := uint(0); i < sc; i++ {
			b := sup.NamedChild(i)
			expr := dottedExpr(b, w.src)
			if expr == "" {
				continue
			}
			baseExprs = append(baseExprs, pyBaseRef{expr: expr, span: spanOf(b)})
			switch classForm(expr) {
			case "protocol":
				form = "protocol"
			case "enum":
				if form == "class" {
					form = "enum"
				}
			}
		}
	}
	info.bases = baseExprs

	node := relation.Node{
		Kind: vocab.NodeKindType,
		ID:   w.nodeID(classChain),
		Attrs: map[string]relation.Value{
			"form":       relation.StringValue(form),
			"visibility": relation.StringValue(pyVisibility(name)),
		},
	}
	if err := w.ex.builder.AddNode(node); err != nil {
		return err
	}
	if err := w.ex.builder.AddRow(relation.Row{
		Kind: vocab.RowKindContains,
		Src:  w.containsSrc(chain),
		Dst:  w.nodeID(classChain),
		File: w.sem.wsPath,
		Span: spanOf(n),
	}); err != nil {
		return err
	}

	// Register under the plain-name key (resolution targets the last
	// same-name definition; overload count tracked in funcs).
	plainKey := chainKey(append(append([]relation.Segment{}, chain...), relation.Segment{Name: name}))
	w.sem.classes[plainKey] = info
	w.sem.funcs[plainKey] = overload + 1

	for _, d := range decos {
		if expr := decoratorExpr(d, w.src); expr != "" {
			w.sem.decos = append(w.sem.decos, pyDecoRec{expr: expr, targetChain: classChain, span: spanOf(d)})
		}
	}

	if body := n.ChildByFieldName("body"); body != nil {
		return w.walkBlock(body, classChain, inClass, nil)
	}
	return nil
}

func (w *pySemWalker) walkLambda(n *sitter.Node, chain []relation.Segment, kind containerKind, localScope map[string]bool) error {
	hint := lambdaHint(n, w.src)
	ck := chainKey(chain)
	ordinal := w.anonCounters[ck+"\x00"+hint]
	w.anonCounters[ck+"\x00"+hint]++

	paramsText := ""
	if params := n.ChildByFieldName("parameters"); params != nil {
		paramsText = nodeText(params, w.src)
	}
	fp := signatureFingerprint(paramsText)

	seg := relation.Segment{Name: hint, Anonymous: true, Ordinal: ordinal, Fingerprint: fp}
	lamChain := append(append([]relation.Segment{}, chain...), seg)

	node := relation.Node{
		Kind: vocab.NodeKindClosure,
		ID:   w.nodeID(lamChain),
		Attrs: map[string]relation.Value{
			"name_hint":   relation.StringValue(hint),
			"ordinal":     relation.IntValue(int64(ordinal)),
			"fingerprint": relation.StringValue(fp),
		},
	}
	if err := w.ex.builder.AddNode(node); err != nil {
		return err
	}
	if err := w.ex.builder.AddRow(relation.Row{
		Kind: vocab.RowKindContains,
		Src:  w.containsSrc(chain),
		Dst:  w.nodeID(lamChain),
		File: w.sem.wsPath,
		Span: spanOf(n),
	}); err != nil {
		return err
	}

	locals := map[string]bool{}
	if params := n.ChildByFieldName("parameters"); params != nil {
		collectParamNames(params, w.src, locals)
	}
	if body := n.ChildByFieldName("body"); body != nil {
		return w.walkExprTree(body, lamChain, inCallable, locals)
	}
	return nil
}

// recordCall stores a call site for pass-2 resolution.
func (w *pySemWalker) recordCall(n *sitter.Node, chain []relation.Segment, kind containerKind) {
	fn := n.ChildByFieldName("function")
	if fn == nil {
		return
	}
	callee := dottedExpr(fn, w.src)
	firstArg := ""
	if args := n.ChildByFieldName("arguments"); args != nil && args.NamedChildCount() > 0 {
		if a := args.NamedChild(0); a.Kind() == "identifier" {
			firstArg = nodeText(a, w.src)
		}
	}
	w.sem.calls = append(w.sem.calls, pyCallRec{
		container:  append([]relation.Segment{}, chain...),
		inCallable: kind == inCallable,
		callee:     callee,
		firstArg:   firstArg,
		span:       spanOf(n),
	})
}

// recordImportBindings derives module-level name bindings from an import
// statement (module aliases and from-import names).
func (w *pySemWalker) recordImportBindings(n *sitter.Node) {
	switch n.Kind() {
	case "import_statement":
		for _, nameNode := range fieldChildren(n, "name") {
			switch nameNode.Kind() {
			case "dotted_name":
				dotted := nodeText(nameNode, w.src)
				head, _, _ := strings.Cut(dotted, ".")
				w.sem.bindings[head] = pyBinding{kind: bindModule, module: head}
			case "aliased_import":
				name := nameNode.ChildByFieldName("name")
				alias := nameNode.ChildByFieldName("alias")
				if name != nil && alias != nil {
					w.sem.bindings[nodeText(alias, w.src)] = pyBinding{kind: bindModule, module: nodeText(name, w.src)}
				}
			}
		}
	case "import_from_statement":
		mod := n.ChildByFieldName("module_name")
		if mod == nil {
			return
		}
		var base string
		if mod.Kind() == "relative_import" {
			base = resolveRelativeBase(nodeText(mod, w.src), w.sem.logical, w.layout)
			if base == "" {
				return
			}
		} else {
			base = nodeText(mod, w.src)
		}
		cc := n.ChildCount()
		for i := uint(0); i < cc; i++ {
			if n.Child(i).Kind() == "wildcard_import" {
				w.sem.star = true
				return
			}
		}
		for _, nameNode := range fieldChildren(n, "name") {
			switch nameNode.Kind() {
			case "dotted_name", "identifier":
				name := nodeText(nameNode, w.src)
				if !strings.Contains(name, ".") {
					w.sem.bindings[name] = pyBinding{kind: bindName, source: base, orig: name}
				}
			case "aliased_import":
				name := nameNode.ChildByFieldName("name")
				alias := nameNode.ChildByFieldName("alias")
				if name != nil && alias != nil {
					w.sem.bindings[nodeText(alias, w.src)] = pyBinding{kind: bindName, source: base, orig: nodeText(name, w.src)}
				}
			}
		}
	}
}

// --- unreachable-statement analysis (lessons 24-25) -----------------------

// analyzeUnreachable finds statements after an unconditional terminator in
// one block. Comment nodes are not statements (lesson 24); nested scopes
// were walked independently (lesson 25) — this looks at THIS block only.
func (w *pySemWalker) analyzeUnreachable(block *sitter.Node, chain []relation.Segment) {
	terminated := false
	var region []*sitter.Node
	count := block.ChildCount()
	for i := uint(0); i < count; i++ {
		stmt := block.Child(i)
		if !stmt.IsNamed() || stmt.Kind() == "comment" {
			continue
		}
		if terminated {
			region = append(region, stmt)
			continue
		}
		if alwaysTerminates(stmt) {
			terminated = true
		}
	}
	if len(region) == 0 {
		return
	}
	first, last := region[0], region[len(region)-1]
	w.sem.unreachable = append(w.sem.unreachable, UnreachableRegion{
		Lang:          vocab.LangPy,
		Member:        w.m.Name,
		Module:        w.sem.logical,
		Container:     w.nodeID(chain).String(),
		File:          w.sem.wsPath,
		Span:          relation.Span{Start: uint32(first.StartByte()), End: uint32(last.EndByte())},
		FirstLineSpan: spanOf(first),
		TestContext:   w.sem.isTest,
	})
}

// alwaysTerminates implements the unconditional-terminator predicate
// (DESIGN.md 6.5): return/raise/break/continue; an if/elif/else where every
// branch (including a present else) terminates; a block whose last
// statement terminates.
func alwaysTerminates(stmt *sitter.Node) bool {
	switch stmt.Kind() {
	case "return_statement", "raise_statement", "break_statement", "continue_statement":
		return true
	case "if_statement":
		cons := stmt.ChildByFieldName("consequence")
		if cons == nil || !blockTerminates(cons) {
			return false
		}
		hasElse := false
		cc := stmt.ChildCount()
		for i := uint(0); i < cc; i++ {
			c := stmt.Child(i)
			switch c.Kind() {
			case "elif_clause":
				ec := c.ChildByFieldName("consequence")
				if ec == nil || !blockTerminates(ec) {
					return false
				}
			case "else_clause":
				hasElse = true
				eb := c.ChildByFieldName("body")
				if eb == nil || !blockTerminates(eb) {
					return false
				}
			}
		}
		return hasElse
	}
	return false
}

// blockTerminates: the last non-comment statement always terminates.
func blockTerminates(block *sitter.Node) bool {
	var last *sitter.Node
	count := block.ChildCount()
	for i := uint(0); i < count; i++ {
		c := block.Child(i)
		if c.IsNamed() && c.Kind() != "comment" {
			last = c
		}
	}
	return last != nil && alwaysTerminates(last)
}

// --- small helpers --------------------------------------------------------

func pyVisibility(name string) string {
	if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return "public" // dunders are protocol surface
	}
	if strings.HasPrefix(name, "_") {
		return "private_convention"
	}
	return "public"
}

// classForm classifies a base expression: typing.Protocol -> protocol,
// enum.Enum family -> enum.
func classForm(baseExpr string) string {
	leaf := baseExpr
	if i := strings.LastIndexByte(baseExpr, '.'); i >= 0 {
		leaf = baseExpr[i+1:]
	}
	switch leaf {
	case "Protocol":
		return "protocol"
	case "Enum", "IntEnum", "StrEnum", "Flag", "IntFlag":
		return "enum"
	}
	return "class"
}

// dottedExpr renders an identifier/attribute chain as a dotted string;
// non-dotted expressions yield "".
func dottedExpr(n *sitter.Node, src []byte) string {
	switch n.Kind() {
	case "identifier":
		return nodeText(n, src)
	case "attribute":
		obj := n.ChildByFieldName("object")
		attr := n.ChildByFieldName("attribute")
		if obj == nil || attr == nil {
			return ""
		}
		base := dottedExpr(obj, src)
		if base == "" {
			return ""
		}
		return base + "." + nodeText(attr, src)
	case "subscript":
		// Generic bases like Protocol[T] classify by their value part.
		if v := n.ChildByFieldName("value"); v != nil {
			return dottedExpr(v, src)
		}
	}
	return ""
}

// decoratorExpr extracts the dotted expression of a decorator (through a
// call: @app.route("/x") -> app.route).
func decoratorExpr(d *sitter.Node, src []byte) string {
	count := d.NamedChildCount()
	for i := uint(0); i < count; i++ {
		c := d.NamedChild(i)
		switch c.Kind() {
		case "identifier", "attribute":
			return dottedExpr(c, src)
		case "call":
			if fn := c.ChildByFieldName("function"); fn != nil {
				return dottedExpr(fn, src)
			}
		}
	}
	return ""
}

// lambdaHint derives the name hint: the assigned variable, property, or
// keyword/parameter name when syntactically derivable, else "anon"
// (SPEC 2.3).
func lambdaHint(n *sitter.Node, src []byte) string {
	parent := n.Parent()
	if parent == nil {
		return "anon"
	}
	switch parent.Kind() {
	case "assignment":
		if left := parent.ChildByFieldName("left"); left != nil && left.Kind() == "identifier" {
			if right := parent.ChildByFieldName("right"); right != nil && right.Id() == n.Id() {
				return nodeText(left, src)
			}
		}
	case "keyword_argument":
		if name := parent.ChildByFieldName("name"); name != nil {
			return nodeText(name, src)
		}
	case "default_parameter", "typed_default_parameter":
		if name := parent.ChildByFieldName("name"); name != nil {
			return nodeText(name, src)
		}
	}
	return "anon"
}

// signatureFingerprint: first 8 hex chars of SHA-256 over the normalized
// signature text (whitespace collapsed) — SPEC 2.3.
func signatureFingerprint(params string) string {
	normalized := strings.Join(strings.Fields(params), " ")
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", sum[:4])
}

// collectParamNames adds parameter identifiers to the local-scope set.
func collectParamNames(params *sitter.Node, src []byte, out map[string]bool) {
	count := params.NamedChildCount()
	for i := uint(0); i < count; i++ {
		p := params.NamedChild(i)
		switch p.Kind() {
		case "identifier":
			out[nodeText(p, src)] = true
		case "typed_parameter", "default_parameter", "typed_default_parameter",
			"list_splat_pattern", "dictionary_splat_pattern", "keyword_separator", "positional_separator":
			cc := p.NamedChildCount()
			for j := uint(0); j < cc; j++ {
				if c := p.NamedChild(j); c.Kind() == "identifier" {
					out[nodeText(c, src)] = true
					break
				}
			}
			if name := p.ChildByFieldName("name"); name != nil {
				out[nodeText(name, src)] = true
			}
		}
	}
}
