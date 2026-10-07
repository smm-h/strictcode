// Package certificate evaluates a strictspec diff certificate for the
// strictspec-certificate rule (strictspec spec/appendix-certificates.md).
//
// Grades (Part A, decision 25):
//
//   - violated: a corpus document is the counterexample; it blocks.
//   - corpus-supported: no counterexample in the declared corpus; it passes.
//   - proven: reserved for the future analyzer; it passes.
//   - any other grade is unsupported, and blocks unless an entry of the
//     declared adjudication file (Part B) discharges it.
//
// An adjudication entry discharges an unsupported claim when its claim_kind
// value equals the claim's class (the certificate's "kind" key) and its scope
// equals the claim's statement; an
// entry discharging no unsupported claim is dangling and blocks too.
//
// The certificate carries certificate_format_version rather than a document
// format_version, so it is read as plain JSON and only the fields the rule
// consumes are checked. The adjudication file is a gated strictspec document,
// validated against the embedded adjudication schema.
package certificate

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/stricttools/strictspec/go/strictspec"
)

//go:embed adjudication.schema.toml
var adjudicationSchema string

var (
	adjudicationOnce    sync.Once
	adjudicationProgram *strictspec.Program
)

func adjudicationValidator() *strictspec.Program {
	adjudicationOnce.Do(func() {
		p, err := strictspec.CompileEmbedded(map[string]string{"adjudication.schema.toml": adjudicationSchema}, "adjudication.schema.toml")
		if err != nil {
			// The schema ships inside strictcode; failing to compile is a
			// strictcode defect, which a test holds.
			panic("certificate: the adjudication schema does not compile: " + err.Error())
		}
		adjudicationProgram = p
	})
	return adjudicationProgram
}

// Blocker is one reason the certificate blocks, attributed to the file it
// concerns (workspace-root-relative).
type Blocker struct {
	File   string
	Reason string
}

// claim is one entry of the certificate's claims array.
type claim struct {
	Class           string `json:"kind"`
	Grade           string `json:"grade"`
	Statement       string `json:"statement"`
	Counterexamples []struct {
		DocumentPath string `json:"document_path"`
	} `json:"counterexamples"`
}

func (c claim) label(i int) string {
	if strings.TrimSpace(c.Statement) != "" {
		return c.Statement
	}
	return fmt.Sprintf("claim #%d (%s)", i, c.Class)
}

// adjudicationEntry is one [[adjudications]] entry.
type adjudicationEntry struct {
	ClaimClass string
	Scope      string
}

// Evaluate reads the certificate (and the adjudication file, when one is
// declared) at the workspace-root-relative paths under root and returns
// every blocker. A missing or unreadable file, a certificate that is not a
// JSON object with a claims array, and an adjudication file that fails its
// schema are errors: a declared certificate must be readable.
func Evaluate(root, certificatePath, adjudicationPath string) ([]Blocker, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(certificatePath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the certificate %s does not exist: produce it with `strictspec diff`, or switch strictcode:strictspec-certificate off", certificatePath)
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Claims *[]json.RawMessage `json:"claims"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("the certificate %s is not valid JSON: %w", certificatePath, err)
	}
	if doc.Claims == nil {
		return nil, fmt.Errorf("the certificate %s has no claims array", certificatePath)
	}
	claims := make([]claim, 0, len(*doc.Claims))
	for i, rawClaim := range *doc.Claims {
		var c claim
		if err := json.Unmarshal(rawClaim, &c); err != nil {
			return nil, fmt.Errorf("the certificate %s: claims[%d] is not a claim object: %w", certificatePath, i, err)
		}
		claims = append(claims, c)
	}

	var blockers []Blocker
	var unsupported []int
	for i, c := range claims {
		switch c.Grade {
		case "violated":
			var witnesses []string
			for _, ce := range c.Counterexamples {
				if ce.DocumentPath != "" {
					witnesses = append(witnesses, ce.DocumentPath)
				}
			}
			reason := fmt.Sprintf("claim '%s' is violated", c.label(i))
			if len(witnesses) != 0 {
				reason += " (witness: " + strings.Join(witnesses, ", ") + ")"
			}
			blockers = append(blockers, Blocker{File: certificatePath, Reason: reason})
		case "corpus-supported", "proven":
		default:
			unsupported = append(unsupported, i)
		}
	}

	if adjudicationPath == "" {
		for _, i := range unsupported {
			blockers = append(blockers, Blocker{File: certificatePath, Reason: fmt.Sprintf(
				"claim '%s' is unsupported (grade %q) and no adjudication file is declared to discharge it",
				claims[i].label(i), claims[i].Grade)})
		}
		return blockers, nil
	}
	entries, err := loadAdjudications(root, adjudicationPath)
	if err != nil {
		return nil, err
	}
	matched := map[int]bool{}
	for _, i := range unsupported {
		c := claims[i]
		found := -1
		for j, e := range entries {
			if e.ClaimClass == c.Class && e.Scope == c.Statement {
				found = j
				break
			}
		}
		if found < 0 {
			blockers = append(blockers, Blocker{File: certificatePath, Reason: fmt.Sprintf(
				"claim '%s' is unsupported and no adjudication entry discharges it (needs claim_kind %q, scope %q)",
				c.label(i), c.Class, c.Statement)})
			continue
		}
		matched[found] = true
	}
	for j, e := range entries {
		if !matched[j] {
			blockers = append(blockers, Blocker{File: adjudicationPath, Reason: fmt.Sprintf(
				"adjudication entry #%d (claim_kind %q, scope %q) matches no unsupported claim in the certificate (dangling)",
				j, e.ClaimClass, e.Scope)})
		}
	}
	return blockers, nil
}

// loadAdjudications reads and validates the adjudication file.
func loadAdjudications(root, path string) ([]adjudicationEntry, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the adjudication file %s does not exist", path)
	}
	if err != nil {
		return nil, err
	}
	res := adjudicationValidator().Validate(raw, "toml")
	if !res.Valid {
		lines := []string{fmt.Sprintf("the adjudication file %s fails its schema:", path)}
		for _, d := range res.Diagnostics {
			lines = append(lines, fmt.Sprintf("  %s at %s: %s", d.Code, d.Path, d.Message))
		}
		return nil, errors.New(strings.Join(lines, "\n"))
	}
	doc, err := strictspec.LoadValue(raw, "toml")
	if err != nil {
		return nil, fmt.Errorf("the adjudication file %s: %w", path, err)
	}
	items, _ := doc.Field("adjudications")
	var out []adjudicationEntry
	for _, item := range items.Items() {
		var e adjudicationEntry
		if v, ok := item.Field("claim_kind"); ok {
			e.ClaimClass, _ = v.AsString()
		}
		if v, ok := item.Field("scope"); ok {
			e.Scope, _ = v.AsString()
		}
		out = append(out, e)
	}
	return out, nil
}
