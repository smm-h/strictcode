package fixture

import "strings"

// DeclarationsPath is where rlsbl keeps a repository's release declarations,
// the file strictcode reads workspace members from (the workspace package's
// DeclarationsFile; spelled again here because the workspace package's own
// tests import this one).
const DeclarationsPath = ".strictmetadata/releasables/releasables.toml"

// DeclarationsHeader is the top of a workspace-layout declarations file;
// fixtures append their [[members]] tables to it.
const DeclarationsHeader = "format_version = 1\nrepository_layout = \"workspace\"\nrelease_branches = [\"main\"]\n"

// Workspace renders a workspace-layout declarations file holding one
// [[members]] table per argument, each the table's body. A body that states
// no releasable gets releasable = false.
func Workspace(members ...string) string {
	var b strings.Builder
	b.WriteString(DeclarationsHeader)
	for _, m := range members {
		b.WriteString("\n[[members]]\n")
		b.WriteString(m)
		if !strings.Contains(m, "releasable =") {
			b.WriteString("releasable = false\n")
		}
	}
	return b.String()
}
