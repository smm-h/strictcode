// Command vocabgen regenerates internal/vocab/vocab_gen.go from
// schema/vocabulary.toml and schema/profiles/. Run from the repository root:
//
//	go run ./cmd/vocabgen
//
// Freshness is enforced by TestGeneratedVocabIsFresh; commit the regenerated
// file with `rlsbl commit` (it is machine-generated).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/smm-h/strictcode/internal/vocab/gen"
)

func main() {
	root, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}
	out, err := gen.Generate(root)
	if err != nil {
		fatal(err)
	}
	target := filepath.Join(root, "internal", "vocab", "vocab_gen.go")
	if err := os.WriteFile(target, out, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s\n", target)
}

// findRepoRoot walks upward from the working directory to the directory
// containing schema/vocabulary.toml.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "schema", "vocabulary.toml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("vocabgen: schema/vocabulary.toml not found above the working directory")
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
