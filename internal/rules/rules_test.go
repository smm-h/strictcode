package rules

import (
	"regexp"
	"testing"

	"github.com/smm-h/strictcode/internal/vocab"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

func TestRegistryHasFourteenMintedRules(t *testing.T) {
	if len(Rules) != 14 {
		t.Fatalf("registry has %d rules, CATALOG.md mints 14", len(Rules))
	}
}

func TestRuleIDsAreValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules {
		if !idPattern.MatchString(r.ID) {
			t.Errorf("rule ID %q is not flat lowercase-hyphenated", r.ID)
		}
		if seen[r.ID] {
			t.Errorf("duplicate rule ID %q", r.ID)
		}
		seen[r.ID] = true
	}
	for _, tomb := range Tombstones {
		if seen[tomb.ID] {
			t.Errorf("tombstoned ID %q is also a live rule — IDs are never reused", tomb.ID)
		}
	}
}

func TestRuleDeclarationsAreComplete(t *testing.T) {
	validSeverity := map[Severity]bool{SeverityError: true, SeverityWarning: true}
	validShape := map[SuppressionShape]bool{
		SuppressNone: true, SuppressPath: true, SuppressProjectDep: true,
		SuppressMemberSet: true, SuppressMember: true,
	}
	for _, r := range Rules {
		if r.Description == "" {
			t.Errorf("%s: empty description", r.ID)
		}
		if !validSeverity[r.Severity] {
			t.Errorf("%s: invalid severity %q", r.ID, r.Severity)
		}
		if !validShape[r.Suppression] {
			t.Errorf("%s: invalid suppression shape %q", r.ID, r.Suppression)
		}
		switch r.FixTier {
		case Tier1, Tier3:
			// Tier 3 = detection-first; tier 1 = a whitelisted transform
			// shipped (unreachable-code as of round 3). Tier 2 requires the
			// consent flow, which has not shipped.
		default:
			t.Errorf("%s: ships at tier %d; only 1 (whitelisted transform) and 3 (detection) exist today", r.ID, r.FixTier)
		}
		if r.ID == "unreachable-code" && r.FixTier != Tier1 {
			t.Errorf("unreachable-code must offer the tier-1 removal transform")
		}
		for _, pf := range r.PlannedFixes {
			if pf.Tier < Tier1 || pf.Tier > Tier2 {
				t.Errorf("%s: planned fix tier %d out of range (only 1 and 2 can be planned beyond the shipped 3)", r.ID, pf.Tier)
			}
			if pf.Description == "" {
				t.Errorf("%s: planned fix without description", r.ID)
			}
		}
	}
}

func TestRuleCapabilitiesExistAndAreDisjoint(t *testing.T) {
	known := map[vocab.Capability]bool{}
	for _, c := range vocab.Capabilities {
		known[c] = true
	}
	for _, r := range Rules {
		inRequires := map[vocab.Capability]bool{}
		for _, c := range r.Requires {
			if !known[c] {
				t.Errorf("%s: requires unknown capability %q", r.ID, c)
			}
			if inRequires[c] {
				t.Errorf("%s: duplicate required capability %q", r.ID, c)
			}
			inRequires[c] = true
		}
		for _, c := range r.Uses {
			if !known[c] {
				t.Errorf("%s: uses unknown capability %q", r.ID, c)
			}
			if inRequires[c] {
				t.Errorf("%s: capability %q in both requires and uses", r.ID, c)
			}
		}
	}
}

func TestNotApplicableOverridesCarryReasons(t *testing.T) {
	validLang := map[vocab.Lang]bool{vocab.LangPy: true, vocab.LangGo: true, vocab.LangTS: true}
	for _, r := range Rules {
		for lang, reason := range r.NotApplicable {
			if !validLang[lang] {
				t.Errorf("%s: not_applicable override for unknown language %q", r.ID, lang)
			}
			if reason == "" {
				t.Errorf("%s: not_applicable override for %s without a reason", r.ID, lang)
			}
		}
	}
}

func TestGroupsAreConsistent(t *testing.T) {
	// Every group member is a live rule that declares the group, and every
	// declared group membership appears in the group table.
	for name, members := range Groups {
		for _, id := range members {
			r, ok := ByID(id)
			if !ok {
				t.Errorf("group %q references unknown rule %q", name, id)
				continue
			}
			found := false
			for _, g := range r.Groups {
				if g == name {
					found = true
				}
			}
			if !found {
				t.Errorf("group %q lists %q but the rule does not declare it", name, id)
			}
		}
	}
	for _, r := range Rules {
		for _, g := range r.Groups {
			members, ok := Groups[g]
			if !ok {
				t.Errorf("%s declares unknown group %q", r.ID, g)
				continue
			}
			found := false
			for _, id := range members {
				if id == r.ID {
					found = true
				}
			}
			if !found {
				t.Errorf("%s declares group %q but the group table does not list it", r.ID, g)
			}
		}
	}
}

