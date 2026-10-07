// Package workspace loads the analysis inputs strictcode reconstructs from
// disk (stricttools/docs/check-semantics.md, workspace and manifest inputs): the
// release declarations rlsbl keeps at
// .strictmetadata/releasables/releasables.toml when present, the per-member
// manifests (pyproject.toml, package.json, go.mod), declared dependency
// scopes, and manifest-declared entry points. Nothing is passed at runtime by
// any caller; everything is read, not owned.
//
// Without a declarations file, the root is a single-project scan: one
// synthesized member named "_" at path ".". The old rlsbl layout
// (.rlsbl-monorepo/workspace.toml) is refused, naming the migration.
package workspace

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/stricttools/strictspec/go/strictspec"
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

// DepSource is where a declared dependency is resolved from.
type DepSource string

const (
	// SourceRegistry: a version constraint resolved against a registry (the
	// constraint may be empty, meaning any version).
	SourceRegistry DepSource = "registry"
	// SourcePath: a local path or file URL (pyproject "name @ file:...",
	// package.json "file:...").
	SourcePath DepSource = "path"
	// SourceWorkspace: a package manager's workspace protocol
	// (package.json "workspace:...").
	SourceWorkspace DepSource = "workspace"
)

// DeclaredDep is one manifest-declared dependency, as written (registry
// name) with its scope, its source, and its version constraint.
type DeclaredDep struct {
	Name  string
	Scope DepScope
	// Source is where the dependency resolves from.
	Source DepSource
	// Constraint is the version constraint as written, without extras and
	// environment markers: ">=1.2" for pyproject, "^1.2.0" for package.json,
	// the required version for go.mod. Empty when the declaration states
	// none; the path or protocol text for a path or workspace source.
	Constraint string
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
	// Version is the manifest-declared version: pyproject [project].version,
	// package.json version. Empty when the manifest declares none (a
	// dynamic pyproject version, go.mod).
	Version     string
	Deps        []DeclaredDep
	EntryPoints []EntryPoint
}

// Member is one workspace member (or the synthesized single-project member).
type Member struct {
	// Name is the workspace member name ("_" for single-project scans).
	Name string
	// Path is the member root relative to the workspace root ("." allowed).
	Path    string
	Library bool
	DevOnly bool
	// Published is true when the member is versioned under a releasable
	// whose publish_mode is "ci" and the member declares a publish pipeline:
	// such a member has consumers outside the workspace.
	Published bool
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
	// Single is true for single-project scans (no declarations file).
	Single bool
	// Layout is the declared repository layout ("standalone" or
	// "workspace"); empty for single-project scans.
	Layout string
	// Members in declaration order (single synthesized member for
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

// MemberAt returns the member declared at the canonical root-relative path,
// or nil.
func (w *Workspace) MemberAt(path string) *Member {
	for _, m := range w.Members {
		if m.Path == path {
			return m
		}
	}
	return nil
}

// Owner returns the member whose territory holds the canonical
// root-relative path: the member with the longest path containing it. Nil
// when no member's path contains it.
func (w *Workspace) Owner(path string) *Member {
	var best *Member
	for _, m := range w.Members {
		if !IsInside(path, m.Path) {
			continue
		}
		if best == nil || len(m.Path) > len(best.Path) || best.Path == "." {
			best = m
		}
	}
	return best
}

// IsInside reports whether the canonical root-relative path p is dir or lies
// under it; every path lies under ".".
func IsInside(p, dir string) bool {
	if dir == "." {
		return true
	}
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// DeclarationsFile is where rlsbl keeps a repository's release declarations,
// relative to the repository root. strictcode reads its members from here.
const DeclarationsFile = ".strictmetadata/releasables/releasables.toml"

// oldWorkspaceFile is the workspace file of rlsbl's old layout, which the
// record migration replaces with DeclarationsFile.
const oldWorkspaceFile = ".rlsbl-monorepo/workspace.toml"

// Load reads the workspace at root. A missing declarations file means
// single-project mode; a malformed one, and the old layout's workspace file,
// are hard errors.
func Load(root string) (*Workspace, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	ws := &Workspace{Root: absRoot}

	if _, err := os.Stat(filepath.Join(absRoot, filepath.FromSlash(oldWorkspaceFile))); err == nil {
		return nil, fmt.Errorf("workspace: %s is rlsbl's old layout, which strictcode no longer reads; "+
			"strictcode reads members from %s: run `rlsbl migrate records` to convert the repository",
			oldWorkspaceFile, DeclarationsFile)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("workspace: %w", err)
	}

	raw, err := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(DeclarationsFile)))
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
		return nil, fmt.Errorf("workspace: %s: %w", DeclarationsFile, err)
	}
	if v, ok := doc.Field("format_version"); !ok {
		return nil, fmt.Errorf("workspace: %s has no format_version", DeclarationsFile)
	} else if n, isInt := v.Int(); !isInt || n != 1 {
		return nil, fmt.Errorf("workspace: %s: format_version must be 1", DeclarationsFile)
	}
	layout, _ := stringField(doc, "repository_layout")
	if layout != "standalone" && layout != "workspace" {
		return nil, fmt.Errorf("workspace: %s: repository_layout must be \"standalone\" or \"workspace\"", DeclarationsFile)
	}
	ws.Layout = layout
	members, ok := doc.Field("members")
	if !ok || len(members.Items()) == 0 {
		return nil, fmt.Errorf("workspace: %s declares no [[members]]", DeclarationsFile)
	}
	publishModes, err := parseReleasables(doc)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	paths := map[string]bool{}
	for i, item := range members.Items() {
		m, err := parseMember(item, i, publishModes)
		if err != nil {
			return nil, err
		}
		if names[m.Name] {
			return nil, fmt.Errorf("workspace: %s: member name %q is declared twice", DeclarationsFile, m.Name)
		}
		if paths[m.Path] {
			return nil, fmt.Errorf("workspace: %s: member path %q is declared twice", DeclarationsFile, m.Path)
		}
		names[m.Name], paths[m.Path] = true, true
		if err := loadManifests(ws, m); err != nil {
			return nil, err
		}
		ws.Members = append(ws.Members, m)
	}
	if layout == "standalone" && (len(ws.Members) != 1 || ws.Members[0].Path != ".") {
		return nil, fmt.Errorf("workspace: %s: a standalone layout declares one member, at path \".\"", DeclarationsFile)
	}
	return ws, nil
}

