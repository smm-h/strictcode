// Round-trip tests: the committed schema documents must validate through the
// generated strictspec readers, and the typed bindings must carry the content
// the rest of the codebase (generator, registry) depends on.
package spec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smm-h/strictcode/internal/spec/profilespec"
	"github.com/smm-h/strictcode/internal/spec/vocabspec"
)

func repoPath(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join("..", "..", rel)
}

func readFile(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(repoPath(t, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return b
}

func TestVocabularyDocumentValidates(t *testing.T) {
	doc := readFile(t, "schema/vocabulary.toml")
	v, diags := vocabspec.ValidateBytes(doc, "toml")
	if len(diags) != 0 {
		t.Fatalf("vocabulary.toml has diagnostics: %v", diags)
	}
	if v == nil {
		t.Fatal("nil typed binding for valid vocabulary.toml")
	}
	if len(v.NodeKinds) == 0 || len(v.RowKinds) == 0 || len(v.Capabilities) == 0 || len(v.Layers) == 0 {
		t.Fatalf("typed binding missing content: %d node kinds, %d row kinds, %d capabilities, %d layers",
			len(v.NodeKinds), len(v.RowKinds), len(v.Capabilities), len(v.Layers))
	}
	// Spot-check entries the generator and registry depend on.
	foundModule := false
	for _, nk := range v.NodeKinds {
		if nk.Id == "module" {
			foundModule = true
		}
	}
	if !foundModule {
		t.Fatal("vocabulary binding lacks node kind 'module'")
	}
}

func TestProfileDocumentsValidate(t *testing.T) {
	profiles := map[string]string{
		"schema/profiles/python.toml": "py",
		"schema/profiles/go.toml":     "go",
		"schema/profiles/ts-js.toml":  "ts",
	}
	for rel, lang := range profiles {
		doc := readFile(t, rel)
		p, diags := profilespec.ValidateBytes(doc, "toml")
		if len(diags) != 0 {
			t.Fatalf("%s has diagnostics: %v", rel, diags)
		}
		if p == nil {
			t.Fatalf("nil typed binding for %s", rel)
		}
		if p.Language != lang {
			t.Fatalf("%s: language = %q, want %q", rel, p.Language, lang)
		}
		if len(p.Constructs) == 0 {
			t.Fatalf("%s: no constructs bound", rel)
		}
		if len(p.Capabilities.Entries()) == 0 {
			t.Fatalf("%s: no capability statuses bound", rel)
		}
	}
}

func TestProfilesDeclareEveryVocabularyCapability(t *testing.T) {
	// SPEC.md section 5: vocabulary and profiles are closed sets; a profile must
	// declare a status for every capability the vocabulary defines, and must not
	// declare unknown capabilities.
	vdoc := readFile(t, "schema/vocabulary.toml")
	v, diags := vocabspec.ValidateBytes(vdoc, "toml")
	if v == nil {
		t.Fatalf("vocabulary invalid: %v", diags)
	}
	vocab := map[string]bool{}
	for _, c := range v.Capabilities {
		vocab[c.Id] = true
	}
	for _, rel := range []string{
		"schema/profiles/python.toml",
		"schema/profiles/go.toml",
		"schema/profiles/ts-js.toml",
	} {
		p, diags := profilespec.ValidateBytes(readFile(t, rel), "toml")
		if p == nil {
			t.Fatalf("%s invalid: %v", rel, diags)
		}
		declared := map[string]bool{}
		for _, kv := range p.Capabilities.Entries() {
			declared[kv.Key] = true
		}
		for id := range vocab {
			if !declared[id] {
				t.Errorf("%s: missing status for capability %q", rel, id)
			}
		}
		for id := range declared {
			if !vocab[id] {
				t.Errorf("%s: declares unknown capability %q", rel, id)
			}
		}
	}
}
