package home

import (
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
)

// Asset describes one bundled Atlas Home asset.
type Asset struct {
	Family    string // agents | adapters
	ID        string // stable id, usually embed-relative path
	EmbedPath string // path inside assets.Content
}

// BundledAssets is the deterministic catalog mirrored into Atlas Home.
func BundledAssets() []Asset {
	out := []Asset{
		{Family: "agents", ID: "agents/base.md", EmbedPath: "agents/base.md"},
		{Family: "agents", ID: "agents/adapters/cursor.md", EmbedPath: "agents/adapters/cursor.md"},
		{Family: "agents", ID: "agents/adapters/opencode.md", EmbedPath: "agents/adapters/opencode.md"},
		{Family: "adapters", ID: "adapter-files/cursor/atlas.mdc", EmbedPath: "adapter-files/cursor/atlas.mdc"},
		{Family: "adapters", ID: "adapter-files/opencode/atlas.md", EmbedPath: "adapter-files/opencode/atlas.md"},
	}
	for _, name := range assets.AtlasAgentFilenames {
		embed := "agents/runtime/" + name
		out = append(out, Asset{
			Family:    "agents",
			ID:        embed,
			EmbedPath: embed,
		})
	}
	return out
}

// AssetHomePath returns the canonical mirrored path under Atlas Home assets/.
func AssetHomePath(homePath string, asset Asset) string {
	return filepath.Join(AssetsRoot(homePath), filepath.FromSlash(asset.EmbedPath))
}

// ConvenienceHomePath returns an additional discoverable path for selected assets.
// Runtime agents also live under $ATLAS_HOME/agents/<file>.
// Adapter entrypoints also live under $ATLAS_HOME/adapters/<runtime>/<file>.
func ConvenienceHomePath(homePath string, asset Asset) string {
	switch {
	case strings.HasPrefix(asset.EmbedPath, "agents/runtime/"):
		return filepath.Join(homePath, "agents", filepath.Base(asset.EmbedPath))
	case asset.EmbedPath == "adapter-files/cursor/atlas.mdc":
		return filepath.Join(homePath, "adapters", "cursor", "atlas.mdc")
	case asset.EmbedPath == "adapter-files/opencode/atlas.md":
		return filepath.Join(homePath, "adapters", "opencode", "atlas.md")
	default:
		return ""
	}
}
