package config

import (
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func init() {
	mcp.RegisterDefaultProjector(cursor.New())
	mcp.RegisterDefaultProjector(opencode.New())
}

// MCPProjectors returns the registered Cursor/OpenCode projectors.
func MCPProjectors() map[mcp.AdapterID]mcp.Projector {
	return mcp.DefaultProjectors()
}

// SelectedMCPAdapters maps persisted adapter IDs onto MCP adapter IDs.
func SelectedMCPAdapters(selected []string) []mcp.AdapterID {
	out := make([]mcp.AdapterID, 0, len(selected))
	for _, a := range selected {
		switch a {
		case "cursor":
			out = append(out, mcp.AdapterCursor)
		case "opencode":
			out = append(out, mcp.AdapterOpenCode)
		}
	}
	return out
}
