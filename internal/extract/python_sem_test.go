package extract

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/relation"
	"github.com/smm-h/strictcode/internal/vocab"
)

// semWorkspace: a single-member workspace exercising the full-semantic
// surface.
func semWorkspace() map[string]string {
	return map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "",
		"pkg/svc.py": `import helpers
from pkg.util import helper as h
import typing
import enum

class Base:
    def save(self):
        return 1

class Service(Base):
    def run(self):
        self.save()
        h()
        helpers.run_all()
        transform = lambda x: x
        cb = lambda: 0
        other = lambda: 0
        return transform(1)

class Proto(typing.Protocol):
    def close(self): ...

class Color(enum.Enum):
    RED = 1

def _private():
    pass

async def fetch():
    def inner():
        return 2
    return inner()

def overloaded():
    return 1

def overloaded():
    return 2

def registry_user():
    Base.register(Color)
    svc = Service()
    svc.run()
`,
		"pkg/util.py": `def helper():
    return 1

def decorate(fn):
    return fn
`,
		"pkg/decorated.py": `from pkg.util import decorate

@decorate
def wrapped():
    return 1

@external.route("/x")
def routed():
    return 2
`,
		"helpers.py": "def run_all():\n    return 0\n",
	}
}

func semNodes(t *testing.T, res *Result, kind vocab.NodeKind) map[string]relation.Node {
	t.Helper()
	return nodeSet(res, kind)
}

func TestPySemFunctionAndTypeNodes(t *testing.T) {
	res := run(t, semWorkspace())
	funcs := semNodes(t, res, vocab.NodeKindFunction)
	types := semNodes(t, res, vocab.NodeKindType)

	for _, want := range []string{
		"py:_:pkg%2Esvc:Base.save",
		"py:_:pkg%2Esvc:Service.run",
		"py:_:pkg%2Esvc:_private",
		"py:_:pkg%2Esvc:fetch",
		"py:_:pkg%2Esvc:fetch.inner",
		"py:_:pkg%2Esvc:overloaded",
		"py:_:pkg%2Esvc:overloaded#1",
		"py:_:pkg%2Eutil:helper",
	} {
		if _, ok := funcs[want]; !ok {
			t.Errorf("missing function node %s", want)
		}
	}
	for _, want := range []string{
		"py:_:pkg%2Esvc:Base",
		"py:_:pkg%2Esvc:Service",
		"py:_:pkg%2Esvc:Proto",
		"py:_:pkg%2Esvc:Color",
	} {
		if _, ok := types[want]; !ok {
			t.Errorf("missing type node %s", want)
		}
	}

	// Attribute discipline.
	run := funcs["py:_:pkg%2Esvc:Service.run"]
	if v, _ := run.Attrs["is_method"].AsBool(); !v {
		t.Error("Service.run must be is_method")
	}
	fetch := funcs["py:_:pkg%2Esvc:fetch"]
	if v, _ := fetch.Attrs["is_async"].AsBool(); !v {
		t.Error("fetch must be is_async")
	}
	if v, _ := fetch.Attrs["is_method"].AsBool(); v {
		t.Error("fetch is not a method")
	}
	priv := funcs["py:_:pkg%2Esvc:_private"]
	if v, _ := priv.Attrs["visibility"].AsString(); v != "private_convention" {
		t.Errorf("_private visibility = %s", v)
	}
	proto := types["py:_:pkg%2Esvc:Proto"]
	if v, _ := proto.Attrs["form"].AsString(); v != "protocol" {
		t.Errorf("Proto form = %s", v)
	}
	color := types["py:_:pkg%2Esvc:Color"]
	if v, _ := color.Attrs["form"].AsString(); v != "enum" {
		t.Errorf("Color form = %s", v)
	}
}

