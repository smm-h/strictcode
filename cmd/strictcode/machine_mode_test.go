package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
)

// Machine output is the framework's: `--json` enters machine mode, stdout
// carries exactly one document (the envelope), and the findings document is
// its `payload` member, validated against the command's declared schema. The
// old `--format json` spelling is gone with no shim.

func analyzableProject(t *testing.T) string {
	t.Helper()
	return fixture.Write(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\n",
		"pkg/__init__.py": "",
		"pkg/a.py":        "def f():\n    return 1\n    x = 2\n",
	})
}

func TestMachineModeStdoutIsTheEnvelope(t *testing.T) {
	root := analyzableProject(t)
	res := newApp().Test([]string{"analyze", root, "--json"})

	var env map[string]interface{}
	if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, res.Stdout)
	}
	if env["app"] != "strictcode" {
		t.Errorf("app = %v, want strictcode", env["app"])
	}
	if env["command"] != "analyze" {
		t.Errorf("command = %v, want analyze", env["command"])
	}

	payload, ok := env["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope carries no payload object: %s", res.Stdout)
	}
	if payload["format_version"] != float64(1) {
		t.Errorf("payload format_version = %v, want 1", payload["format_version"])
	}
	if payload["workspace_root"] == nil {
		t.Error("payload has no workspace_root")
	}
	if _, ok := payload["findings"].([]interface{}); !ok {
		t.Errorf("payload has no findings array: %v", payload)
	}

	// The human report never reaches stdout in machine mode.
	if strings.Contains(res.Stdout, "finding(s):") {
		t.Errorf("text report leaked into machine-mode stdout:\n%s", res.Stdout)
	}
}

func TestFormatFlagIsGone(t *testing.T) {
	root := analyzableProject(t)
	res := newApp().Test([]string{"analyze", root, "--format", "json"})
	if res.ExitCode == 0 {
		t.Fatal("--format must be an unknown flag now")
	}
	if !strings.Contains(res.Stderr, "format") {
		t.Errorf("error should name the unknown flag: %s", res.Stderr)
	}
}

func TestHumanModeStillRendersTheTextReport(t *testing.T) {
	root := analyzableProject(t)
	res := newApp().Test([]string{"analyze", root})
	if !strings.Contains(res.Stdout, "finding(s):") {
		t.Errorf("human mode lost its report:\n%s", res.Stdout)
	}
	if strings.Contains(res.Stdout, `"interface_version"`) {
		t.Errorf("the envelope must never appear outside machine mode:\n%s", res.Stdout)
	}
}

func TestExitCodeSurvivesMachineMode(t *testing.T) {
	// An error-severity finding still fails the run, and the envelope
	// reports the same status the process leaves with.
	root := fixture.Write(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"solo\"\ndependencies = [\"transport\"]\n",
		"pkg/__init__.py": "",
		"pkg/a.py":        "def f():\n    return 1\n",
	})
	res := newApp().Test([]string{"analyze", root, "--json"})

	var env map[string]interface{}
	if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, res.Stdout)
	}
	if env["exit_code"] != float64(res.ExitCode) {
		t.Errorf("envelope exit_code %v does not match the run's %d",
			env["exit_code"], res.ExitCode)
	}
}