func TestGroupLibraryMembership(t *testing.T) {
	want := []string{
		"library-forbidden-imports",
		"library-stdout",
		"library-direct-logging",
		"library-entry-point",
	}
	got := Groups["library"]
	if len(got) != len(want) {
		t.Fatalf("group:library has %d members, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("group:library[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTombstonesAreWellFormed(t *testing.T) {
	for _, tomb := range Tombstones {
		if !idPattern.MatchString(tomb.ID) {
			t.Errorf("tombstone ID %q is not flat lowercase-hyphenated", tomb.ID)
		}
		if tomb.RetiredIn == "" || tomb.Reason == "" || tomb.Migration == "" {
			t.Errorf("tombstone %q missing retired_in, reason, or migration", tomb.ID)
		}
		for _, succ := range tomb.ReplacedBy {
			if _, ok := ByID(succ); !ok {
				t.Errorf("tombstone %q names unknown successor %q", tomb.ID, succ)
			}
		}
	}
}

func TestMatrixImportCyclesGoNotApplicable(t *testing.T) {
	r, ok := ByID("import-cycles")
	if !ok {
		t.Fatal("import-cycles not in registry")
	}
	cell := MatrixCell(r, vocab.LangGo)
	if cell.Status != CellNotApplicable {
		t.Fatalf("import-cycles on go = %q, want n/a (lesson 20)", cell.Status)
	}
	if cell.Reason == "" {
		t.Fatal("n/a cell without reason")
	}
	for _, lang := range []vocab.Lang{vocab.LangPy, vocab.LangTS} {
		if c := MatrixCell(r, lang); c.Status != CellSupported {
			t.Errorf("import-cycles on %s = %q, want supported (import-graph extractors landed)", lang, c.Status)
		}
	}
}

func TestMatrixUnreachableCode(t *testing.T) {
	r, _ := ByID("unreachable-code")
	if c := MatrixCell(r, vocab.LangGo); c.Status != CellNotApplicable {
		t.Errorf("unreachable-code on go = %q, want n/a (go vet owns it)", c.Status)
	}
	if c := MatrixCell(r, vocab.LangPy); c.Status != CellSupported {
		t.Errorf("unreachable-code on py = %q, want supported (round 3)", c.Status)
	}
	// TS has no explicit override; the profile capability is planned.
	if c := MatrixCell(r, vocab.LangTS); c.Status != CellPlanned {
		t.Errorf("unreachable-code on ts = %q, want planned", c.Status)
	}
}

func TestMatrixLibraryDirectLoggingPythonOnly(t *testing.T) {
	r, _ := ByID("library-direct-logging")
	if c := MatrixCell(r, vocab.LangGo); c.Status != CellNotApplicable {
		t.Errorf("library-direct-logging on go = %q, want n/a", c.Status)
	}
	if c := MatrixCell(r, vocab.LangTS); c.Status != CellNotApplicable {
		t.Errorf("library-direct-logging on ts = %q, want n/a", c.Status)
	}
	if c := MatrixCell(r, vocab.LangPy); c.Status != CellSupported {
		t.Errorf("library-direct-logging on py = %q, want supported (round 3)", c.Status)
	}
}

func TestMatrixUsesNeverBlocks(t *testing.T) {
	// deps-unused uses import-attr-guarded, which is not-applicable on Go —
	// the cell must still be supported (not n/a): uses never blocks.
	r, _ := ByID("deps-unused")
	if c := MatrixCell(r, vocab.LangGo); c.Status != CellSupported {
		t.Errorf("deps-unused on go = %q, want supported (uses-capability n/a must not block)", c.Status)
	}
}

func TestLanguageIndependentRules(t *testing.T) {
	r, _ := ByID("stale-suppression")
	if !r.LanguageIndependent() {
		t.Fatal("stale-suppression must be language-independent")
	}
	for _, other := range Rules {
		if other.ID != "stale-suppression" && other.LanguageIndependent() {
			t.Errorf("%s is unexpectedly language-independent", other.ID)
		}
	}
}
