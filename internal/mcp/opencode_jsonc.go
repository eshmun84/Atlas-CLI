package mcp

import (
	"fmt"
	"os"
)

// OpenCodeJSONCRel is the JSONC sibling of the Atlas-owned OpenCode MCP layer.
// OpenCode accepts JSON and JSONC under `.opencode/`; Atlas merges only
// `.opencode/opencode.json` and does not implement JSONC-safe merging.
// Root `opencode.jsonc` is user-owned and may coexist — Atlas does not write it.
const OpenCodeJSONCRel = ".opencode/opencode.jsonc"

// OpenCodeJSONCBlocks reports whether OpenCode MCP reconciliation must abort
// because a JSONC config occupies the Atlas MCP layer path.
// When blocked, callers must not create `.opencode/opencode.json` or mutate ownership.
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
		return true, "opencode: blocked — .opencode/opencode.jsonc is a symlink; Atlas does not merge JSONC MCP configs (convert to .opencode/opencode.json or remove the symlink)", nil
	}
	if info.IsDir() {
		return true, "opencode: blocked — .opencode/opencode.jsonc is a directory; Atlas does not merge JSONC MCP configs", nil
	}
	return true, fmt.Sprintf(
		"opencode: blocked — %s is present; Atlas merges only .opencode/opencode.json and does not implement JSONC-safe MCP merging. Convert to .opencode/opencode.json or remove %s before reconciling OpenCode MCP",
		OpenCodeJSONCRel, OpenCodeJSONCRel,
	), nil
}
