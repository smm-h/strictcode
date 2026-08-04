// Package rules is the rule registry: the Go declarations behind CATALOG.md.
// Rule IDs follow the mint-once scheme — flat lowercase-hyphenated names, one
// ID per diagnosis, encoding nothing. All metadata lives here; the committed
// registry dump (REGISTRY.json, produced by `strictcode registry dump`) is the
// CI-diffed artifact.
//
// Lifecycle: mint (minor) and tombstone (breaking). IDs are never renamed or
// reused; a tombstoned ID stays in Tombstones forever and renders an
// actionable error when config references it.
package rules

import "github.com/smm-h/strictcode/internal/vocab"

// Severity is a rule's default severity. Config may override per rule.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// SuppressionShape is the natural target shape a rule's suppressions name
// (DESIGN.md section 12.3). Config suppressions for a rule must match its
// shape; every suppression carries a mandatory non-empty reason.
type SuppressionShape string

const (
	// SuppressNone: the rule accepts no suppressions (e.g. stale-suppression —
	// suppressing the staleness check would defeat it).
	SuppressNone SuppressionShape = "none"
	// SuppressPath: a file path (Python, TS/JS) or package directory (Go).
	SuppressPath SuppressionShape = "path"
	// SuppressProjectDep: a (project, dep) pair.
	SuppressProjectDep SuppressionShape = "project-dep"
	// SuppressMemberSet: the set of modules forming one reported cycle.
	SuppressMemberSet SuppressionShape = "member-set"
	// SuppressMember: a single workspace member name.
	SuppressMember SuppressionShape = "member"
)

// FixTier is a tier of the three-tier fix system (DESIGN.md section 7).
// Tier 3 is suggestion-only; every rule ships detection-first at tier 3.
type FixTier int

const (
	Tier1 FixTier = 1
	Tier2 FixTier = 2
	Tier3 FixTier = 3
)

// PlannedFix records a fix tier a rule is planned to gain, with what the
// transform will do. Planned fixes are documentation until the transform
// lands through the tier-1 whitelist / tier-2 consent process.
type PlannedFix struct {
	Tier        FixTier
	Description string
}

// Rule is one minted rule's registry declaration.
type Rule struct {
	ID          string
	Severity    Severity
	Description string

	// Requires: every capability must be supported in a language's profile
	// for the rule's matrix cell to be supported; any planned makes the cell
	// planned; any not-applicable makes the rule n/a for that language.
	Requires []vocab.Capability
	// Uses: optional enrichment. A not-applicable or absent uses-capability
	// never blocks support.
	Uses []vocab.Capability

	// Groups this rule belongs to (bare names; rendered as group:<name>).
	Groups []string

	// Suppression is the rule's suppression target shape.
	Suppression SuppressionShape

	// FixTier is the tier the rule ships at today (always Tier3 in v1:
	// detection-first).
	FixTier FixTier
	// PlannedFixes lists tiers the rule is planned to gain.
	PlannedFixes []PlannedFix

	// NotApplicable holds explicit per-language overrides with mandatory
	// reasons, for cases where the capability calculus is satisfied but the
	// check is meaningless in the ecosystem.
	NotApplicable map[vocab.Lang]string
}

// Tombstone records a retired rule ID. The unknown-rule hard error renders
// it: "retired in <RetiredIn>: <Reason>; use <ReplacedBy>; <Migration>."
type Tombstone struct {
	ID         string
	RetiredIn  string
	Reason     string
	ReplacedBy []string // empty = gone without successor; two+ = it split
	Migration  string
}

// Groups maps group names to member rule IDs. Groups are convenience
// switches: a finding never carries a group; suppressions never target one.
var Groups = map[string][]string{
	"library": {
		"library-forbidden-imports",
		"library-stdout",
		"library-direct-logging",
		"library-entry-point",
	},
}

// Tombstones is the retired-rule set. Empty today: nothing has shipped, and
// donor names were re-minted at their best form pre-ship (CATALOG.md).
var Tombstones = []Tombstone{}

