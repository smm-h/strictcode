// Package rules is the rule registry: the Go declarations documented in stricttools/docs/rules.md.
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

// Severity is the severity of a rule's findings. A rule's declared severity
// is its option's default (internal/options); a repository changes it only
// through its strictcode:<rule id> options entry.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// SuppressionShape is the natural target shape a rule's suppressions name
// (stricttools/docs/config.md). Config suppressions for a rule must match its
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

// FixTier is a tier of the three-tier fix system (stricttools/docs/fixes.md).
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

	// OptionSubject names the subject document under .strictmetadata/options/
	// (without ".toml") that the rule's strictcode:<rule id> option entries
	// are filed in.
	OptionSubject string
	// Adoption marks a rule a repository opts into: its option defaults to
	// off whatever its severity.
	Adoption bool
	// PathScoped marks a rule whose option entries may name one workspace
	// member's path, so the rule's value can differ per member.
	PathScoped bool
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

// Groups maps group names to member rule IDs. A group classifies rules in the
// registry and its documentation; it switches nothing (a rule's behavior
// changes only through its option), a finding never carries a group, and a
// suppression never targets one.
var Groups = map[string][]string{
	"library": {
		"library-forbidden-imports",
		"library-stdout",
		"library-direct-logging",
		"library-entry-point",
	},
}

// Tombstones is the retired-rule set. Empty today: nothing has shipped, and
// donor names were re-minted at their best form pre-ship (stricttools/docs/decisions.md).
var Tombstones = []Tombstone{}

