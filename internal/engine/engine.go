// Package engine is the stateless batch pipeline (stricttools/docs/implementation.md, runtime model):
// load the declarations and the options (hard errors), read the workspace
// from disk, extract the one shared relation, run the checks whose options
// are not off, produce findings. No cache, no persistence, no state between
// runs.
package engine

import (
	"path/filepath"

	"github.com/smm-h/strictcode/internal/checks"
	"github.com/smm-h/strictcode/internal/config"
	"github.com/smm-h/strictcode/internal/extract"
	"github.com/smm-h/strictcode/internal/findings"
	"github.com/smm-h/strictcode/internal/options"
	"github.com/smm-h/strictcode/internal/workspace"
)

// Inputs are what one run reads before it extracts anything.
type Inputs struct {
	WS   *workspace.Workspace
	Cfg  *config.Effective
	Opts *options.Resolved
}

// Load reads the workspace at dir, its config file (cfgName, relative to
// the workspace root), and its strictcode options entries.
func Load(dir, cfgName string) (*Inputs, error) {
	ws, err := workspace.Load(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(ws.Root, filepath.FromSlash(cfgName)))
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(ws.Members))
	for _, m := range ws.Members {
		paths = append(paths, m.Path)
	}
	opts, err := options.Load(ws.Root, paths)
	if err != nil {
		return nil, err
	}
	return &Inputs{WS: ws, Cfg: cfg, Opts: opts}, nil
}

// Result is one analysis run's output.
type Result struct {
	// WorkspaceRoot is the absolute workspace root.
	WorkspaceRoot string
	Findings      []findings.Finding
}

// Analyze runs the full pipeline over the workspace at dir. cfgName is the
// config file name resolved relative to dir.
func Analyze(dir, cfgName string) (*Result, error) {
	in, err := Load(dir, cfgName)
	if err != nil {
		return nil, err
	}
	res, err := extract.Extract(in.WS)
	if err != nil {
		return nil, err
	}
	fs, err := checks.Run(in.WS, res, in.Cfg, in.Opts, cfgName, ExecRunner)
	if err != nil {
		return nil, err
	}
	return &Result{WorkspaceRoot: in.WS.Root, Findings: fs}, nil
}
