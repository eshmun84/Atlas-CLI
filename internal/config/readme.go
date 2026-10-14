package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

const FileREADME = "README.md"

// RootREADMEContent returns the greenfield README body for a project name.
func RootREADMEContent(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "project"
	}
	return "# " + name + "\n"
}

// EnsureRootREADME creates README.md once when absent. Never modifies an existing file.
func EnsureRootREADME(root, projectName string) (created bool, fp fsafety.TransactionFootprint, err error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return false, fp, fmt.Errorf("readme: workspace root is required")
	}
	full, err := fsafety.ContainedJoin(root, FileREADME)
	if err != nil {
		return false, fp, err
	}
	info, statErr := os.Lstat(full)
	if statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fp, fmt.Errorf("readme: refusing symlink %s", FileREADME)
		}
		// Existing file or directory: preserve (brownfield). Do not mutate.
		return false, fp, nil
	}
	if !os.IsNotExist(statErr) {
		return false, fp, fmt.Errorf("readme: lstat: %w", statErr)
	}

	content := RootREADMEContent(projectName)
	wrote, dirs, writeErr := fsafety.AtomicWriteContainedTracked(root, FileREADME, []byte(content), 0o644, 0o755, ".atlas-readme-*.tmp")
	fp.MergeDirs(dirs)
	if writeErr != nil {
		return false, fp, fmt.Errorf("readme: write: %w", writeErr)
	}
	fp.AddFile(wrote)
	return true, fp, nil
}
