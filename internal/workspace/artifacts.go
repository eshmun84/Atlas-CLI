package workspace

import "path/filepath"

// Recognized runtime/adaptor artifact relative paths.
var runtimeArtifactNames = []string{
	"AGENTS.md",
	"AGENT.md",
	"CLAUDE.md",
	"GEMINI.md",
	".cursor",
	".opencode",
	".claude",
	".agents",
	".codex",
}

// DiscoverRuntimeArtifacts lists recognized agent/adaptor artifacts. Read-only.
func DiscoverRuntimeArtifacts(root string) []string {
	var found []string
	for _, name := range runtimeArtifactNames {
		if exists(root, name) {
			found = append(found, filepath.ToSlash(name))
		}
	}
	return found
}
