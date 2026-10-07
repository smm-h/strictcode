package certificate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/fixture"
)

func cert(claims ...map[string]interface{}) string {
	doc := map[string]interface{}{
		"certificate_format_version": 1,
		"schema_id":                  "widget-config",
		"old_format_version":         1,
		"new_format_version":         2,
		"corpus":                     map[string]interface{}{"declared_glob": "corpus/*", "resolved_file_count": 3, "content_hash": "abc"},
		"claims":                     claims,
		"strictspec_release":         "0.1.0",
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

func claimJSON(class, grade, statement string) map[string]interface{} {
	return map[string]interface{}{"kind": class, "grade": grade, "statement": statement}
}

const adjudicationHeader = "format_version = 1\nschema_id = \"widget-config\"\nold_format_version = 1\nnew_format_version = 2\n"

func adjudication(class, scope string) string {
	return "\n[[adjudications]]\nclaim_kind = \"" + class + "\"\nscope = \"" + scope + "\"\njustification = \"no documents exist at rest\"\nauthor = \"maintainer\"\ndate = 2026-07-27\n"
}

func evaluate(t *testing.T, files map[string]string, adj string) ([]Blocker, error) {
	t.Helper()
	root := fixture.Write(t, files)
	return Evaluate(root, "cert.json", adj)
}

func TestTheAdjudicationSchemaCompiles(t *testing.T) {
	if adjudicationValidator() == nil {
		t.Fatal("no validator")
	}
}

func TestGreenGradesPass(t *testing.T) {
	blockers, err := evaluate(t, map[string]string{"cert.json": cert(
		claimJSON("flip-scan", "corpus-supported", "every document valid at N stays valid at N+1"),
		claimJSON("down-taxonomy", "proven", "s"),
	)}, "")
	if err != nil || len(blockers) != 0 {
		t.Fatalf("green grades blocked: %v %v", blockers, err)
	}
}

func TestViolatedBlocksNamingItsWitness(t *testing.T) {
	violated := claimJSON("flip-scan", "violated", "narrowing without a bump")
	violated["counterexamples"] = []map[string]interface{}{{"document_path": "corpus/bad.toml", "diagnostics": []interface{}{}}}
	blockers, err := evaluate(t, map[string]string{"cert.json": cert(
		claimJSON("flip-scan", "corpus-supported", "a"), violated,
	)}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(blockers) != 1 || !strings.Contains(blockers[0].Reason, "violated") || !strings.Contains(blockers[0].Reason, "corpus/bad.toml") {
		t.Fatalf("blockers: %+v", blockers)
	}
	if blockers[0].File != "cert.json" {
		t.Errorf("blocker file %q", blockers[0].File)
	}
}

func TestUnsupportedWithoutAdjudicationBlocks(t *testing.T) {
	blockers, err := evaluate(t, map[string]string{"cert.json": cert(claimJSON("flip-scan", "no-corpus", "s"))}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(blockers) != 1 || !strings.Contains(blockers[0].Reason, "no adjudication file") {
		t.Fatalf("blockers: %+v", blockers)
	}
}

func TestAdjudicationDischargesAndDanglingBlocks(t *testing.T) {
	files := map[string]string{
		"cert.json": cert(claimJSON("flip-scan", "no-corpus", "greenfield claim")),
		"adj.toml":  adjudicationHeader + adjudication("flip-scan", "greenfield claim"),
	}
	if blockers, err := evaluate(t, files, "adj.toml"); err != nil || len(blockers) != 0 {
		t.Fatalf("a discharged claim blocked: %v %v", blockers, err)
	}
	files["adj.toml"] += adjudication("down-taxonomy", "nonexistent")
	blockers, err := evaluate(t, files, "adj.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(blockers) != 1 || !strings.Contains(blockers[0].Reason, "dangling") || blockers[0].File != "adj.toml" {
		t.Fatalf("dangling entry: %+v", blockers)
	}
}

func TestUnreadableFilesAreErrors(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		adj   string
		want  string
	}{
		"missing certificate":   {map[string]string{}, "", "does not exist"},
		"malformed certificate": {map[string]string{"cert.json": "{ not json"}, "", "not valid JSON"},
		"no claims":             {map[string]string{"cert.json": `{"schema_id": "x"}`}, "", "claims"},
		"missing adjudication":  {map[string]string{"cert.json": cert(claimJSON("flip-scan", "no-corpus", "s"))}, "missing.toml", "does not exist"},
		"invalid adjudication": {map[string]string{
			"cert.json": cert(claimJSON("flip-scan", "no-corpus", "s")),
			"adj.toml":  adjudicationHeader + "\n[[adjudications]]\nclaim_kind = \"flip-scan\"\nscope = \"s\"\njustification = \"j\"\n",
		}, "adj.toml", "fails its schema"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := evaluate(t, c.files, c.adj)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error naming %q, got %v", c.want, err)
			}
		})
	}
}
