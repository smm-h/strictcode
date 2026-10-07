package extract

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smm-h/strictcode/internal/workspace"
)

// excludedAnywhere are directory names no source walk enters at any depth
// (lesson 29): environments, caches, and VCS metadata, which no project
// names a package after. *.egg-info is handled separately as a suffix
// pattern.
var excludedAnywhere = map[string]bool{
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".git":          true,
	"node_modules":  true,
	".tox":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".ruff_cache":   true,
	".selfdoc":      true,
}

// excludedAtMemberRoot are build-artifact and asset directory names a source
// walk leaves out only as the first component of a member-relative path
// (lesson 29). Deeper, the same name is an ordinary package directory: a Go
// project's internal/build is a package (lesson 40).
var excludedAtMemberRoot = map[string]bool{
	"build":  true,
	"dist":   true,
	"_build": true,
	"static": true,
	"public": true,
	"assets": true,
}

// excludedPath reports whether a member-relative file path lies in a
// directory no source walk enters.
func excludedPath(memberRel string) bool {
	parts := strings.Split(memberRel, "/")
	for i, dir := range parts[:len(parts)-1] {
		if excludedAnywhere[dir] || strings.HasSuffix(dir, ".egg-info") {
			return true
		}
		if i == 0 && excludedAtMemberRoot[dir] {
			return true
		}
	}
	return false
}

// sourceList is every file git lists under the workspace root: tracked
// files and untracked files that are not ignored (lesson 36), as
// workspace-root-relative slash paths, sorted. A gitignored file, a
// third-party clone among them, is never read.
type sourceList struct {
	files []string
}

// listSources runs git ls-files at the workspace root. A root outside any
// git work tree is refused: which files are the project's is what git
// says, and without git there is no answer to read.
func listSources(root string) (*sourceList, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("extract: %s is not inside a git work tree, and strictcode reads the source files git lists: run strictcode in a git repository (git ls-files: %s)",
				root, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("extract: git ls-files did not run: %w", err)
	}
	seen := map[string]bool{}
	var files []string
	for _, entry := range strings.Split(stdout.String(), "\x00") {
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		// A tracked file deleted from the working tree, a symlink, or a
		// submodule is not a source file to read.
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(entry)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("extract: %w", err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, entry)
	}
	sort.Strings(files)
	return &sourceList{files: files}, nil
}

// walkMember yields the member's files with the given name predicate, as
// member-relative slash paths, sorted: the files git lists under the
// member's path, minus the excluded directories and minus the trees of
// other members nested under it (lesson 13).
func walkMember(list *sourceList, ws *workspace.Workspace, m *workspace.Member, wantFile func(name string) bool) ([]string, error) {
	var nested []string
	for _, other := range ws.Members {
		if other != m && other.Path != m.Path && workspace.IsInside(other.Path, m.Path) {
			nested = append(nested, other.Path)
		}
	}
	var files []string
	for _, f := range list.files {
		if !workspace.IsInside(f, m.Path) {
			continue
		}
		inNested := false
		for _, n := range nested {
			if workspace.IsInside(f, n) {
				inNested = true
				break
			}
		}
		if inNested {
			continue
		}
		rel := f
		if m.Path != "." {
			rel = strings.TrimPrefix(f, m.Path+"/")
		}
		if excludedPath(rel) || !wantFile(rel[strings.LastIndexByte(rel, '/')+1:]) {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
}

// wsRelPath joins a member-relative path into a workspace-root-relative one.
func wsRelPath(m *workspace.Member, memberRel string) string {
	if m.Path == "." {
		return memberRel
	}
	return m.Path + "/" + memberRel
}
