// Package findings is the output layer: the findings model, the
// schema-valid machine document (self-validated through the strictspec
// findings reader before it is handed to the framework, which validates it
// again against the declared payload schema), the human-readable text
// rendering, and the exit-code rule.
package findings

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/smm-h/strictcli/go/strictcli"
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

// --- The machine document -------------------------------------------------

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

// Document is the strictcode findings document: the analyze command's
// machine payload, carried as the envelope's payload member under --json.
// Its shape is owned by schema/strictspec/findings.schema.toml, which is why
// it keeps its own format_version and version stamps rather than deferring to
// the envelope's framing.
type Document struct {
	// FormatVersion is the strictspec document version stamp, exact-match
	// checked by the findings schema.
	FormatVersion     int           `json:"format_version"`
	StrictcodeVersion string        `json:"strictcode_version"`
	WorkspaceRoot     string        `json:"workspace_root"`
	Findings          []findingJSON `json:"findings"`
}

// Build assembles the findings document (sorted) and self-validates it
// against the findings schema — an invalid document is a bug and a hard
// error, never emitted output. The framework validates the same value again
// against Schema where it writes the envelope; the two duties are different
// authorities over the same document and both are kept.
func Build(version, workspaceRoot string, fs []Finding) (Document, error) {
	Sort(fs)
	doc := Document{
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
		return Document{}, fmt.Errorf("findings: %w", err)
	}
	out = append(out, '\n')
	if _, diags := findingsspec.ValidateBytes(out, "json"); len(diags) != 0 {
		return Document{}, fmt.Errorf("findings: generated document fails the findings schema (tool bug): %v", diags)
	}
	return doc, nil
}

// Schema is the analyze command's declared payload schema (strictcli §19.5),
// the closed-subset statement of the same document
// schema/strictspec/findings.schema.toml governs. The framework enforces it
// where it writes the envelope and --dump-schema publishes it, so a consumer
// has something to generate against without reading the strictspec document.
//
// The target-kind enum is derived from the vocabulary rather than written
// out, because the vocabulary is where node kinds are declared and a second
// hand-kept list could only drift from it.
var Schema = strictcli.SchemaObject(
	map[string]interface{}{
		"format_version":     strictcli.SchemaConst(1),
		"strictcode_version": strictcli.SchemaType("string"),
		"workspace_root":     strictcli.SchemaType("string"),
		"findings": strictcli.SchemaArray(strictcli.SchemaObject(
			map[string]interface{}{
				"rule":     strictcli.SchemaType("string"),
				"severity": strictcli.SchemaEnum("error", "warning"),
				"message":  strictcli.SchemaType("string"),
				"target": strictcli.SchemaObject(
					map[string]interface{}{
						"id":   strictcli.SchemaType("string"),
						"kind": strictcli.SchemaEnum(nodeKindValues()...),
						"file": strictcli.SchemaType("string"),
						"line": strictcli.SchemaType("integer"),
					},
					[]string{"id", "kind", "file", "line"},
					false,
				),
				// Omitted entirely for a finding with no known fix.
				"fix": strictcli.SchemaObject(
					map[string]interface{}{
						"tier":        strictcli.SchemaType("integer"),
						"description": strictcli.SchemaType("string"),
					},
					[]string{"tier", "description"},
					false,
				),
			},
			[]string{"rule", "severity", "message", "target"},
			false,
		)),
	},
	[]string{"format_version", "strictcode_version", "workspace_root", "findings"},
	false,
)

// nodeKindValues is the vocabulary's node kinds as schema enum values.
func nodeKindValues() []interface{} {
	out := make([]interface{}, 0, len(vocab.NodeKinds))
	for _, k := range vocab.NodeKinds {
		out = append(out, string(k))
	}
	return out
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
