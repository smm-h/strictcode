package extract

// Pass-2 resolution for Python full-semantic extraction: with every
// member's pass-1 record in hand, resolve call sites, decorators, and base
// references through the module-level symbol tables, emitting calls,
// instantiates, decorates, and conforms_to rows for workspace-local
// targets, and honest side data otherwise.

import (
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
)

// pyTarget is a resolved workspace-local symbol.
type pyTarget struct {
	member string
	module string
	chain  []relation.Segment
	kind   vocab.NodeKind // NodeKindFunction | NodeKindType | NodeKindModule
}

func (t pyTarget) nodeID() relation.NodeID {
	return relation.NodeID{Lang: "py", Member: t.member, Module: t.module, Chain: t.chain}
}

func (ex *extraction) resolvePySemantics() error {
	// Deterministic iteration: members in workspace order, modules sorted.
	for _, m := range ex.ws.Members {
		mods := ex.pySem.mods[m.Name]
		logicals := make([]string, 0, len(mods))
		for l := range mods {
			logicals = append(logicals, l)
		}
		sort.Strings(logicals)
		for _, logical := range logicals {
			if err := ex.resolveModule(mods[logical]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ex *extraction) resolveModule(sem *pyModSem) error {
	ex.unreachable = append(ex.unreachable, sem.unreachable...)

	// Base references -> conforms_to (declared, nominal, inheritance).
	classKeys := make([]string, 0, len(sem.classes))
	for k := range sem.classes {
		classKeys = append(classKeys, k)
	}
	sort.Strings(classKeys)
	for _, key := range classKeys {
		info := sem.classes[key]
		for _, base := range info.bases {
			target, ok := ex.resolvePyName(sem, strings.Split(base.expr, "."))
			if !ok || target.kind != vocab.NodeKindType {
				continue
			}
			row := relation.Row{
				Kind: vocab.RowKindConformsTo,
				Src:  relation.NodeID{Lang: "py", Member: sem.member.Name, Module: sem.logical, Chain: info.chain},
				Dst:  target.nodeID(),
				File: sem.wsPath,
				Span: base.span,
				Attrs: map[string]relation.Value{
					"provenance": relation.StringValue("declared"),
					"discipline": relation.StringValue("nominal"),
					"mechanism":  relation.StringValue("inheritance"),
				},
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
		}
	}

	// Decorators -> decorates rows (resolvable local functions only; an
	// unresolvable decorator produces no row — the row's src must exist).
	for _, d := range sem.decos {
		target, ok := ex.resolvePyName(sem, strings.Split(d.expr, "."))
		if !ok || target.kind != vocab.NodeKindFunction {
			continue
		}
		row := relation.Row{
			Kind: vocab.RowKindDecorates,
			Src:  target.nodeID(),
			Dst:  relation.NodeID{Lang: "py", Member: sem.member.Name, Module: sem.logical, Chain: d.targetChain},
			File: sem.wsPath,
			Span: d.span,
		}
		if err := ex.builder.AddRow(row); err != nil {
			return err
		}
	}

	// Call sites.
	for _, call := range sem.calls {
		if err := ex.resolveCall(sem, call); err != nil {
			return err
		}
	}
	return nil
}

func (ex *extraction) resolveCall(sem *pyModSem, call pyCallRec) error {
	containerID := relation.NodeID{Lang: "py", Member: sem.member.Name, Module: sem.logical, Chain: call.container}
	site := CallSite{
		Lang:        vocab.LangPy,
		Member:      sem.member.Name,
		Module:      sem.logical,
		Container:   containerID.String(),
		File:        sem.wsPath,
		Span:        call.span,
		TestContext: sem.isTest,
	}

	if call.callee == "" {
		site.Resolution = CallUnresolved
		ex.callSites = append(ex.callSites, site)
		return nil
	}
	parts := strings.Split(call.callee, ".")
	site.Callee = ex.canonicalCallee(sem, parts)

	// ABC register pattern: X.register(C) with X and C local types ->
	// conforms_to (declared_external, nominal, register).
	if len(parts) >= 2 && parts[len(parts)-1] == "register" && call.firstArg != "" {
		if base, ok := ex.resolvePyName(sem, parts[:len(parts)-1]); ok && base.kind == vocab.NodeKindType {
			if impl, ok := ex.resolvePyName(sem, []string{call.firstArg}); ok && impl.kind == vocab.NodeKindType {
				row := relation.Row{
					Kind: vocab.RowKindConformsTo,
					Src:  impl.nodeID(),
					Dst:  base.nodeID(),
					File: sem.wsPath,
					Span: call.span,
					Attrs: map[string]relation.Value{
						"provenance": relation.StringValue("declared_external"),
						"discipline": relation.StringValue("nominal"),
						"mechanism":  relation.StringValue("register"),
					},
				}
				if err := ex.builder.AddRow(row); err != nil {
					return err
				}
			}
		}
	}

	target, ok := ex.resolvePyCallTarget(sem, call)
	switch {
	case ok && target.kind == vocab.NodeKindFunction || ok && target.kind == vocab.NodeKindClosure:
		site.Resolution = CallSyntactic
		site.Target = target.nodeID().String()
		if call.inCallable {
			row := relation.Row{
				Kind:  vocab.RowKindCalls,
				Src:   containerID,
				Dst:   target.nodeID(),
				File:  sem.wsPath,
				Span:  call.span,
				Attrs: map[string]relation.Value{"resolution": relation.StringValue("syntactic")},
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
		}
	case ok && target.kind == vocab.NodeKindType:
		site.Resolution = CallSyntactic
		site.Target = target.nodeID().String()
		if call.inCallable {
			row := relation.Row{
				Kind: vocab.RowKindInstantiates,
				Src:  containerID,
				Dst:  target.nodeID(),
				File: sem.wsPath,
				Span: call.span,
			}
			if err := ex.builder.AddRow(row); err != nil {
				return err
			}
		}
	default:
		if ex.pyCalleeIsExternal(sem, parts) {
			site.Resolution = CallExternal
		} else {
			site.Resolution = CallUnresolved
		}
	}
	ex.callSites = append(ex.callSites, site)
	return nil
}

// canonicalCallee expands the callee head through module-level bindings:
// `import sys as s; s.stdout.write` -> sys.stdout.write; `from sys import
// stdout; stdout.write` -> sys.stdout.write. Unbound heads pass through.
func (ex *extraction) canonicalCallee(sem *pyModSem, parts []string) string {
	b, ok := sem.bindings[parts[0]]
	if !ok {
		return strings.Join(parts, ".")
	}
	switch b.kind {
	case bindModule:
		return strings.Join(append([]string{b.module}, parts[1:]...), ".")
	case bindName:
		return strings.Join(append([]string{b.source, b.orig}, parts[1:]...), ".")
	}
	return strings.Join(parts, ".")
}

// resolvePyCallTarget resolves a call site's callee, honoring local-scope
// shadowing, self/cls method dispatch, and nested defs before module-level
// names.
func (ex *extraction) resolvePyCallTarget(sem *pyModSem, call pyCallRec) (pyTarget, bool) {
	parts := strings.Split(call.callee, ".")
	head := parts[0]

	// self.m() / cls.m(): method lookup on the enclosing class, chasing
	// locally resolvable bases; not found -> unresolved (could be
	// inherited from an external base — never guess).
	if (head == "self" || head == "cls") && len(parts) == 2 {
		classChain := enclosingClassChain(sem, call.container)
		if classChain == nil {
			return pyTarget{}, false
		}
		return ex.lookupMethod(sem, classChain, parts[1], map[string]bool{})
	}

	// Local shadowing: a name bound as a parameter or local assignment in
	// any enclosing callable makes the call unresolvable (no dataflow).
	for i := len(call.container); i > 0; i-- {
		ck := chainKey(call.container[:i])
		if locals, ok := sem.callableLocals[ck]; ok && locals[head] {
			// Nested defs beat the shadow set (a nested def IS the local).
			if defs, ok := sem.nestedDefs[ck]; ok && defs[head] > 0 && len(parts) == 1 {
				count := defs[head]
				chain := append(append([]relation.Segment{}, call.container[:i]...),
					relation.Segment{Name: head, Overload: count - 1})
				return pyTarget{member: sem.member.Name, module: sem.logical, chain: chain, kind: vocab.NodeKindFunction}, true
			}
			return pyTarget{}, false
		}
		if defs, ok := sem.nestedDefs[ck]; ok && defs[head] > 0 && len(parts) == 1 {
			count := defs[head]
			chain := append(append([]relation.Segment{}, call.container[:i]...),
				relation.Segment{Name: head, Overload: count - 1})
			return pyTarget{member: sem.member.Name, module: sem.logical, chain: chain, kind: vocab.NodeKindFunction}, true
		}
	}

	return ex.resolvePyName(sem, parts)
}

// enclosingClassChain walks the container chain outward to the nearest
// class segment and returns the chain up to it.
func enclosingClassChain(sem *pyModSem, container []relation.Segment) []relation.Segment {
	for i := len(container); i > 0; i-- {
		key := plainChainKey(container[:i])
		if sem.classes[key] != nil {
			return container[:i]
		}
	}
	return nil
}

// plainChainKey renders a chain with names only (classes register under
// plain keys; the last same-name definition wins).
func plainChainKey(chain []relation.Segment) string {
	parts := make([]string, len(chain))
	for i, s := range chain {
		parts[i] = s.Name
	}
	return strings.Join(parts, ".")
}

// lookupMethod finds a method on a class, chasing locally resolvable bases
// (visited guards cycles). Unfound methods are unresolved, never guessed.
func (ex *extraction) lookupMethod(sem *pyModSem, classChain []relation.Segment, method string, visited map[string]bool) (pyTarget, bool) {
	key := sem.member.Name + "\x00" + sem.logical + "\x00" + plainChainKey(classChain)
	if visited[key] {
		return pyTarget{}, false
	}
	visited[key] = true

	info := sem.classes[plainChainKey(classChain)]
	if info == nil {
		return pyTarget{}, false
	}
	if count := info.methods[method]; count > 0 {
		chain := append(append([]relation.Segment{}, info.chain...),
			relation.Segment{Name: method, Overload: count - 1})
		return pyTarget{member: sem.member.Name, module: sem.logical, chain: chain, kind: vocab.NodeKindFunction}, true
	}
	for _, base := range info.bases {
		baseTarget, ok := ex.resolvePyName(sem, strings.Split(base.expr, "."))
		if !ok || baseTarget.kind != vocab.NodeKindType {
			continue
		}
		baseSem := ex.pySem.get(baseTarget.member, baseTarget.module)
		if baseSem == nil {
			continue
		}
		if t, ok := ex.lookupMethod(baseSem, baseTarget.chain, method, visited); ok {
			return t, true
		}
	}
	return pyTarget{}, false
}

// resolvePyName resolves a dotted name in a module's scope to a
// workspace-local function, type, or module: module-level defs and
// classes, then import bindings into other modules.
func (ex *extraction) resolvePyName(sem *pyModSem, parts []string) (pyTarget, bool) {
	head := parts[0]

	// Module-level class.
	if info := sem.classes[head]; info != nil {
		switch len(parts) {
		case 1:
			return pyTarget{member: sem.member.Name, module: sem.logical, chain: info.chain, kind: vocab.NodeKindType}, true
		case 2:
			// ClassName.method (static-style call).
			return ex.lookupMethod(sem, info.chain, parts[1], map[string]bool{})
		}
		return pyTarget{}, false
	}
	// Module-level function.
	if count := sem.funcs[head]; count > 0 && sem.classes[head] == nil {
		if len(parts) != 1 {
			return pyTarget{}, false // attribute access on a function
		}
		return pyTarget{
			member: sem.member.Name, module: sem.logical,
			chain: []relation.Segment{{Name: head, Overload: count - 1}},
			kind:  vocab.NodeKindFunction,
		}, true
	}

	// Import bindings.
	if b, ok := sem.bindings[head]; ok {
		switch b.kind {
		case bindModule:
			return ex.resolveDottedGlobal(sem, append(strings.Split(b.module, "."), parts[1:]...))
		case bindName:
			return ex.resolveDottedGlobal(sem, append(append(strings.Split(b.source, "."), b.orig), parts[1:]...))
		}
	}
	return pyTarget{}, false
}

// resolveDottedGlobal resolves an absolute dotted path against the
// workspace's Python modules: the longest module-logical prefix wins, and
// the remainder resolves within that module (function/class, or
// class.method).
func (ex *extraction) resolveDottedGlobal(sem *pyModSem, parts []string) (pyTarget, bool) {
	dotted := strings.Join(parts, ".")

	// Find the owning module: prefer the calling member's own modules, then
	// any member's (cross-member namespace imports resolve too).
	findModule := func(logical string) *pyModSem {
		if own := ex.pySem.get(sem.member.Name, logical); own != nil {
			return own
		}
		for _, m := range ex.ws.Members {
			if s := ex.pySem.get(m.Name, logical); s != nil {
				return s
			}
		}
		return nil
	}

	for cut := len(parts); cut >= 1; cut-- {
		prefix := strings.Join(parts[:cut], ".")
		target := findModule(prefix)
		if target == nil {
			continue
		}
		rest := parts[cut:]
		switch len(rest) {
		case 0:
			return pyTarget{member: target.member.Name, module: prefix, kind: vocab.NodeKindModule}, true
		case 1, 2:
			return ex.resolvePyName(target, rest)
		default:
			return pyTarget{}, false
		}
	}
	_ = dotted
	return pyTarget{}, false
}

// pyCalleeIsExternal classifies an unresolvable callee: stdlib/builtin
// heads and heads bound to external imports are external; unknown bare
// names under a star import (or plain unknown) are unresolved.
func (ex *extraction) pyCalleeIsExternal(sem *pyModSem, parts []string) bool {
	head := parts[0]
	if head == "self" || head == "cls" {
		return false
	}
	if b, ok := sem.bindings[head]; ok {
		// Bound to an import that did not resolve to a workspace module:
		// external package or stdlib.
		target := b.module
		if b.kind == bindName {
			target = b.source
		}
		if _, ok := ex.resolveDottedGlobal(sem, strings.Split(target, ".")); !ok {
			return true
		}
		return false
	}
	if pyStdlib[head] || pyBuiltins[head] {
		return true
	}
	return false
}

// pyBuiltins: Python builtin callables relevant to honest external
// classification (library-stdout keys on print).
var pyBuiltins = map[string]bool{
	"print": true, "open": true, "input": true, "exec": true, "eval": true,
	"len": true, "range": true, "isinstance": true, "issubclass": true,
	"getattr": true, "setattr": true, "hasattr": true, "repr": true,
	"str": true, "int": true, "float": true, "bool": true, "list": true,
	"dict": true, "set": true, "tuple": true, "type": true, "super": true,
	"sorted": true, "enumerate": true, "zip": true, "map": true,
	"filter": true, "vars": true, "id": true, "hash": true, "iter": true,
	"next": true, "min": true, "max": true, "sum": true, "abs": true,
	"round": true, "format": true, "any": true, "all": true,
}
