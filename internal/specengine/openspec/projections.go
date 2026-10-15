package openspec

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Marker paths that prove OpenSpec tool projections exist for an adapter.
// These are OpenSpec-owned; Atlas must never create them by hand.
var adapterProjectionMarkers = map[string][]string{
	"cursor": {
		".cursor/commands/opsx-propose.md",
		".cursor/skills/openspec-propose/SKILL.md",
	},
	"opencode": {
		".opencode/commands/opsx-propose.md",
		".opencode/skills/openspec-propose/SKILL.md",
	},
}

// MissingToolProjections returns selected adapters whose OpenSpec-generated
// commands/skills markers are absent. Empty when tools==none or all present.
func MissingToolProjections(root string, adapters []string) []string {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil
	}
	var missing []string
	seen := map[string]bool{}
	for _, a := range adapters {
		id := strings.ToLower(strings.TrimSpace(a))
		markers, ok := adapterProjectionMarkers[id]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		if !hasAllMarkers(root, markers) {
			missing = append(missing, id)
		}
	}
	return missing
}

// NeedsToolUpdate reports whether a ready OpenSpec project should run
// `openspec update` to (re)project tool integrations for selected adapters.
func NeedsToolUpdate(root string, adapters []string) bool {
	if ToolsForAdapters(adapters) == "none" {
		return false
	}
	st, _ := ClassifyProject(root)
	if st != ProjectReady {
		return false
	}
	return len(MissingToolProjections(root, adapters)) > 0
}

func hasAllMarkers(root string, markers []string) bool {
	for _, rel := range markers {
		info, err := fsafety.LstatContained(root, rel)
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}