// parseReleasables reads each [[releasables]] table's name and publish_mode,
// which decide whether a member versioned under it is published.
func parseReleasables(doc strictspec.Value) (map[string]string, error) {
	modes := map[string]string{}
	rels, ok := doc.Field("releasables")
	if !ok {
		return modes, nil
	}
	for i, item := range rels.Items() {
		name, ok := stringField(item, "name")
		if !ok || name == "" {
			return nil, fmt.Errorf("workspace: %s: releasables[%d] has no name", DeclarationsFile, i)
		}
		if _, dup := modes[name]; dup {
			return nil, fmt.Errorf("workspace: %s: releasable name %q is declared twice", DeclarationsFile, name)
		}
		mode, _ := stringField(item, "publish_mode")
		if mode != "ci" && mode != "none" {
			return nil, fmt.Errorf("workspace: %s: releasable %q: publish_mode must be \"ci\" or \"none\"", DeclarationsFile, name)
		}
		modes[name] = mode
	}
	return modes, nil
}

// parseMember reads the fields of one [[members]] table strictcode uses.
// The document is rlsbl's, which validates every other key.
func parseMember(p strictspec.Value, idx int, publishModes map[string]string) (*Member, error) {
	name, ok := stringField(p, "name")
	if !ok || name == "" {
		return nil, fmt.Errorf("workspace: %s: members[%d] has no name", DeclarationsFile, idx)
	}
	path, ok := stringField(p, "path")
	if !ok || path == "" {
		return nil, fmt.Errorf("workspace: %s: member %q has no path", DeclarationsFile, name)
	}
	if clean := cleanRelative(path); clean != path {
		return nil, fmt.Errorf("workspace: %s: member %q has the path %q, which is not canonical (relative, '/'-separated, no '.', '..', or empty segment, the root written \".\")",
			DeclarationsFile, name, path)
	}
	m := &Member{
		Name:      name,
		Path:      path,
		Manifests: map[vocab.Lang]*Manifest{},
	}
	m.Library = boolField(p, "library")
	m.DevOnly = boolField(p, "dev_only")
	rel, ok := p.Field("releasable")
	if !ok {
		return nil, fmt.Errorf("workspace: %s: member %q has no releasable (a releasable name, or false)", DeclarationsFile, name)
	}
	if s, isStr := rel.AsString(); isStr && s != "" {
		mode, declared := publishModes[s]
		if !declared {
			return nil, fmt.Errorf("workspace: %s: member %q names the releasable %q, which no [[releasables]] table declares", DeclarationsFile, name, s)
		}
		pipelines, _ := p.Field("pipelines")
		m.Published = mode == "ci" && len(pipelines.Items()) > 0
	} else if b, isBool := rel.Bool(); !isBool || b {
		return nil, fmt.Errorf("workspace: %s: member %q: releasable must be a releasable name or false", DeclarationsFile, name)
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

// cleanRelative is the canonical spelling of a relative slash path: "." for
// the root, otherwise path.Clean's form. A path that is absolute or climbs
// out returns "" so it never equals its input.
func cleanRelative(p string) string {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.TrimSpace(p) != p {
		return ""
	}
	c := pathpkg.Clean(p)
	if c == ".." || strings.HasPrefix(c, "../") {
		return ""
	}
	return c
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
