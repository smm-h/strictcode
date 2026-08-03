package vocab

import (
	"os"
	"testing"

	"github.com/smm-h/strictcode/internal/vocab/gen"
)

// TestGeneratedVocabIsFresh regenerates the vocabulary constants from the
// schema documents and compares them to the committed vocab_gen.go. A
// mismatch means someone edited schema/vocabulary.toml or a profile without
// running `go run ./cmd/vocabgen`.
func TestGeneratedVocabIsFresh(t *testing.T) {
	want, err := gen.Generate("../..")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	got, err := os.ReadFile("vocab_gen.go")
	if err != nil {
		t.Fatalf("read committed file: %v", err)
	}
	if string(got) != string(want) {
		t.Fatal("internal/vocab/vocab_gen.go is stale — run `go run ./cmd/vocabgen` and commit with rlsbl commit")
	}
}
