// Package registrydump renders the committed registry artifact,
// schema/registry.json: the machine-readable rule registry the release diff
// classifies version bumps from, carrying each rule's computed support-matrix
// cells so the docs site renders the matrix from data instead of repeating the
// calculus.
//
// The rendering is deterministic. RegistryJSON self-validates its output
// through the strictspec-generated reader before returning, so an invalid dump
// is a hard error, never a written artifact.
package registrydump

import (
	"encoding/json"
	"fmt"

	"github.com/smm-h/strictcode/internal/rules"
	"github.com/smm-h/strictcode/internal/spec/registryspec"
	"github.com/smm-h/strictcode/internal/vocab"
)

// FormatVersion is the registry dump's document format_version, checked
// exactly by the strictspec schema (schema/strictspec/registry.schema.toml).
const FormatVersion = 2

type ruleJSON struct {
	ID            string            `json:"id"`
	Severity      string            `json:"severity"`
	Description   string            `json:"description"`
	Requires      []string          `json:"requires"`
	Uses          []string          `json:"uses"`
	Groups        []string          `json:"groups"`
	Suppression   string            `json:"suppression"`
	FixTier       int               `json:"fix_tier"`
	PlannedFixes  []plannedFixJSON  `json:"planned_fixes"`
	NotApplicable map[string]string `json:"not_applicable"`
	// LanguageIndependent rules engage no language capability, so Support is
	// empty for them: the dump makes no per-language claim it cannot back.
	LanguageIndependent bool                `json:"language_independent"`
	Support             map[string]cellJSON `json:"support"`
}

type cellJSON struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type plannedFixJSON struct {
	Tier        int    `json:"tier"`
	Description string `json:"description"`
}

type tombstoneJSON struct {
	ID         string   `json:"id"`
	RetiredIn  string   `json:"retired_in"`
	Reason     string   `json:"reason"`
	ReplacedBy []string `json:"replaced_by"`
	Migration  string   `json:"migration"`
}

type registryJSON struct {
	FormatVersion int                 `json:"format_version"`
	Rules         []ruleJSON          `json:"rules"`
	Groups        map[string][]string `json:"groups"`
	Tombstones    []tombstoneJSON     `json:"tombstones"`
}

// RegistryJSON renders the registry as deterministic, indented JSON with a
// trailing newline, self-validated against the strictspec registry schema.
func RegistryJSON() ([]byte, error) {
	doc := registryJSON{
		FormatVersion: FormatVersion,
		Rules:         make([]ruleJSON, 0, len(rules.Rules)),
		Groups:        map[string][]string{},
		Tombstones:    make([]tombstoneJSON, 0, len(rules.Tombstones)),
	}
	for _, r := range rules.Rules {
		rj := ruleJSON{
			ID:            r.ID,
			Severity:      string(r.Severity),
			Description:   r.Description,
			Requires:      capStrings(r.Requires),
			Uses:          capStrings(r.Uses),
			Groups:        emptyNotNil(r.Groups),
			Suppression:   string(r.Suppression),
			FixTier:       int(r.FixTier),
			PlannedFixes:  make([]plannedFixJSON, 0, len(r.PlannedFixes)),
			NotApplicable:       map[string]string{},
			LanguageIndependent: r.LanguageIndependent(),
			Support:             map[string]cellJSON{},
		}
		if !r.LanguageIndependent() {
			for _, lang := range vocab.Langs {
				c := rules.MatrixCell(r, lang)
				rj.Support[string(lang)] = cellJSON{Status: string(c.Status), Reason: c.Reason}
			}
		}
		for _, pf := range r.PlannedFixes {
			rj.PlannedFixes = append(rj.PlannedFixes, plannedFixJSON{Tier: int(pf.Tier), Description: pf.Description})
		}
		for lang, reason := range r.NotApplicable {
			rj.NotApplicable[string(lang)] = reason
		}
		doc.Rules = append(doc.Rules, rj)
	}
	for name, members := range rules.Groups {
		doc.Groups[name] = emptyNotNil(members)
	}
	for _, t := range rules.Tombstones {
		doc.Tombstones = append(doc.Tombstones, tombstoneJSON{
			ID: t.ID, RetiredIn: t.RetiredIn, Reason: t.Reason,
			ReplacedBy: emptyNotNil(t.ReplacedBy), Migration: t.Migration,
		})
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("registrydump: %w", err)
	}
	out = append(out, '\n')

	// Self-check: the artifact must validate against its own schema.
	if _, diags := registryspec.ValidateBytes(out, "json"); len(diags) != 0 {
		return nil, fmt.Errorf("registrydump: generated schema/registry.json fails its schema: %v", diags)
	}
	return out, nil
}

func capStrings(caps []vocab.Capability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	return out
}

func emptyNotNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
