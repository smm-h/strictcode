package checks

import (
	"sort"
	"strings"
	"testing"
)

// cycleSets renders each import-cycles finding's module set.
func cycleSets(t *testing.T, files map[string]string) []string {
	t.Helper()
	var out []string
	for _, f := range byRule(analyze(t, files), "import-cycles") {
		out = append(out, strings.TrimPrefix(f.Message, "import cycle among modules: "))
	}
	sort.Strings(out)
	return out
}

func pyPkg(modules map[string]string) map[string]string {
	files := map[string]string{"pyproject.toml": "[project]\nname = \"p\"\n", "pkg/__init__.py": ""}
	for name, src := range modules {
		files["pkg/"+name+".py"] = src
	}
	return files
}

// The cycle cases of rlsbl's circular-dependency tests.
func TestPortedCycleShapes(t *testing.T) {
	cases := map[string]struct {
		modules map[string]string
		want    []string
	}{
		"linear": {map[string]string{"a": "import pkg.b\n", "b": "import pkg.c\n", "c": ""}, nil},
		"tree":   {map[string]string{"a": "import pkg.b\nimport pkg.c\n", "b": "import pkg.d\n", "c": "import pkg.d\n", "d": ""}, nil},
		"two":    {map[string]string{"a": "import pkg.b\n", "b": "import pkg.a\n"}, []string{"pkg.a, pkg.b"}},
		"three":  {map[string]string{"a": "import pkg.b\n", "b": "import pkg.c\n", "c": "import pkg.a\n"}, []string{"pkg.a, pkg.b, pkg.c"}},
		"two independent cycles": {map[string]string{
			"a": "import pkg.b\n", "b": "import pkg.a\n",
			"x": "import pkg.y\n", "y": "import pkg.x\n",
		}, []string{"pkg.a, pkg.b", "pkg.x, pkg.y"}},
		"cycle with a tail": {map[string]string{
			"tail": "import pkg.a\n", "a": "import pkg.b\n", "b": "import pkg.a\nimport pkg.leaf\n", "leaf": "",
		}, []string{"pkg.a, pkg.b"}},
		"large": {map[string]string{
			"a": "import pkg.b\n", "b": "import pkg.c\n", "c": "import pkg.d\n", "d": "import pkg.e\n", "e": "import pkg.a\n",
		}, []string{"pkg.a, pkg.b, pkg.c, pkg.d, pkg.e"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := cycleSets(t, pyPkg(c.modules))
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Fatalf("cycles %v, want %v", got, c.want)
			}
		})
	}
}

// Lesson 47: a cycle among test modules is not reported.
func TestLesson47CyclesAmongTestsAreNotReported(t *testing.T) {
	files := pyPkg(map[string]string{"core": "x = 1\n"})
	files["tests/__init__.py"] = ""
	files["tests/test_a.py"] = "from tests.test_b import y\nx = 1\n"
	files["tests/test_b.py"] = "from tests.test_a import x\ny = 2\n"
	if got := cycleSets(t, files); len(got) != 0 {
		t.Fatalf("a cycle among tests was reported: %v", got)
	}
}

func TestPortedTypeScriptCycles(t *testing.T) {
	files := map[string]string{
		"package.json": `{"name": "app", "main": "./src/a.ts"}`,
		"src/a.ts":     "import { b } from './b';\nexport const a = 1;\n",
		"src/b.ts":     "import { c } from './c.js';\nexport const b = 1;\n",
		"src/c.ts":     "import { a } from './a';\nexport const c = 1;\n",
		"src/d.js":     "const e = require('./e.js');\n",
		"src/e.js":     "const d = require('./d.js');\n",
	}
	got := cycleSets(t, files)
	want := []string{"src/a, src/b, src/c", "src/d, src/e"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("cycles %v, want %v", got, want)
	}
}
