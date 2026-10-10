package skills

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
)

// BundledPin is one shipped skill package identity.
type BundledPin struct {
	ID      string
	Version string
}

// DefaultPins is the minimal v1 enabled set proving the architecture.
func DefaultPins() []Pin {
	out := make([]Pin, 0, len(BundledPackages()))
	for _, b := range BundledPackages() {
		out = append(out, Pin{ID: b.ID, Version: b.Version})
	}
	return out
}

// BundledPackages returns the deterministic bundled skill set.
func BundledPackages() []BundledPin {
	return []BundledPin{
		{ID: "testing", Version: "1.0.0"},
		{ID: "code-review", Version: "1.0.0"},
		{ID: "security-review", Version: "1.0.0"},
		{ID: "documentation", Version: "1.0.0"},
	}
}

// EmbedPackagePrefix returns the embed-relative package directory.
func EmbedPackagePrefix(id, version string) string {
	return filepath.ToSlash(filepath.Join("skills", id, version))
}

// ListBundledEmbedFiles returns embed paths for all bundled skill package files.
func ListBundledEmbedFiles() ([]string, error) {
	var out []string
	err := fs.WalkDir(assets.Content, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		out = append(out, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("skills: list bundled: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

// ReadBundledSkillMD loads SKILL.md from the embed FS for a bundled pin.
func ReadBundledSkillMD(id, version string) (string, error) {
	path := EmbedPackagePrefix(id, version) + "/" + FileSkillMD
	data, err := assets.Content.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("skills: bundled %s@%s: %w", id, version, err)
	}
	return string(data), nil
}

// AgentSkillRefs maps Atlas agent ids to canonical skill ids (no provider paths).
func AgentSkillRefs(agentID string) []string {
	agentID = strings.TrimSpace(agentID)
	switch agentID {
	case "atlas-orchestrator":
		return []string{"testing", "code-review", "security-review", "documentation"}
	case "atlas-worker":
		return []string{"testing", "documentation"}
	case "atlas-review-quality":
		return []string{"code-review", "testing"}
	case "atlas-review-architecture":
		return []string{"code-review"}
	case "atlas-review-risk":
		return []string{"security-review"}
	case "atlas-review-refuter":
		return []string{"code-review", "security-review"}
	default:
		if strings.HasPrefix(agentID, "atlas-sdd-") {
			return []string{"documentation", "testing"}
		}
		return nil
	}
}
