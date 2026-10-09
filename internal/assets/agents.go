package assets

import (
	"fmt"
	"strings"
)

// AtlasAgentFilenames is the canonical Atlas-owned runtime agent pack.
// Order is stable for registry, manifest, lock, and materialization.
var AtlasAgentFilenames = []string{
	"atlas-orchestrator.md",
	"atlas-sdd-init.md",
	"atlas-sdd-explore.md",
	"atlas-sdd-research.md",
	"atlas-sdd-propose.md",
	"atlas-sdd-update.md",
	"atlas-sdd-implement.md",
	"atlas-sdd-verify.md",
	"atlas-sdd-archive.md",
	"atlas-review-architecture.md",
	"atlas-review-risk.md",
	"atlas-review-quality.md",
	"atlas-review-refuter.md",
	"atlas-worker.md",
}

// ReadRuntimeAgent returns one embedded Atlas runtime agent markdown file.
func ReadRuntimeAgent(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid atlas agent filename %q", filename)
	}
	if !IsAtlasAgentFilename(name) {
		return "", fmt.Errorf("unknown atlas agent %q", filename)
	}
	data, err := Content.ReadFile("agents/runtime/" + name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsAtlasAgentFilename reports whether name is in the Atlas agent pack.
func IsAtlasAgentFilename(name string) bool {
	name = strings.TrimSpace(name)
	for _, known := range AtlasAgentFilenames {
		if known == name {
			return true
		}
	}
	return false
}
