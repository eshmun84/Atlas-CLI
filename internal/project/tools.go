package project

import "os/exec"

// ToolInfo describes local availability of a development tool.
type ToolInfo struct {
	Name      string
	Available bool
	Path      string
}

// RequiredTools is the fixed set of tools inspected in this slice.
var RequiredTools = []string{
	"git",
	"go",
	"gh",
	"openspec",
	"cursor",
	"opencode",
}

// DiscoverTools reports PATH availability for required tools.
// Tools are located with LookPath only; they are not executed.
func DiscoverTools() []ToolInfo {
	tools := make([]ToolInfo, 0, len(RequiredTools))
	for _, name := range RequiredTools {
		info := ToolInfo{Name: name}
		if path, err := exec.LookPath(name); err == nil {
			info.Available = true
			info.Path = path
		}
		tools = append(tools, info)
	}
	return tools
}
