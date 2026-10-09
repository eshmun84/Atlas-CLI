package mcp

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// ContainedJoin resolves root/rel for writing while rejecting path traversal and
// any symlink in the chain from project root to the destination.
func ContainedJoin(root, rel string) (string, error) {
	full, err := fsafety.ContainedJoin(root, rel)
	if err != nil {
		return "", fmt.Errorf("mcp: %s", trimFsafetyPrefix(err.Error()))
	}
	return full, nil
}

// EnsureContainedDir creates parent directories for a contained path using only
// real (non-symlink) components under root.
func EnsureContainedDir(root, relFile string) (string, error) {
	full, err := fsafety.EnsureContainedDir(root, relFile, 0o755)
	if err != nil {
		return "", fmt.Errorf("mcp: %s", trimFsafetyPrefix(err.Error()))
	}
	return full, nil
}

func trimFsafetyPrefix(msg string) string {
	return strings.TrimPrefix(msg, "fsafety: ")
}
