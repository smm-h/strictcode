package findings

import (
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/vocab"
)

func sample() []Finding {
	return []Finding{
		{
			Rule: "dead-modules", Severity: rules.SeverityWarning,
			Message: "module b.legacy is never imported",
			Target:  Target{ID: "py:core:b%2Elegacy:", Kind: vocab.NodeKindModule, File: "core/src/b/legacy.py", Line: 1},
		},
		{
			Rule: "deps-unused", Severity: rules.SeverityError,
			Message: "declared dependency transport is never imported",
			Target:  Target{ID: "py:core:_:", Kind: vocab.NodeKindWorkspaceMember, File: "core/pyproject.toml", Line: 1},
			Fix:     &Fix{Tier: 3, Description: "Remove the declaration or import the package."},
		},
	}
}

func TestRenderJSONIsSchemaValid(t *testing.T) {
	out, err := RenderJSON("0.0.0", "/ws", sample())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"strictcode_version": "0.0.0"`, `"rule": "deps-unused"`, `"tier": 3`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON missing %q", want)
		}
	}
}

func TestRenderJSONEmptyFindings(t *testing.T) {
	out, err := RenderJSON("0.0.0", "/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"findings": []`) {
		t.Fatalf("empty findings must render as []: %s", out)
	}
}

func TestRenderText(t *testing.T) {
	text := RenderText(sample())
	// Sorted by file: pyproject.toml before src/.
	first := strings.Index(text, "core/pyproject.toml:1: error: declared dependency")
	second := strings.Index(text, "core/src/b/legacy.py:1: warning: module")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("text ordering wrong:\n%s", text)
	}
	if !strings.Contains(text, "2 finding(s): 1 error(s), 1 warning(s)") {
		t.Fatalf("summary missing:\n%s", text)
	}
	if !strings.Contains(text, "fix (tier 3):") {
		t.Fatalf("fix line missing:\n%s", text)
	}
	if RenderText(nil) != "no findings\n" {
		t.Fatal("empty text rendering")
	}
}

func TestFailRun(t *testing.T) {
	if FailRun(nil) {
		t.Fatal("no findings must pass")
	}
	warnOnly := []Finding{{Rule: "x", Severity: rules.SeverityWarning}}
	if FailRun(warnOnly) {
		t.Fatal("warnings alone must not fail the run")
	}
	if !FailRun(sample()) {
		t.Fatal("error finding must fail the run")
	}
}
