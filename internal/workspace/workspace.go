// Package workspace loads the analysis inputs strictcode reconstructs from
// disk (DESIGN.md section 6.6): the rlsbl workspace file
// (.rlsbl-monorepo/workspace.toml) when present, the per-member manifests
// (pyproject.toml, package.json, go.mod), declared dependency scopes, and
// manifest-declared entry points. Nothing is passed at runtime by any
// caller; everything is read, not owned.
//
// Without a workspace file, the root is a single-project scan: one
// synthesized member named "_" at path ".".
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictspec/go/strictspec"
)

// DepScope is a declared dependency's scope (vocabulary enum
// dependency_scope).
type DepScope string

const (
	ScopeRuntime  DepScope = "runtime"
	ScopeDev      DepScope = "dev"
	ScopePeer     DepScope = "peer"
	ScopeExplicit DepScope = "explicit"
)

// Optional reports whether guards may satisfy this scope (lessons 1-2: only
// dev/peer deps are optional; runtime/explicit are hard).
func (s DepScope) Optional() bool { return s == ScopeDev || s == ScopePeer }

// DeclaredDep is one manifest-declared dependency, as written (registry
// name) with its scope.
type DeclaredDep struct {
	Name  string
	Scope DepScope
}

// EntryPoint is one manifest-declared entry point.
type EntryPoint struct {
	// Form is a vocabulary entry_point_form value: script, gui_script,
	// export, bin, main_package.
	Form string
	// Name is the declared name (script key, bin key, export subpath, or
	// package path for Go main packages).
	Name string
	// Target is the raw declared target: "pkg.mod:func" for Python scripts,
	// a file path for npm entries.
	Target string
}

// Manifest is one language's manifest within a member.
type Manifest struct {
	Lang vocab.Lang
	// Path is the manifest file path relative to the workspace root.
	Path string
	// Name is the manifest-declared package name (the registry name):
	// pyproject [project.name], package.json name. Empty for go.mod.
	Name string
	// GoModulePath is the module path from go.mod (Go only).
	GoModulePath string
	Deps         []DeclaredDep
	EntryPoints  []EntryPoint
}

// Member is one workspace member (or the synthesized single-project member).
type Member struct {
	// Name is the workspace member name ("_" for single-project scans).
	Name string
	// Path is the member root relative to the workspace root ("." allowed).
	Path       string
	Library    bool
	DevOnly    bool
	Releasable bool // belongs to a releasable group (published externally)
	// ImportName is the explicit Python import-name override from
	// workspace.toml (resolution order step 3).
	ImportName string
	// RegistryNameOverride is workspace.toml's registry_name, when set.
	RegistryNameOverride string
	// LintAllow is the per-member allow list for library-forbidden-imports.
	LintAllow []string
	// Manifests maps each language present in the member to its manifest.
	Manifests map[vocab.Lang]*Manifest
}

// RegistryName returns the member's registry name for a language: the
// workspace override when set, else the manifest-declared name.
func (m *Member) RegistryName(lang vocab.Lang) string {
	if m.RegistryNameOverride != "" {
		return m.RegistryNameOverride
	}
	if mf := m.Manifests[lang]; mf != nil {
		return mf.Name
	}
	return ""
}

// Workspace is the loaded analysis input.
type Workspace struct {
	// Root is the absolute workspace root.
	Root string
	// Single is true for single-project scans (no workspace.toml).
	Single bool
	// Members in workspace declaration order (single synthesized member for
	// single-project scans).
	Members []*Member
}

