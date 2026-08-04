package extract

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/workspace"
)

// excludedDirs is the always-excluded set from DESIGN.md section 6.6
// (lesson 29). *.egg-info is handled separately as a suffix pattern.
var excludedDirs = map[string]bool{
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".git":          true,
	"node_modules":  true,
	"build":         true,
	"dist":          true,
	".tox":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".ruff_cache":   true,
	".selfdoc":      true,
	"_build":        true,
	"static":        true,
	"public":        true,
	"assets":        true,
}

func dirExcluded(name string) bool {
	return excludedDirs[name] || strings.HasSuffix(name, ".egg-info")
}

// walkMember yields the member's files with the given extension predicate,
// as member-relative slash paths, sorted. Sibling members' trees are pruned
// (lesson 13): when a member's declared path is a parent of another
// member's path, the sibling subtree never enters this member's scan.
func walkMember(ws *workspace.Workspace, m *workspace.Member, wantFile func(name string) bool) ([]string, error) {
	memberRoot := filepath.Join(ws.Root, filepath.FromSlash(m.Path))

	// Absolute roots of sibling members nested under this member's path.
	var pruned []string
	for _, other := range ws.Members {
		if other == m {
			continue
		}
		otherRoot := filepath.Join(ws.Root, filepath.FromSlash(other.Path))
		if otherRoot != memberRoot && strings.HasPrefix(otherRoot+string(filepath.Separator), memberRoot+string(filepath.Separator)) {
			pruned = append(pruned, otherRoot)
		}
	}

	var files []string
	err := filepath.WalkDir(memberRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != memberRoot && dirExcluded(d.Name()) {
				return filepath.SkipDir
			}
			for _, p := range pruned {
				if path == p {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !wantFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(memberRoot, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// wsRelPath joins a member-relative path into a workspace-root-relative one.
func wsRelPath(m *workspace.Member, memberRel string) string {
	if m.Path == "." {
		return memberRel
	}
	return m.Path + "/" + memberRel
}