// Rules lists the fourteen minted rules in catalog order.
var Rules = []Rule{
	// --- Dependency hygiene ---
	{
		ID:          "deps-unused",
		Severity:    SeverityError,
		Description: "A workspace-internal dependency declared in the manifest that no source file imports.",
		Requires: []vocab.Capability{
			vocab.CapImportExtraction,
			vocab.CapResolveImportsInternal,
			vocab.CapDeclaredDependencyExtraction,
			vocab.CapTestContextClassification,
		},
		Uses:        []vocab.Capability{vocab.CapImportAttrGuarded},
		Suppression: SuppressProjectDep,
		FixTier:     Tier3,
		PlannedFixes: []PlannedFix{
			{Tier: Tier2, Description: "Remove the declaration from the manifest."},
		},
	},
	{
		ID:          "deps-hard-guarded-only",
		Severity:    SeverityError,
		Description: "A hard dependency (scope runtime/explicit) imported only under optional-import guards — contradictory; declare it optional or import it unconditionally.",
		Requires: []vocab.Capability{
			vocab.CapImportExtraction,
			vocab.CapResolveImportsInternal,
			vocab.CapDeclaredDependencyExtraction,
			vocab.CapTestContextClassification,
			vocab.CapImportAttrGuarded,
		},
		Suppression: SuppressProjectDep,
		FixTier:     Tier3,
	},
	{
		ID:          "deps-undeclared",
		Severity:    SeverityError,
		Description: "Production source importing a workspace package the manifest does not declare.",
		Requires: []vocab.Capability{
			vocab.CapImportExtraction,
			vocab.CapResolveImportsInternal,
			vocab.CapDeclaredDependencyExtraction,
			vocab.CapTestContextClassification,
		},
		Uses: []vocab.Capability{
			vocab.CapImportAttrGuarded,
			vocab.CapImportAttrTypeChecking,
		},
		Suppression: SuppressProjectDep,
		FixTier:     Tier3,
		PlannedFixes: []PlannedFix{
			{Tier: Tier2, Description: "Add the declaration to the manifest."},
		},
	},
	{
		ID:          "deps-runtime-test-only",
		Severity:    SeverityWarning,
		Description: "A runtime-scoped dependency imported only by test code — should be dev-scoped.",
		Requires: []vocab.Capability{
			vocab.CapImportExtraction,
			vocab.CapResolveImportsInternal,
			vocab.CapDeclaredDependencyExtraction,
			vocab.CapTestContextClassification,
		},
		Uses:        []vocab.Capability{vocab.CapImportAttrGuarded},
		Suppression: SuppressProjectDep,
		FixTier:     Tier3,
		PlannedFixes: []PlannedFix{
			{Tier: Tier2, Description: "Rescope the declaration to dev."},
		},
	},
	{
		ID:          "deps-dev-in-production",
		Severity:    SeverityError,
		Description: "A dev-scoped dependency imported by production code.",
		Requires: []vocab.Capability{
			vocab.CapImportExtraction,
			vocab.CapResolveImportsInternal,
			vocab.CapDeclaredDependencyExtraction,
			vocab.CapTestContextClassification,
		},
		Uses:        []vocab.Capability{vocab.CapImportAttrGuarded},
		Suppression: SuppressProjectDep,
		FixTier:     Tier3,
		PlannedFixes: []PlannedFix{
			{Tier: Tier2, Description: "Rescope the declaration to runtime."},
		},
	},

	// --- Dead code ---
	{
		ID:          "dead-modules",
		Severity:    SeverityWarning,
		Description: "Source units unreachable or unreferenced, per the per-language algorithms pinned in DESIGN.md section 6.2.",
		Requires: []vocab.Capability{
			vocab.CapModuleEnumeration,
			vocab.CapImportExtraction,
			vocab.CapResolveImportsModules,
			vocab.CapTestContextClassification,
		},
		// export-extraction is a uses-capability (moved from requires,
		// BUILDLOG 2026-08-04): the export-exemption facet (lesson 16) is
		// Python-only; the Go and TS algorithms need no export surface for
		// the rule to hold.
		Uses:        []vocab.Capability{vocab.CapExportExtraction, vocab.CapEntryPointDiscovery},
		Suppression: SuppressPath,
		FixTier:     Tier3,
		PlannedFixes: []PlannedFix{
			{Tier: Tier2, Description: "Delete the dead unit (consent-gated: deletion is behavior-relevant)."},
		},
	},
	{
		ID:          "dead-workspace-packages",
		Severity:    SeverityWarning,
		Description: "A library workspace member no sibling imports; test-only importers reported distinctly from zero importers.",
		Requires: []vocab.Capability{
			vocab.CapImportExtraction,
			vocab.CapResolveImportsInternal,
			vocab.CapDeclaredDependencyExtraction,
			vocab.CapTestContextClassification,
		},
		Suppression: SuppressMember,
		FixTier:     Tier3,
	},

	// --- Cycles ---
	{
		ID:          "import-cycles",
		Severity:    SeverityWarning,
		Description: "Import cycles within a project: Tarjan SCC over the module imports projection, SCCs of size >= 2 only.",
		Requires: []vocab.Capability{
			vocab.CapModuleEnumeration,
			vocab.CapImportExtraction,
			vocab.CapResolveImportsModules,
		},
		Suppression: SuppressMemberSet,
		FixTier:     Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "the Go compiler rejects import cycles; re-checking is noise",
		},
	},

	// --- Config hygiene ---
	{
		ID:          "stale-suppression",
		Severity:    SeverityError,
		Description: "A suppression in strictcode.toml referencing a path, rule, or (project, dep) pair that no longer exists on disk or in the registry.",
		Suppression: SuppressNone,
		FixTier:     Tier3,
	},

	// --- Library boundary (group:library; runs only on library = true members) ---
	{
		ID:          "library-forbidden-imports",
		Severity:    SeverityError,
		Description: "A library importing an application-concern module (per-language default lists, replaceable; workspace and per-language allow lists subtracted).",
		Requires:    []vocab.Capability{vocab.CapImportExtraction},
		Uses:        []vocab.Capability{vocab.CapTestContextClassification},
		Groups:      []string{"library"},
		Suppression: SuppressNone,
		FixTier:     Tier3,
	},
	{
		ID:          "library-stdout",
		Severity:    SeverityError,
		Description: "A library writing to standard streams (print, sys.stdout.write, fmt.Print*, console.log family).",
		Requires: []vocab.Capability{
			vocab.CapCallableExtraction,
			vocab.CapCallResolutionSyntactic,
		},
		Groups:      []string{"library"},
		Suppression: SuppressNone,
		FixTier:     Tier3,
	},
	{
		ID:          "library-direct-logging",
		Severity:    SeverityWarning,
		Description: "A Python library calling the root logger directly instead of taking a logger.",
		Requires: []vocab.Capability{
			vocab.CapCallableExtraction,
			vocab.CapCallResolutionSyntactic,
		},
		Groups:      []string{"library"},
		Suppression: SuppressNone,
		FixTier:     Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "the diagnosis is specific to Python's root-logger idiom",
			vocab.LangTS: "the diagnosis is specific to Python's root-logger idiom",
		},
	},
	{
		ID:          "library-entry-point",
		Severity:    SeverityError,
		Description: "A library declaring a CLI entry point ([project.scripts], func main in package main, npm bin).",
		Requires:    []vocab.Capability{vocab.CapEntryPointDiscovery},
		Groups:      []string{"library"},
		Suppression: SuppressNone,
		FixTier:     Tier3,
	},

	// --- Correctness ---
	{
		ID:          "unreachable-code",
		Severity:    SeverityError,
		Description: "Statements following an unconditional terminator in the same block (comment-aware; nested scopes independent). All projects, not only libraries.",
		Requires:    []vocab.Capability{vocab.CapUnreachableStatementAnalysis},
		Suppression: SuppressPath,
		// The flagship whitelisted transform shipped in round 3: removal of
		// the unreachable statements, verified by post-fix re-extraction.
		FixTier: Tier1,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "go vet reports unreachable code natively",
		},
	},
}

// ByID returns the rule with the given ID; the boolean is false when no such
// live rule exists (tombstones are not rules).
func ByID(id string) (Rule, bool) {
	for _, r := range Rules {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}

// TombstoneByID returns the tombstone for a retired ID, if any.
func TombstoneByID(id string) (Tombstone, bool) {
	for _, t := range Tombstones {
		if t.ID == id {
			return t, true
		}
	}
	return Tombstone{}, false
}
