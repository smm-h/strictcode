// Package findings is the output layer: the findings model, the
// schema-valid JSON rendering (self-validated through the strictspec
// findings reader before being emitted), the human-readable text rendering,
// and the exit-code rule.
package findings

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/spec/findingsspec"
	"github.com/smm-h/strictcode/internal/vocab"
)

// Target is what a finding points at: a node plus its site.
type Target struct {
	// ID is the serialized qualified node ID (schema/SPEC.md section 2).
	ID string
	// Kind is the node kind.
	Kind vocab.NodeKind
	// File is the workspace-root-relative file path.
	File string
	// Line is the 1-based line, derived from the byte span at output time.
	Line int
}

// Fix describes the offered fix, when one exists.
type Fix struct {
	Tier        int
	Description string
}

// Finding is one diagnosis instance.
type Finding struct {
	Rule     string
	Severity rules.Severity
	Message  string
	Target   Target
	Fix      *Fix
}

// Sort orders findings deterministically: file, line, rule, target ID,
// message.
func Sort(fs []Finding) {
	sort.Slice(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Target.File != b.Target.File {
			return a.Target.File < b.Target.File
		}
		if a.Target.Line != b.Target.Line {
			return a.Target.Line < b.Target.Line
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Target.ID != b.Target.ID {
			return a.Target.ID < b.Target.ID
		}
		return a.Message < b.Message
	})
}

// FailRun reports whether the findings set fails the run (nonzero exit):
// any error-severity finding does; warnings alone do not.
func FailRun(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == rules.SeverityError {
			return true
		}
	}
	return false
}

// --- JSON rendering -------------------------------------------------------

type targetJSON struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type fixJSON struct {
	Tier        int    `json:"tier"`
	Description string `json:"description"`
}

type findingJSON struct {
	Rule     string     `json:"rule"`
	Severity string     `json:"severity"`
	Message  string     `json:"message"`
	Target   targetJSON `json:"target"`
	Fix      *fixJSON   `json:"fix,omitempty"`
}

type documentJSON struct {
	// FormatVersion is the strictspec document version stamp, exact-match
	// checked by the findings schema.
	FormatVersion     int           `json:"format_version"`
	StrictcodeVersion string        `json:"strictcode_version"`
	WorkspaceRoot     string        `json:"workspace_root"`
	Findings          []findingJSON `json:"findings"`
}

// RenderJSON renders the strictcode findings document (sorted, indented,
// trailing newline) and self-validates it against the findings schema — an
// invalid document is a bug and a hard error, never emitted output.
func RenderJSON(version, workspaceRoot string, fs []Finding) ([]byte, error) {
	Sort(fs)
	doc := documentJSON{
		FormatVersion:     1,
		StrictcodeVersion: version,
		WorkspaceRoot:     workspaceRoot,
		Findings:          make([]findingJSON, 0, len(fs)),
	}
	for _, f := range fs {
		fj := findingJSON{
			Rule:     f.Rule,
			Severity: string(f.Severity),
			Message:  f.Message,
			Target: targetJSON{
				ID:   f.Target.ID,
				Kind: string(f.Target.Kind),
				File: f.Target.File,
				Line: f.Target.Line,
			},
		}
		if f.Fix != nil {
			fj.Fix = &fixJSON{Tier: f.Fix.Tier, Description: f.Fix.Description}
		}
		doc.Findings = append(doc.Findings, fj)
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("findings: %w", err)
	}
	out = append(out, '\n')
	if _, diags := findingsspec.ValidateBytes(out, "json"); len(diags) != 0 {
		return nil, fmt.Errorf("findings: generated output fails the findings schema (tool bug): %v", diags)
	}
	return out, nil
}

// RenderText renders the human-readable report: one line per finding
// (file:line: severity: message [rule]), then a summary.
func RenderText(fs []Finding) string {
	Sort(fs)
	var b strings.Builder
	errors, warnings := 0, 0
	for _, f := range fs {
		fmt.Fprintf(&b, "%s:%d: %s: %s [%s]\n", f.Target.File, f.Target.Line, f.Severity, f.Message, f.Rule)
		if f.Severity == rules.SeverityError {
			errors++
		} else {
			warnings++
		}
		if f.Fix != nil {
			fmt.Fprintf(&b, "    fix (tier %d): %s\n", f.Fix.Tier, f.Fix.Description)
		}
	}
	if len(fs) == 0 {
		b.WriteString("no findings\n")
	} else {
		fmt.Fprintf(&b, "\n%d finding(s): %d error(s), %d warning(s)\n", len(fs), errors, warnings)
	}
	return b.String()
}