// MemberByName returns the named member, or nil.
func (w *Workspace) MemberByName(name string) *Member {
	for _, m := range w.Members {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// workspaceFile is the rlsbl workspace file location.
const workspaceFile = ".rlsbl-monorepo/workspace.toml"

// Load reads the workspace at root. A missing workspace file means
// single-project mode; a malformed one is a hard error.
func Load(root string) (*Workspace, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	ws := &Workspace{Root: absRoot}

	wsPath := filepath.Join(absRoot, workspaceFile)
	raw, err := os.ReadFile(wsPath)
	if os.IsNotExist(err) {
		ws.Single = true
		member := &Member{Name: "_", Path: ".", Manifests: map[vocab.Lang]*Manifest{}}
		if err := loadManifests(ws, member); err != nil {
			return nil, err
		}
		ws.Members = []*Member{member}
		return ws, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}

	doc, err := strictspec.LoadValue(raw, "toml")
	if err != nil {
		return nil, fmt.Errorf("workspace: %s: %w", workspaceFile, err)
	}
	projects, ok := doc.Field("projects")
	if !ok {
		return nil, fmt.Errorf("workspace: %s has no [[projects]]", workspaceFile)
	}
	seen := map[string]bool{}
	for i, p := range projects.Items() {
		m, err := parseProject(p, i)
		if err != nil {
			return nil, err
		}
		if seen[m.Name] {
			return nil, fmt.Errorf("workspace: duplicate member name %q", m.Name)
		}
		seen[m.Name] = true
		if err := loadManifests(ws, m); err != nil {
			return nil, err
		}
		ws.Members = append(ws.Members, m)
	}
	if len(ws.Members) == 0 {
		return nil, fmt.Errorf("workspace: %s declares no projects", workspaceFile)
	}
	return ws, nil
}

func parseProject(p strictspec.Value, idx int) (*Member, error) {
	name, ok := stringField(p, "name")
	if !ok || name == "" {
		return nil, fmt.Errorf("workspace: projects[%d] has no name", idx)
	}
	path, ok := stringField(p, "path")
	if !ok || path == "" {
		return nil, fmt.Errorf("workspace: project %q has no path", name)
	}
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "."
	}
	m := &Member{
		Name:      name,
		Path:      path,
		Manifests: map[vocab.Lang]*Manifest{},
	}
	m.Library = boolField(p, "library")
	// Both spellings exist in current rlsbl workspaces (dev_only, and the
	// older dev_node); the donor reads either as the dev marker.
	m.DevOnly = boolField(p, "dev_only") || boolField(p, "dev_node")
	if rel, ok := p.Field("releasable"); ok {
		if s, isStr := rel.AsString(); isStr && s != "" {
			m.Releasable = true
		}
	}
	m.ImportName, _ = stringField(p, "import_name")
	m.RegistryNameOverride, _ = stringField(p, "registry_name")
	if la, ok := p.Field("lint_allow"); ok {
		for _, item := range la.Items() {
			if s, isStr := item.AsString(); isStr {
				m.LintAllow = append(m.LintAllow, s)
			}
		}
	}
	return m, nil
}

// loadManifests detects and parses the member's per-language manifests.
func loadManifests(ws *Workspace, m *Member) error {
	dir := filepath.Join(ws.Root, m.Path)
	type probe struct {
		file string
		lang vocab.Lang
		load func(ws *Workspace, m *Member, relPath string) (*Manifest, error)
	}
	probes := []probe{
		{"pyproject.toml", vocab.LangPy, loadPyproject},
		{"package.json", vocab.LangTS, loadPackageJSON},
		{"go.mod", vocab.LangGo, loadGoMod},
	}
	for _, pr := range probes {
		full := filepath.Join(dir, pr.file)
		if _, err := os.Stat(full); err != nil {
			continue
		}
		rel, err := filepath.Rel(ws.Root, full)
		if err != nil {
			return fmt.Errorf("workspace: %w", err)
		}
		mf, err := pr.load(ws, m, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		m.Manifests[pr.lang] = mf
	}
	return nil
}

// --- shared helpers -------------------------------------------------------

func stringField(v strictspec.Value, name string) (string, bool) {
	f, ok := v.Field(name)
	if !ok {
		return "", false
	}
	return f.AsString()
}

func boolField(v strictspec.Value, name string) bool {
	f, ok := v.Field(name)
	if !ok {
		return false
	}
	b, _ := f.Bool()
	return b
}

// sortedEntries returns a map value's entries sorted by key for
// deterministic iteration.
func sortedEntries(v strictspec.Value) []strictspec.KV {
	entries := v.Entries()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}
