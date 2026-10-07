package registrydump

import (
	"fmt"
	"strings"

	"github.com/smm-h/strictcode/internal/options"
	"github.com/smm-h/strictcode/internal/rules"
	"github.com/stricttools/strictcli/go/strictcli"
)

// RulesFormatVersion is the format version of the `registry rules` payload.
const RulesFormatVersion = 1

// RuleOption is one rule's option in the `registry rules` payload.
type RuleOption struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Values  string `json:"values"`
	Default string `json:"default"`
	Scope   string `json:"scope"`
}

// RuleEntry is one rule in the `registry rules` payload.
type RuleEntry struct {
	ID          string     `json:"id"`
	Severity    string     `json:"severity"`
	Description string     `json:"description"`
	Option      RuleOption `json:"option"`
}

// RulesList is the `registry rules` payload: every rule strictcode
// implements, by rule ID, in the registry's order, each with its option.
// rlsbl reads it to learn whether the strictcode on PATH implements the
// rules it requires, by capability rather than by version.
type RulesList struct {
	FormatVersion     int         `json:"format_version"`
	StrictcodeVersion string      `json:"strictcode_version"`
	Rules             []RuleEntry `json:"rules"`
}

// Rules builds the `registry rules` payload.
func Rules(version string) RulesList {
	out := RulesList{
		FormatVersion:     RulesFormatVersion,
		StrictcodeVersion: version,
		Rules:             make([]RuleEntry, 0, len(rules.Rules)),
	}
	for _, r := range rules.Rules {
		out.Rules = append(out.Rules, RuleEntry{
			ID:          r.ID,
			Severity:    string(r.Severity),
			Description: r.Description,
			Option: RuleOption{
				ID:      options.ID(r.ID),
				Subject: r.OptionSubject,
				Values:  options.Ranking(r),
				Default: string(options.Default(r)),
				Scope:   options.Scope(r),
			},
		})
	}
	return out
}

// RenderRules is the human rendering of the payload: one line per rule, its
// ID, its option's default, and its option's ranking.
func RenderRules(list RulesList) string {
	var b strings.Builder
	width := 0
	for _, r := range list.Rules {
		if len(r.ID) > width {
			width = len(r.ID)
		}
	}
	for _, r := range list.Rules {
		fmt.Fprintf(&b, "%-*s  default %-5s  %s\n", width, r.ID, r.Option.Default, r.Option.Values)
	}
	return b.String()
}

// RulesSchema is the declared payload schema of `registry rules`.
var RulesSchema = strictcli.SchemaObject(
	map[string]interface{}{
		"format_version":     strictcli.SchemaConst(RulesFormatVersion),
		"strictcode_version": strictcli.SchemaType("string"),
		"rules": strictcli.SchemaArray(strictcli.SchemaObject(
			map[string]interface{}{
				"id":          strictcli.SchemaType("string"),
				"severity":    strictcli.SchemaEnum("error", "warning"),
				"description": strictcli.SchemaType("string"),
				"option": strictcli.SchemaObject(
					map[string]interface{}{
						"id":      strictcli.SchemaType("string"),
						"subject": strictcli.SchemaType("string"),
						"values":  strictcli.SchemaType("string"),
						"default": strictcli.SchemaEnum("error", "warn", "off"),
						"scope":   strictcli.SchemaEnum("none", "path"),
					},
					[]string{"id", "subject", "values", "default", "scope"},
					false,
				),
			},
			[]string{"id", "severity", "description", "option"},
			false,
		)),
	},
	[]string{"format_version", "strictcode_version", "rules"},
	false,
)
