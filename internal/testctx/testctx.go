// Package testctx implements the shared test-context predicate (DESIGN.md
// section 6.4): one definition of "non-production", computed on the
// slash-separated path relative to the project root, shared by every check
// and every extractor. Root-relative matching is deliberate — a production
// src/test/ directory is not test code (lesson 6).
package testctx

import "strings"

// unconditionalDirs match as any path component, at any depth (lesson 7).
var unconditionalDirs = map[string]bool{
	"__tests__": true,
	"testdata":  true,
}

// rootDirs match as the first path component only (lesson 8).
var rootDirs = map[string]bool{
	"test":             true,
	"tests":            true,
	"example":          true,
	"examples":         true,
	"integration_test": true,
}

// IsTestContext reports whether the file at relPath (relative to the project
// root, slash-separated) is test code.
func IsTestContext(relPath string) bool {
	parts := strings.Split(relPath, "/")
	if len(parts) > 1 && rootDirs[parts[0]] {
		return true
	}
	// Directory components (everything but the file name).
	for _, dir := range parts[:len(parts)-1] {
		if unconditionalDirs[dir] {
			return true
		}
	}
	return isTestFileName(parts[len(parts)-1])
}

// isTestFileName applies the file-name patterns: test_*.py, *_test.py,
// conftest.py, *_test.go, *.test.[jt]sx?, *.spec.[jt]sx?.
func isTestFileName(name string) bool {
	switch {
	case strings.HasSuffix(name, ".py"):
		stem := strings.TrimSuffix(name, ".py")
		return strings.HasPrefix(name, "test_") ||
			strings.HasSuffix(stem, "_test") ||
			name == "conftest.py"
	case strings.HasSuffix(name, ".go"):
		return strings.HasSuffix(name, "_test.go")
	}
	// *.test.<ext> / *.spec.<ext> for ts, tsx, js, jsx.
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx"} {
		if strings.HasSuffix(name, ext) {
			stem := strings.TrimSuffix(name, ext)
			return strings.HasSuffix(stem, ".test") || strings.HasSuffix(stem, ".spec")
		}
	}
	return false
}
