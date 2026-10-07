package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/smm-h/strictcode/internal/checks"
)

// ExecRunner runs a Python tool rule's command as a child process,
// capturing its output, and stops it at the run's timeout.
func ExecRunner(run checks.ToolRun) (checks.ToolResult, error) {
	if len(run.Argv) == 0 {
		return checks.ToolResult{}, errors.New("no command")
	}
	c, cancel := context.WithTimeout(context.Background(), run.Timeout)
	defer cancel()
	cmd := exec.CommandContext(c, run.Argv[0], run.Argv[1:]...)
	cmd.Dir = run.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if c.Err() == context.DeadlineExceeded {
		return checks.ToolResult{}, fmt.Errorf("stopped after %s", run.Timeout)
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return checks.ToolResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}, nil
	case errors.As(err, &exitErr):
		return checks.ToolResult{ExitCode: exitErr.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}, nil
	default:
		return checks.ToolResult{}, err
	}
}