// Rules lists the minted rules in catalog order.
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
		Uses:          []vocab.Capability{vocab.CapImportAttrGuarded},
		Suppression:   SuppressProjectDep,
		OptionSubject: "dependencies",
		FixTier:       Tier3,
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
		Suppression:   SuppressProjectDep,
		OptionSubject: "dependencies",
		FixTier:       Tier3,
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
		Suppression:   SuppressProjectDep,
		OptionSubject: "dependencies",
		FixTier:       Tier3,
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
		Uses:          []vocab.Capability{vocab.CapImportAttrGuarded},
		Suppression:   SuppressProjectDep,
		OptionSubject: "dependencies",
		FixTier:       Tier3,
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
		Uses:          []vocab.Capability{vocab.CapImportAttrGuarded},
		Suppression:   SuppressProjectDep,
		OptionSubject: "dependencies",
		FixTier:       Tier3,
		PlannedFixes: []PlannedFix{
			{Tier: Tier2, Description: "Rescope the declaration to runtime."},
		},
	},

	{
		ID:            "deps-stale",
		Severity:      SeverityError,
		Description:   "An intra-workspace dependency constraint in a manifest that the dependency member's declared version does not satisfy.",
		Requires:      []vocab.Capability{vocab.CapDeclaredDependencyExtraction},
		Suppression:   SuppressProjectDep,
		OptionSubject: "dependencies",
		FixTier:       Tier3,
		PlannedFixes: []PlannedFix{
			{Tier: Tier2, Description: "Raise the constraint to admit the dependency's current version."},
		},
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "a go.mod requirement names a module version minimal version selection resolves, and a workspace member's go.mod declares no version of its own to compare it with",
		},
	},

	// --- Dead code ---
	{
		ID:          "dead-modules",
		Severity:    SeverityWarning,
		Description: "Source units unreachable or unreferenced, per the per-language algorithms on the check semantics page.",
		Requires: []vocab.Capability{
			vocab.CapModuleEnumeration,
			vocab.CapImportExtraction,
			vocab.CapResolveImportsModules,
			vocab.CapTestContextClassification,
		},
		// export-extraction is a uses-capability (moved from requires,
		// stricttools/docs/decisions.md, 2026-08-04): the export-exemption facet (lesson 16) is
		// Python-only; the Go and TS algorithms need no export surface for
		// the rule to hold.
		Uses:          []vocab.Capability{vocab.CapExportExtraction, vocab.CapEntryPointDiscovery},
		Suppression:   SuppressPath,
		OptionSubject: "code",
		FixTier:       Tier3,
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
		Suppression:   SuppressMember,
		OptionSubject: "code",
		FixTier:       Tier3,
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
		Suppression:   SuppressMemberSet,
		OptionSubject: "code",
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "the Go compiler rejects import cycles; re-checking is noise",
		},
	},

	// --- Config hygiene ---
	{
		ID:            "stale-suppression",
		Severity:      SeverityError,
		Description:   "A suppression in strictcode.toml referencing a path, rule, or (project, dep) pair that no longer exists on disk or in the registry.",
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
	},

	// --- Library boundary (group:library; runs only on library = true members) ---
	{
		ID:            "library-forbidden-imports",
		Severity:      SeverityError,
		Description:   "A library importing an application-concern module (per-language default lists, replaceable; workspace and per-language allow lists subtracted).",
		Requires:      []vocab.Capability{vocab.CapImportExtraction},
		Uses:          []vocab.Capability{vocab.CapTestContextClassification},
		Groups:        []string{"library"},
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
	},
	{
		ID:          "library-stdout",
		Severity:    SeverityError,
		Description: "A library writing to standard streams (print, sys.stdout.write, fmt.Print*, console.log family).",
		Requires: []vocab.Capability{
			vocab.CapCallableExtraction,
			vocab.CapCallResolutionSyntactic,
		},
		Groups:        []string{"library"},
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
	},
	{
		ID:          "library-direct-logging",
		Severity:    SeverityWarning,
		Description: "A Python library calling the root logger directly instead of taking a logger.",
		Requires: []vocab.Capability{
			vocab.CapCallableExtraction,
			vocab.CapCallResolutionSyntactic,
		},
		Groups:        []string{"library"},
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "the diagnosis is specific to Python's root-logger idiom",
			vocab.LangTS: "the diagnosis is specific to Python's root-logger idiom",
		},
	},
	{
		ID:            "library-entry-point",
		Severity:      SeverityError,
		Description:   "A library declaring a CLI entry point ([project.scripts], func main in package main, npm bin).",
		Requires:      []vocab.Capability{vocab.CapEntryPointDiscovery},
		Groups:        []string{"library"},
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
	},

	// --- Correctness ---
	{
		ID:            "unreachable-code",
		Severity:      SeverityError,
		Description:   "Statements following an unconditional terminator in the same block (comment-aware; nested scopes independent). All projects, not only libraries.",
		Requires:      []vocab.Capability{vocab.CapUnreachableStatementAnalysis},
		Suppression:   SuppressPath,
		OptionSubject: "code",
		// The flagship whitelisted transform shipped in round 3: removal of
		// the unreachable statements, verified by post-fix re-extraction.
		FixTier: Tier1,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "go vet reports unreachable code natively",
		},
	},

	// --- Python tools and the scope guards over their configuration ---
	{
		ID:            "lint",
		Severity:      SeverityError,
		Description:   "ruff check reports a violation in the paths the [python_tools.lint] declaration names, run through the project's environment with uv run.",
		Suppression:   SuppressNone,
		OptionSubject: "code",
		Adoption:      true,
		PathScoped:    true,
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "runs ruff, a Python linter",
			vocab.LangTS: "runs ruff, a Python linter",
		},
	},
	{
		ID:            "lint-scope-guard",
		Severity:      SeverityError,
		Description:   "ruff's own configuration narrows the paths the [python_tools.lint] declaration names: include or extend-include in pyproject.toml [tool.ruff], ruff.toml, or .ruff.toml.",
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "guards ruff, a Python linter",
			vocab.LangTS: "guards ruff, a Python linter",
		},
	},
	{
		ID:            "format",
		Severity:      SeverityError,
		Description:   "ruff format --check reports a file it would reformat in the paths the [python_tools.format] declaration names, run through the project's environment with uv run.",
		Suppression:   SuppressNone,
		OptionSubject: "code",
		Adoption:      true,
		PathScoped:    true,
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "runs ruff, a Python formatter",
			vocab.LangTS: "runs ruff, a Python formatter",
		},
	},
	{
		ID:            "format-scope-guard",
		Severity:      SeverityError,
		Description:   "ruff's own configuration narrows the paths the [python_tools.format] declaration names: include or extend-include in pyproject.toml [tool.ruff], ruff.toml, or .ruff.toml.",
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "guards ruff, a Python formatter",
			vocab.LangTS: "guards ruff, a Python formatter",
		},
	},
	{
		ID:            "type-check",
		Severity:      SeverityError,
		Description:   "mypy reports an error in the paths the [python_tools.type-check] declaration names, run through the project's environment with uv run.",
		Suppression:   SuppressNone,
		OptionSubject: "code",
		Adoption:      true,
		PathScoped:    true,
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "runs mypy, a Python type checker",
			vocab.LangTS: "runs mypy, a Python type checker",
		},
	},
	{
		ID:            "type-check-scope-guard",
		Severity:      SeverityError,
		Description:   "mypy's own configuration declares a scope the paths of the [python_tools.type-check] declaration silently override: files, packages, or modules in pyproject.toml [tool.mypy], mypy.ini, .mypy.ini, or setup.cfg [mypy].",
		Suppression:   SuppressNone,
		OptionSubject: "code",
		FixTier:       Tier3,
		NotApplicable: map[vocab.Lang]string{
			vocab.LangGo: "guards mypy, a Python type checker",
			vocab.LangTS: "guards mypy, a Python type checker",
		},
	},

	// --- Schema migration certificates ---
	{
		ID:            "strictspec-certificate",
		Severity:      SeverityError,
		Description:   "The strictspec diff certificate the [strictspec_certificate] declaration names holds a violated claim, an unsupported claim no adjudication entry discharges, or an adjudication entry that discharges no claim.",
		Suppression:   SuppressNone,
		OptionSubject: "release",
		Adoption:      true,
		FixTier:       Tier3,
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
