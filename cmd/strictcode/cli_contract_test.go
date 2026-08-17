package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
)

// The registration-level contract strictcli v0.33.0 enforces, pinned here so a
// regression is a failing test and not a panic discovered at first run:
//
//   - every flag and arg declares exactly one presence
//   - a mutating command declares no value default (the fallbacks live in the
//     handlers, and the flags' help says so)
//   - `fix`'s apply-vs-preview decision is a required member-spelled selector,
//     not a pair of bools a guard has to reconcile

// TestAppRegisters is the whole registration contract in one assertion:
// strictcli validates presence, effect classification and the mutating-default
// ban at registration time, so building the app either succeeds or panics.
func TestAppRegisters(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registration rejected the CLI: %v", r)
		}
	}()
	if newApp() == nil {
		t.Fatal("newApp returned nil")
	}
}

func unreachableProject(t *testing.T) string {
	t.Helper()
	return fixture.Write(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "",
		"pkg/a.py":        "def f():\n    return 1\n    x = 2\n",
	})
}

// A member-spelled selector elects on PRESENCE, so declining a member is not a
// way to elect the other one. Under the old bool mutex `--no-apply` satisfied
// the group while selecting nothing, which is the exact shape that lets a
// negation mean "do the other thing".
func TestFixNegatedMemberDoesNotElect(t *testing.T) {
	root := unreachableProject(t)
	res := newApp().Test([]string{"fix", root, "--no-apply"})
	if res.ExitCode == 0 {
		t.Fatalf("--no-apply must not stand in for an election; exit 0 with:\n%s", res.Stdout)
	}
	if strings.Contains(res.Stdout, "fix(es) planned") {
		t.Errorf("--no-apply was read as --preview:\n%s", res.Stdout)
	}
}

// Neither member elected is refused by the framework, not by the handler.
func TestFixRequiresAnElection(t *testing.T) {
	root := unreachableProject(t)
	res := newApp().Test([]string{"fix", root})
	if res.ExitCode == 0 {
		t.Fatalf("fix with no --apply/--preview must fail:\n%s", res.Stdout)
	}
}

// Both members is likewise unrepresentable: a selector elects exactly one.
func TestFixRefusesBothMembers(t *testing.T) {
	root := unreachableProject(t)
	res := newApp().Test([]string{"fix", root, "--apply", "--preview"})
	if res.ExitCode == 0 {
		t.Fatalf("fix with both members must fail:\n%s", res.Stdout)
	}
}

// --preview lists the plan and leaves every file byte-identical.
func TestFixPreviewTouchesNothing(t *testing.T) {
	root := unreachableProject(t)
	target := filepath.Join(root, "pkg", "a.py")
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}

	res := newApp().Test([]string{"fix", root, "--preview"})
	if res.ExitCode != 0 {
		t.Fatalf("fix --preview exited %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "fix(es) planned") {
		t.Errorf("preview printed no plan:\n%s", res.Stdout)
	}

	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("preview rewrote %s", target)
	}
}

// --apply is the only spelling that writes, and the write is verified.
func TestFixApplyWrites(t *testing.T) {
	root := unreachableProject(t)
	target := filepath.Join(root, "pkg", "a.py")

	res := newApp().Test([]string{"fix", root, "--apply"})
	if res.ExitCode != 0 {
		t.Fatalf("fix --apply exited %d: %s", res.ExitCode, res.Stderr)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "x = 2") {
		t.Errorf("--apply left the unreachable statement in place:\n%s", after)
	}
}

// The mutating commands carry no declared default, so their fallbacks have to
// be reachable from an invocation that states nothing. These pin the third
// remedy the mutating-default ban names: the handler substitutes, and the help
// says so.

func TestRegistryDumpFallsBackToItsDefaultPath(t *testing.T) {
	t.Chdir(t.TempDir())
	res := newApp().Test([]string{"registry", "dump"})
	if res.ExitCode != 0 {
		t.Fatalf("registry dump exited %d: %s", res.ExitCode, res.Stderr)
	}
	if _, err := os.Stat(defaultRegistryOut); err != nil {
		t.Errorf("no dump at the documented fallback path %s: %v", defaultRegistryOut, err)
	}
}

func TestMatrixGenFallsBackToItsDefaultPath(t *testing.T) {
	t.Chdir(t.TempDir())
	res := newApp().Test([]string{"matrix", "gen"})
	if res.ExitCode != 0 {
		t.Fatalf("matrix gen exited %d: %s", res.ExitCode, res.Stderr)
	}
	if _, err := os.Stat(defaultMatrixOut); err != nil {
		t.Errorf("no matrix at the documented fallback path %s: %v", defaultMatrixOut, err)
	}
}

func TestFixFallsBackToTheCurrentDirectory(t *testing.T) {
	t.Chdir(unreachableProject(t))
	res := newApp().Test([]string{"fix", "--preview"})
	if res.ExitCode != 0 {
		t.Fatalf("fix --preview with no dir exited %d: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "fix(es) planned") {
		t.Errorf("the current-directory fallback found nothing to plan:\n%s", res.Stdout)
	}
}

// Each mutating flag whose default moved into its handler must say so, so an
// operator reading --help still learns what absence means.
func TestMutatingFallbacksAreDocumented(t *testing.T) {
	for _, tc := range []struct{ argv, want string }{
		{"fix", defaultConfigName},
		{"registry dump", defaultRegistryOut},
		{"matrix gen", defaultMatrixOut},
	} {
		res := newApp().Test(append(strings.Fields(tc.argv), "--help"))
		if !strings.Contains(res.Stdout, "omitted means "+tc.want) {
			t.Errorf("%s --help does not state its fallback %q:\n%s", tc.argv, tc.want, res.Stdout)
		}
	}
}
