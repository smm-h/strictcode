// Package fixture builds on-disk workspace fixtures for tests: a map of
// relative paths to file contents, written into a temp directory. Used by
// the workspace, extractor, and check test suites (including the lessons
// register), where each scenario is a small literal workspace.
package fixture

import (
	"os"
	"path/filepath"
	"testing"
)

// Write materializes files (relative slash paths -> contents) under a fresh
// temp directory and returns its path. Parent directories are created as
// needed. An entry whose path ends in "/" creates a bare directory.
func Write(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if rel[len(rel)-1] == '/' {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("fixture: %v", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	return root
}
