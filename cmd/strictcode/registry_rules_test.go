package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/smm-h/strictcode/internal/rules"
)

// `registry rules --json` lists every rule strictcode implements, by rule ID
// and in the registry's order, each with its option: the list rlsbl's
// strictcode check reads to decide whether the strictcode on PATH has the
// rules it requires.
func TestRegistryRulesListsEveryRule(t *testing.T) {
	res := newApp().Test([]string{"registry", "rules", "--json"})
	if res.ExitCode != 0 {
		t.Fatalf("registry rules --json exited %d: %s", res.ExitCode, res.Stderr)
	}
	var env struct {
		Payload struct {
			FormatVersion     int    `json:"format_version"`
			StrictcodeVersion string `json:"strictcode_version"`
			Rules             []struct {
				ID       string `json:"id"`
				Severity string `json:"severity"`
				Option   struct {
					ID      string `json:"id"`
					Values  string `json:"values"`
					Default string `json:"default"`
					Scope   string `json:"scope"`
					Subject string `json:"subject"`
				} `json:"option"`
			} `json:"rules"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, res.Stdout)
	}
	if env.Payload.FormatVersion != 1 || env.Payload.StrictcodeVersion == "" {
		t.Errorf("payload header: %+v", env.Payload)
	}
	if len(env.Payload.Rules) != len(rules.Rules) {
		t.Fatalf("listed %d rules, the registry has %d", len(env.Payload.Rules), len(rules.Rules))
	}
	for i, r := range rules.Rules {
		got := env.Payload.Rules[i]
		if got.ID != r.ID {
			t.Errorf("rule %d is %q, want %q", i, got.ID, r.ID)
		}
		if got.Option.ID != "strictcode:"+r.ID {
			t.Errorf("%s: option %q", r.ID, got.Option.ID)
		}
		if got.Option.Subject != r.OptionSubject {
			t.Errorf("%s: subject %q, want %q", r.ID, got.Option.Subject, r.OptionSubject)
		}
	}
	byID := map[string]string{}
	for _, r := range env.Payload.Rules {
		byID[r.ID] = r.Option.Default + " " + r.Option.Scope
	}
	for id, want := range map[string]string{
		"lint":                   "off path",
		"format":                 "off path",
		"type-check":             "off path",
		"strictspec-certificate": "off none",
		"deps-unused":            "error none",
		"dead-modules":           "warn none",
	} {
		if byID[id] != want {
			t.Errorf("%s: default and scope %q, want %q", id, byID[id], want)
		}
	}
}

func TestRegistryRulesHumanModeListsRuleIDs(t *testing.T) {
	res := newApp().Test([]string{"registry", "rules"})
	if res.ExitCode != 0 {
		t.Fatalf("registry rules exited %d: %s", res.ExitCode, res.Stderr)
	}
	for _, r := range rules.Rules {
		if !strings.Contains(res.Stdout, r.ID) {
			t.Errorf("human listing omits %s:\n%s", r.ID, res.Stdout)
		}
	}
}
