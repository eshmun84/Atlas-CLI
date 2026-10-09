package mcp

import (
	"fmt"
	"os"
)

// OpenCodeJSONCRel is the OpenCode JSONC project config path.
// OpenCode accepts JSON and JSONC; Atlas Slice 32 only merges opencode.json.
const OpenCodeJSONCRel = "opencode.jsonc"

// OpenCodeJSONCBlocks reports whether OpenCode MCP reconciliation must abort
// because a supported JSONC config is present and Atlas cannot merge it safely.
// When blocked, callers must not create opencode.json or mutate ownership.
func OpenCodeJSONCBlocks(root string) (blocked bool, message string, err error) {
	full, joinErr := ContainedJoin(root, OpenCodeJSONCRel)
	if joinErr != nil {
		return false, "", joinErr
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true, "opencode: blocked — opencode.jsonc is a symlink; Atlas does not merge JSONC MCP configs (convert to opencode.json or remove the symlink)", nil
	}
	if info.IsDir() {
		return true, "opencode: blocked — opencode.jsonc is a directory; Atlas does not merge JSONC MCP configs", nil
	}
	return true, fmt.Sprintf(
		"opencode: blocked — %s is present; Atlas Slice 32 merges only opencode.json and does not implement JSONC-safe MCP merging. Convert to opencode.json or remove %s before reconciling OpenCode MCP",
		OpenCodeJSONCRel, OpenCodeJSONCRel,
	), nil
}
