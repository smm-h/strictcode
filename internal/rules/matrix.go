package rules

import "github.com/smm-h/strictcode/internal/vocab"

// CellStatus is a matrix cell verdict.
type CellStatus string

const (
	CellSupported     CellStatus = "supported"
	CellPlanned       CellStatus = "planned"
	CellNotApplicable CellStatus = "n/a"
)

// Cell is one language x rule matrix cell: the verdict plus the mandatory
// reason when not applicable.
type Cell struct {
	Status CellStatus
	Reason string
}

// LanguageIndependent reports whether a rule engages no language
// capabilities at all (e.g. stale-suppression, evaluated against config,
// disk, and registry). Such rules apply to every project regardless of
// language and are presented as a single spanning row in the matrix.
func (r Rule) LanguageIndependent() bool {
	return len(r.Requires) == 0 && len(r.Uses) == 0
}

// MatrixCell computes the matrix cell for a rule and language per the
// CATALOG.md requires/uses model:
//
//  1. an explicit per-rule not_applicable override wins, with its reason;
//  2. a required capability that is not-applicable in the profile makes the
//     rule n/a, surfacing the capability's reason;
//  3. otherwise any planned required capability makes the cell planned;
//  4. otherwise supported. Uses-capabilities never block support.
//
// A rule requiring a capability the profile lacks entirely is a hard error
// at generation time (closed sets); here it panics, because the registry
// tests and vocabgen enforce the closed set before this can run.
func MatrixCell(r Rule, lang vocab.Lang) Cell {
	if reason, ok := r.NotApplicable[lang]; ok {
		return Cell{Status: CellNotApplicable, Reason: reason}
	}
	profile, ok := vocab.Profiles[lang]
	if !ok {
		panic("rules: unknown language " + string(lang))
	}
	status := CellSupported
	for _, cap := range r.Requires {
		pc, ok := profile.Capabilities[cap]
		if !ok {
			panic("rules: profile " + string(lang) + " lacks capability " + string(cap))
		}
		switch pc.Status {
		case vocab.StatusNotApplicable:
			return Cell{Status: CellNotApplicable, Reason: pc.Reason}
		case vocab.StatusPlanned:
			status = CellPlanned
		}
	}
	return Cell{Status: status}
}