func TestPySemLambdaIdentity(t *testing.T) {
	res := run(t, semWorkspace())
	closures := semNodes(t, res, vocab.NodeKindClosure)

	fpX := signatureFingerprint("x")
	fpEmpty := signatureFingerprint("")

	// transform = lambda x: x -> hint transform, ordinal 0, fp over "x".
	want := "py:_:pkg%2Esvc:Service.run.transform~0~" + fpX
	if _, ok := closures[want]; !ok {
		t.Fatalf("missing transform lambda %s (have %v)", want, keys(closures))
	}
	// cb and other: distinct hints, each ordinal 0, empty-params fp.
	for _, hint := range []string{"cb", "other"} {
		id := "py:_:pkg%2Esvc:Service.run." + hint + "~0~" + fpEmpty
		if _, ok := closures[id]; !ok {
			t.Errorf("missing lambda %s", id)
		}
	}
	// Closure attrs carry the identity fields.
	n := closures[want]
	if h, _ := n.Attrs["name_hint"].AsString(); h != "transform" {
		t.Errorf("name_hint = %s", h)
	}
	if fp, _ := n.Attrs["fingerprint"].AsString(); fp != fpX {
		t.Errorf("fingerprint = %s", fp)
	}
}

func TestPySemSameHintOrdinals(t *testing.T) {
	res := run(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"solo\"\n",
		"m.py":           "def f(with_cb):\n    with_cb(cb=lambda: 1)\n    with_cb(cb=lambda: 2)\n",
	})
	closures := semNodes(t, res, vocab.NodeKindClosure)
	fp := signatureFingerprint("")
	for _, want := range []string{
		"py:_:m:f.cb~0~" + fp,
		"py:_:m:f.cb~1~" + fp,
	} {
		if _, ok := closures[want]; !ok {
			t.Fatalf("missing same-hint ordinal closure %s (have %v)", want, keys(closures))
		}
	}
}

func TestPySemContainment(t *testing.T) {
	res := run(t, semWorkspace())
	contains := rowSet(res, vocab.RowKindContains)
	for _, want := range []string{
		"py:_:pkg%2Esvc: -> py:_:pkg%2Esvc:Service",
		"py:_:pkg%2Esvc:Service -> py:_:pkg%2Esvc:Service.run",
		"py:_:pkg%2Esvc:fetch -> py:_:pkg%2Esvc:fetch.inner",
	} {
		if _, ok := contains[want]; !ok {
			t.Errorf("missing contains row %s", want)
		}
	}
}

func TestPySemCallResolution(t *testing.T) {
	res := run(t, semWorkspace())
	calls := rowSet(res, vocab.RowKindCalls)

	// self.save() -> Base.save via base chase? No: Service inherits Base;
	// save is defined on Base only, so the method lookup chases the local
	// base.
	if _, ok := calls["py:_:pkg%2Esvc:Service.run -> py:_:pkg%2Esvc:Base.save"]; !ok {
		t.Errorf("self.save() not resolved through local base: %v", keys(calls))
	}
	// from-import alias call: h() -> pkg.util.helper.
	if _, ok := calls["py:_:pkg%2Esvc:Service.run -> py:_:pkg%2Eutil:helper"]; !ok {
		t.Errorf("aliased from-import call not resolved: %v", keys(calls))
	}
	// import-module call: helpers.run_all() -> helpers module function.
	if _, ok := calls["py:_:pkg%2Esvc:Service.run -> py:_:helpers:run_all"]; !ok {
		t.Errorf("module-attribute call not resolved: %v", keys(calls))
	}
	// nested def call: inner() within fetch.
	if _, ok := calls["py:_:pkg%2Esvc:fetch -> py:_:pkg%2Esvc:fetch.inner"]; !ok {
		t.Errorf("nested def call not resolved: %v", keys(calls))
	}
	// svc.run() where svc is a local variable: must NOT be resolved.
	for key := range calls {
		if strings.Contains(key, "registry_user") && strings.Contains(key, "Service.run") {
			t.Errorf("local-variable method call was guessed: %s", key)
		}
	}

	// Instantiation: Service() in registry_user.
	inst := rowSet(res, vocab.RowKindInstantiates)
	if _, ok := inst["py:_:pkg%2Esvc:registry_user -> py:_:pkg%2Esvc:Service"]; !ok {
		t.Errorf("instantiates row missing: %v", keys(inst))
	}
}

