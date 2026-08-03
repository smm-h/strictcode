// Package strictcode is the module-root package whose sole job is to embed
// the VERSION file — the single source of truth for the release version — so
// the CLI derives its version from the exact file rlsbl's Go release target
// bumps during `rlsbl release run`. (go:embed cannot reference ../VERSION
// from a subpackage, so the embedding file lives at the module root.)
package strictcode

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version is the strictcode release version, trimmed of the trailing newline
// the VERSION file carries.
var Version = strings.TrimSpace(versionFile)
