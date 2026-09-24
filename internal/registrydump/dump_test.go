package registrydump

import (
	"os"
	"testing"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/spec/registryspec"
	"github.com/smm-h/strictcode/internal/vocab"
)

// TestRegistryJSONIsFresh compares the rendered registry dump to the
// committed schema/registry.json. A mismatch means the registry declarations
// changed without regenerating: run `go run ./cmd/strictcode registry dump`
// and commit with rlsbl commit.
func TestRegistryJSONIsFresh(t *testing.T) {
	want, err := RegistryJSON()
	if err != nil {
		t.Fatalf("RegistryJSON: %v", err)
	}
	got, err := os.ReadFile("../../schema/registry.json")
	if err != nil {
		t.Fatalf("read committed schema/registry.json: %v", err)
	}
	if string(got) != string(want) {
		t.Fatal("schema/registry.json is stale — run `go run ./cmd/strictcode registry dump` and commit with rlsbl commit")
	}
}

// TestCommittedRegistryValidatesAndBinds loads the committed artifact
// through the strictspec-generated reader and cross-checks the typed binding
// against the registry declarations.
func TestCommittedRegistryValidatesAndBinds(t *testing.T) {
	raw, err := os.ReadFile("../../schema/registry.json")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	doc, diags := registryspec.ValidateBytes(raw, "json")
	if doc == nil {
		t.Fatalf("schema/registry.json fails its schema: %v", diags)
	}
	if len(doc.Rules) != len(rules.Rules) {
		t.Fatalf("bound %d rules, registry declares %d", len(doc.Rules), len(rules.Rules))
	}
	for i, r := range doc.Rules {
		if r.Id != rules.Rules[i].ID {
			t.Errorf("rule %d: bound ID %q, declared %q", i, r.Id, rules.Rules[i].ID)
		}
	}
	if len(doc.Tombstones) != len(rules.Tombstones) {
		t.Fatalf("bound %d tombstones, registry declares %d", len(doc.Tombstones), len(rules.Tombstones))
	}
}

// TestSupportCellsCoverEveryLanguage checks the committed support cells the
// docs site renders the matrix from: every rule that engages a language
// capability carries one cell per language, equal to the Go calculus, and a
// language-independent rule carries none.
func TestSupportCellsCoverEveryLanguage(t *testing.T) {
	raw, err := os.ReadFile("../../schema/registry.json")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	doc, diags := registryspec.ValidateBytes(raw, "json")
	if doc == nil {
		t.Fatalf("schema/registry.json fails its schema: %v", diags)
	}
	for i, r := range doc.Rules {
		decl := rules.Rules[i]
		cells := map[string]string{}
		for _, kv := range r.Support.Entries() {
			status, _ := kv.Value.Field("status")
			s, _ := status.AsString()
			cells[kv.Key] = s
		}
		if r.LanguageIndependent != decl.LanguageIndependent() {
			t.Errorf("%s: language_independent is %v, declared %v", r.Id, r.LanguageIndependent, decl.LanguageIndependent())
		}
		if decl.LanguageIndependent() {
			if len(cells) != 0 {
				t.Errorf("%s: language-independent rule carries %d support cells", r.Id, len(cells))
			}
			continue
		}
		for _, lang := range vocab.Langs {
			want := string(rules.MatrixCell(decl, lang).Status)
			if cells[string(lang)] != want {
				t.Errorf("%s/%s: support cell %q, calculus says %q", r.Id, lang, cells[string(lang)], want)
			}
		}
	}
}
