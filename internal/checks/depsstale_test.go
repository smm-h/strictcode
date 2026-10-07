package checks

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
)

// npmPair is a workspace of two npm members: lib at libVersion, and app
// declaring lib with the given package.json version specifier.
func npmPair(libVersion, spec string) map[string]string {
	return map[string]string{
		fixture.DeclarationsPath: fixture.Workspace("path = \"lib\"\nname = \"lib\"\n", "path = \"app\"\nname = \"app\"\n"),
		"lib/package.json":       `{"name": "lib", "version": "` + libVersion + `", "main": "./index.js"}`,
		"lib/index.js":           "export const x = 1;\n",
		"app/package.json":       `{"name": "app", "version": "1.0.0", "main": "./index.js", "dependencies": {"lib": "` + spec + `"}}`,
		"app/index.js":           "import { x } from 'lib';\nexport const y = x;\n",
	}
}

// Lesson 33: a registry constraint the dependency member's declared
// version no longer satisfies is reported, naming both members, the
// constraint, and the current version.
func TestLesson33OutdatedConstraintIsReported(t *testing.T) {
	got := byRule(analyze(t, npmPair("2.0.0", "==1.0.0")), "deps-stale")
	if len(got) != 1 {
		t.Fatalf("outdated constraint must be reported once: %+v", got)
	}
	for _, want := range []string{"'app'", "'lib'", "==1.0.0", "2.0.0"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message %q does not name %q", got[0].Message, want)
		}
	}
	if got[0].Target.File != "app/package.json" || got[0].Severity != "error" {
		t.Errorf("finding site or severity: %+v", got[0])
	}
}

func TestLesson33SatisfiedConstraintIsClean(t *testing.T) {
	if got := byRule(analyze(t, npmPair("2.0.0", ">=1.0.0")), "deps-stale"); len(got) != 0 {
		t.Fatalf("satisfied constraint reported: %+v", got)
	}
}

func TestLesson33PathAndWorkspaceSourcesAreNotJudged(t *testing.T) {
	for _, spec := range []string{"file:../lib", "workspace:^1.0.0", "workspace:*"} {
		if got := byRule(analyze(t, npmPair("2.0.0", spec)), "deps-stale"); len(got) != 0 {
			t.Errorf("%s: a path or workspace source was judged: %+v", spec, got)
		}
	}
}

func TestLesson33UnevaluatedConstraintsGiveNoFinding(t *testing.T) {
	for _, spec := range []string{">=1.0.0,<2.0.0", "1.x", "*", "^1.0.0 || ^2.0.0", "!=2.0.0", ">=1.0.0 <2.0.0"} {
		if got := byRule(analyze(t, npmPair("2.0.0", spec)), "deps-stale"); len(got) != 0 {
			t.Errorf("%s: an unevaluated constraint was reported: %+v", spec, got)
		}
	}
}

func TestLesson33PythonConstraintAndSuppression(t *testing.T) {
	files := map[string]string{
		fixture.DeclarationsPath: fixture.Workspace("path = \"a\"\nname = \"a\"\n", "path = \"b\"\nname = \"b\"\n"),
		"a/pyproject.toml":       "[project]\nname = \"a\"\nversion = \"1.0.0\"\ndependencies = [\"b[fast]~=1.4 ; python_version >= '3.11'\"]\n",
		"a/a_pkg/__init__.py":    "import b\n",
		"b/pyproject.toml":       "[project]\nname = \"b\"\nversion = \"2.1.0\"\n",
		"b/b/__init__.py":        "",
	}
	got := byRule(analyze(t, files), "deps-stale")
	if len(got) != 1 || !strings.Contains(got[0].Message, "~=1.4") {
		t.Fatalf("a ~= constraint below the dependency's version must be reported once: %+v", got)
	}
	files["strictcode.toml"] = "format_version = 1\n[[rules.deps-stale.suppressions]]\nproject = \"a\"\ndep = \"b\"\nreason = \"a pins b until its own migration\"\n"
	if got := byRule(analyze(t, files), "deps-stale"); len(got) != 0 {
		t.Fatalf("(project, dep) suppression not honored: %+v", got)
	}
}

func TestLesson33DependencyWithoutAStaticVersionIsNotJudged(t *testing.T) {
	files := map[string]string{
		fixture.DeclarationsPath: fixture.Workspace("path = \"a\"\nname = \"a\"\n", "path = \"b\"\nname = \"b\"\n"),
		"a/pyproject.toml":       "[project]\nname = \"a\"\ndependencies = [\"b==1.0\"]\n",
		"a/a_pkg/__init__.py":    "import b\n",
		"b/pyproject.toml":       "[project]\nname = \"b\"\ndynamic = [\"version\"]\n",
		"b/b/__init__.py":        "",
	}
	if got := byRule(analyze(t, files), "deps-stale"); len(got) != 0 {
		t.Fatalf("a dynamic version was judged: %+v", got)
	}
}

func TestEvaluateConstraint(t *testing.T) {
	cases := []struct {
		constraint, version string
		want                constraintVerdict
	}{
		{">=1.0.0", "2.0.0", constraintSatisfied},
		{">=2.0.1", "2.0.0", constraintOutdated},
		{">1.0", "1.0", constraintOutdated},
		{"<=1.0", "1.0", constraintSatisfied},
		{"<2", "2.0", constraintOutdated},
		{"==1.0.0", "1.0.0", constraintSatisfied},
		{"=1.0.0", "1.0.1", constraintOutdated},
		{"1.0.0", "1.0.0", constraintSatisfied},
		{"^1.2.0", "1.9.0", constraintSatisfied},
		{"^1.2.0", "2.0.0", constraintOutdated},
		{"^1.2.0", "1.1.0", constraintOutdated},
		{"^0.3.0", "0.3.9", constraintSatisfied},
		{"^0.3.0", "0.4.0", constraintOutdated},
		{"~1.2.0", "1.2.7", constraintSatisfied},
		{"~1.2.0", "1.3.0", constraintOutdated},
		{"~=1.4", "1.4.2", constraintSatisfied},
		{"~=1.4", "2.1.0", constraintOutdated},
		{">=1.0,<2.0", "3.0", constraintUnevaluated},
		{"!=1.0", "1.0", constraintUnevaluated},
		{"1.x", "1.0", constraintUnevaluated},
		{">=1.0", "1.0.0rc1", constraintUnevaluated},
		{"", "1.0", constraintUnevaluated},
		{">=+1.0", "1.0", constraintUnevaluated},
	}
	for _, c := range cases {
		if got := evaluateConstraint(c.constraint, c.version); got != c.want {
			t.Errorf("evaluateConstraint(%q, %q) = %d, want %d", c.constraint, c.version, got, c.want)
		}
	}
}
