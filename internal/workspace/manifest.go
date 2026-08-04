package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/smm-h/strictcode/internal/vocab"
	"github.com/smm-h/strictspec/go/strictspec"
	"golang.org/x/mod/modfile"
)

// pep508Name matches the distribution-name prefix of a PEP 508 requirement.
var pep508Name = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?`)

// loadPyproject parses a pyproject.toml: name, dependency scopes
// (project.dependencies = runtime; project.optional-dependencies = peer;
// dependency-groups = dev), and [project.scripts]/[project.gui-scripts]
// entry points.
func loadPyproject(ws *Workspace, m *Member, relPath string) (*Manifest, error) {
	doc, err := readManifestDoc(ws, relPath, "toml")
	if err != nil {
		return nil, err
	}
	mf := &Manifest{Lang: vocab.LangPy, Path: relPath}

	project, hasProject := doc.Field("project")
	if hasProject {
		mf.Name, _ = stringField(project, "name")

		if deps, ok := project.Field("dependencies"); ok {
			for _, item := range deps.Items() {
				addRequirement(mf, item, ScopeRuntime)
			}
		}
		if opt, ok := project.Field("optional-dependencies"); ok {
			for _, kv := range sortedEntries(opt) {
				for _, item := range kv.Value.Items() {
					addRequirement(mf, item, ScopePeer)
				}
			}
		}
		for _, key := range []string{"scripts", "gui-scripts"} {
			form := "script"
			if key == "gui-scripts" {
				form = "gui_script"
			}
			if scripts, ok := project.Field(key); ok {
				for _, kv := range sortedEntries(scripts) {
					target, _ := kv.Value.AsString()
					mf.EntryPoints = append(mf.EntryPoints, EntryPoint{Form: form, Name: kv.Key, Target: target})
				}
			}
		}
	}
	if groups, ok := doc.Field("dependency-groups"); ok {
		for _, kv := range sortedEntries(groups) {
			for _, item := range kv.Value.Items() {
				// {include-group = "..."} tables are group references, not deps.
				addRequirement(mf, item, ScopeDev)
			}
		}
	}
	return mf, nil
}

func addRequirement(mf *Manifest, item strictspec.Value, scope DepScope) {
	s, ok := item.AsString()
	if !ok {
		return
	}
	name := pep508Name.FindString(strings.TrimSpace(s))
	if name == "" {
		return
	}
	mf.Deps = append(mf.Deps, DeclaredDep{Name: name, Scope: scope})
}

// loadPackageJSON parses a package.json: name, dependency scopes
// (dependencies = runtime; devDependencies = dev; peerDependencies and
// optionalDependencies = peer), and exports/main/bin entry points
// (lesson 19: entry points from exports (recursive), main, and bin).
func loadPackageJSON(ws *Workspace, m *Member, relPath string) (*Manifest, error) {
	doc, err := readManifestDoc(ws, relPath, "json")
	if err != nil {
		return nil, err
	}
	mf := &Manifest{Lang: vocab.LangTS, Path: relPath}
	mf.Name, _ = stringField(doc, "name")

	depFields := []struct {
		key   string
		scope DepScope
	}{
		{"dependencies", ScopeRuntime},
		{"devDependencies", ScopeDev},
		{"peerDependencies", ScopePeer},
		{"optionalDependencies", ScopePeer},
	}
	for _, df := range depFields {
		if deps, ok := doc.Field(df.key); ok {
			for _, kv := range sortedEntries(deps) {
				mf.Deps = append(mf.Deps, DeclaredDep{Name: kv.Key, Scope: df.scope})
			}
		}
	}

	if main, ok := stringField(doc, "main"); ok && main != "" {
		mf.EntryPoints = append(mf.EntryPoints, EntryPoint{Form: "export", Name: "main", Target: main})
	}
	if exports, ok := doc.Field("exports"); ok {
		collectExports(mf, "exports", exports)
	}
	if bin, ok := doc.Field("bin"); ok {
		if s, isStr := bin.AsString(); isStr {
			binName := mf.Name
			if binName == "" {
				binName = "bin"
			}
			mf.EntryPoints = append(mf.EntryPoints, EntryPoint{Form: "bin", Name: binName, Target: s})
		} else {
			for _, kv := range sortedEntries(bin) {
				if target, isStr := kv.Value.AsString(); isStr {
					mf.EntryPoints = append(mf.EntryPoints, EntryPoint{Form: "bin", Name: kv.Key, Target: target})
				}
			}
		}
	}
	return mf, nil
}

// collectExports recursively traverses the package.json exports
// condition/subpath tree; every string leaf is an export entry point.
func collectExports(mf *Manifest, path string, v strictspec.Value) {
	if s, ok := v.AsString(); ok {
		mf.EntryPoints = append(mf.EntryPoints, EntryPoint{Form: "export", Name: path, Target: s})
		return
	}
	if v.Kind() == strictspec.KindRecord {
		for _, kv := range sortedEntries(v) {
			collectExports(mf, path+"/"+kv.Key, kv.Value)
		}
	}
	// Arrays (fallback targets) and null leaves are ignored: null blocks a
	// subpath, and array fallbacks are a legacy npm form.
}

// loadGoMod parses a go.mod: module path and require directives (all
// runtime scope — Go has no dev-dependency concept in go.mod).
func loadGoMod(ws *Workspace, m *Member, relPath string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(ws.Root, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	f, err := modfile.Parse(relPath, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("workspace: %s: %w", relPath, err)
	}
	mf := &Manifest{Lang: vocab.LangGo, Path: relPath}
	if f.Module != nil {
		mf.GoModulePath = f.Module.Mod.Path
	}
	for _, req := range f.Require {
		mf.Deps = append(mf.Deps, DeclaredDep{Name: req.Mod.Path, Scope: ScopeRuntime})
	}
	return mf, nil
}

func readManifestDoc(ws *Workspace, relPath, syntax string) (strictspec.Value, error) {
	raw, err := os.ReadFile(filepath.Join(ws.Root, filepath.FromSlash(relPath)))
	if err != nil {
		return strictspec.Value{}, fmt.Errorf("workspace: %w", err)
	}
	doc, err := strictspec.LoadValue(raw, syntax)
	if err != nil {
		return strictspec.Value{}, fmt.Errorf("workspace: %s: %w", relPath, err)
	}
	return doc, nil
}
