// Package engine is the stateless batch pipeline (DESIGN.md section 9):
// load config (hard errors), read the workspace from disk, extract the one
// shared relation, run the enabled checks, produce findings. No cache, no
// persistence, no state between runs.
package engine

import (
	"path/filepath"

	"github.com/smm-h/strictcode/internal/checks"
	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/workspace"
)

// Result is one analysis run's output.
type Result struct {
	// WorkspaceRoot is the absolute workspace root.
	WorkspaceRoot string
	Findings      []findings.Finding
}

// Analyze runs the full pipeline over the workspace at dir. cfgName is the
// config file name resolved relative to dir.
func Analyze(dir, cfgName string) (*Result, error) {
	ws, err := workspace.Load(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(ws.Root, filepath.FromSlash(cfgName)))
	if err != nil {
		return nil, err
	}
	res, err := extract.Extract(ws)
	if err != nil {
		return nil, err
	}
	fs := checks.Run(ws, res, cfg, cfgName)
	return &Result{WorkspaceRoot: ws.Root, Findings: fs}, nil
}