func TestPySemCallSideTable(t *testing.T) {
	res := run(t, semWorkspace())
	byCallee := map[string][]CallSite{}
	for _, c := range res.Calls {
		byCallee[c.Callee] = append(byCallee[c.Callee], c)
	}
	// transform(1): resolved to the lambda? transform is a local
	// assignment (lambda) — local shadowing makes it unresolved (no
	// dataflow); it must NOT be guessed.
	for _, c := range byCallee["transform"] {
		if c.Resolution == CallSyntactic {
			t.Error("local lambda variable call was guessed as syntactic")
		}
	}
	// svc.run(): unresolved (instance dispatch).
	found := false
	for _, c := range byCallee["svc.run"] {
		if c.Resolution == CallUnresolved {
			found = true
		}
	}
	if !found {
		t.Errorf("svc.run must be an unresolved site: %+v", byCallee)
	}
}

func TestPySemExternalCallsCanonicalized(t *testing.T) {
	res := run(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"solo\"\n",
		"m.py": `import sys as s
from sys import stderr
import logging

def f():
    print("x")
    s.stdout.write("y")
    stderr.write("z")
    logging.info("w")
`,
	})
	callees := map[string]CallResolution{}
	for _, c := range res.Calls {
		callees[c.Callee] = c.Resolution
	}
	for _, want := range []string{"print", "sys.stdout.write", "sys.stderr.write", "logging.info"} {
		if callees[want] != CallExternal {
			t.Errorf("callee %q = %q, want external (canonicalized)", want, callees[want])
		}
	}
}

func TestPySemDecorates(t *testing.T) {
	res := run(t, semWorkspace())
	decorates := rowSet(res, vocab.RowKindDecorates)
	if _, ok := decorates["py:_:pkg%2Eutil:decorate -> py:_:pkg%2Edecorated:wrapped"]; !ok {
		t.Fatalf("decorates row missing: %v", keys(decorates))
	}
	// @external.route: unresolvable decorator, no fabricated row.
	for key := range decorates {
		if strings.Contains(key, "routed") {
			t.Errorf("external decorator produced a row: %s", key)
		}
	}
}

func TestPySemConformance(t *testing.T) {
	res := run(t, semWorkspace())
	conforms := rowSet(res, vocab.RowKindConformsTo)

	r, ok := conforms["py:_:pkg%2Esvc:Service -> py:_:pkg%2Esvc:Base"]
	if !ok {
		t.Fatalf("inheritance conforms_to missing: %v", keys(conforms))
	}
	if p, _ := r.Attrs["provenance"].AsString(); p != "declared" {
		t.Errorf("provenance = %s", p)
	}
	if m, _ := r.Attrs["mechanism"].AsString(); m != "inheritance" {
		t.Errorf("mechanism = %s", m)
	}

	// ABC register pattern: Base.register(Color).
	reg, ok := conforms["py:_:pkg%2Esvc:Color -> py:_:pkg%2Esvc:Base"]
	if !ok {
		t.Fatalf("register conforms_to missing: %v", keys(conforms))
	}
	if m, _ := reg.Attrs["mechanism"].AsString(); m != "register" {
		t.Errorf("register mechanism = %s", m)
	}
	if p, _ := reg.Attrs["provenance"].AsString(); p != "declared_external" {
		t.Errorf("register provenance = %s", p)
	}
}

func TestPySemUnreachableRegions(t *testing.T) {
	res := run(t, map[string]string{
		"pyproject.toml": "[project]\nname = \"solo\"\n",
		"m.py": `def f():
    return 1
    x = 2
    y = 3

def g():
    if True:
        return 1
    else:
        raise ValueError
    dead = True

def fine():
    if maybe():
        return 1
    alive = 2
    return alive
`,
	})
	if len(res.Unreachable) != 2 {
		t.Fatalf("want 2 unreachable regions, got %+v", res.Unreachable)
	}
	first := res.Unreachable[0]
	if !strings.Contains(first.Container, "m:f") {
		t.Errorf("first region container = %s", first.Container)
	}
	// The region spans x = 2 through y = 3.
	text := "def f():\n    return 1\n    x = 2\n    y = 3\n"
	start := strings.Index(text, "x = 2")
	end := strings.Index(text, "y = 3") + len("y = 3")
	if first.Span.Start != uint32(start) || first.Span.End != uint32(end) {
		t.Errorf("region span %d..%d, want %d..%d", first.Span.Start, first.Span.End, start, end)
	}
}
