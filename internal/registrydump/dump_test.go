package registrydump

import (
	"os"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/spec/registryspec"
)

// TestRegistryJSONIsFresh compares the rendered registry dump to the
// committed REGISTRY.json. A mismatch means the registry declarations
// changed without regenerating: run `go run ./cmd/strictcode registry dump`
// and commit with rlsbl commit.
func TestRegistryJSONIsFresh(t *testing.T) {
	want, err := RegistryJSON()
	if err != nil {
		t.Fatalf("RegistryJSON: %v", err)
	}
	got, err := os.ReadFile("../../REGISTRY.json")
	if err != nil {
		t.Fatalf("read committed REGISTRY.json: %v", err)
	}
	if string(got) != string(want) {
		t.Fatal("REGISTRY.json is stale — run `go run ./cmd/strictcode registry dump` and commit with rlsbl commit")
	}
}

// TestMatrixMarkdownIsFresh compares the rendered matrix to the committed
// docs/MATRIX.md.
func TestMatrixMarkdownIsFresh(t *testing.T) {
	want := MatrixMarkdown()
	got, err := os.ReadFile("../../docs/MATRIX.md")
	if err != nil {
		t.Fatalf("read committed docs/MATRIX.md: %v", err)
	}
	if string(got) != string(want) {
		t.Fatal("docs/MATRIX.md is stale — run `go run ./cmd/strictcode matrix gen` and commit with rlsbl commit")
	}
}

// TestCommittedRegistryValidatesAndBinds loads the committed artifact
// through the strictspec-generated reader and cross-checks the typed binding
// against the registry declarations.
func TestCommittedRegistryValidatesAndBinds(t *testing.T) {
	raw, err := os.ReadFile("../../REGISTRY.json")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	doc, diags := registryspec.ValidateBytes(raw, "json")
	if doc == nil {
		t.Fatalf("REGISTRY.json fails its schema: %v", diags)
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

// TestMatrixCoversEveryRuleAndCapability guards against a rendering bug
// silently dropping rows.
func TestMatrixCoversEveryRuleAndCapability(t *testing.T) {
	matrix := string(MatrixMarkdown())
	for _, r := range rules.Rules {
		if !strings.Contains(matrix, "`"+r.ID+"`") {
			t.Errorf("matrix does not mention rule %q", r.ID)
		}
	}
}
