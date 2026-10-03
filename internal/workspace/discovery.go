package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscoveryResult is the read-only snapshot of a workspace.
type DiscoveryResult struct {
	RootPath string

	Git          GitInfo
	Files        FileInfo
	Technologies []Technology
	Tools        []ToolInfo

	Warnings []string
}

// Discover inspects root using read-only filesystem and Git operations.
func Discover(root string) (DiscoveryResult, error) {
	if strings.TrimSpace(root) == "" {
		return DiscoveryResult{}, fmt.Errorf("root path is required")
	}

	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return DiscoveryResult{}, fmt.Errorf("root path does not exist: %s", root)
		}
		return DiscoveryResult{}, fmt.Errorf("stat root path: %w", err)
	}
	if !info.IsDir() {
		return DiscoveryResult{}, fmt.Errorf("root path is not a directory: %s", root)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("resolve root path: %w", err)
	}

	result := DiscoveryResult{
		RootPath: absRoot,
	}

	files, err := DiscoverFiles(absRoot)
	if err != nil {
		return DiscoveryResult{}, err
	}
	result.Files = files
	result.Technologies = DiscoverTechnologies(files)
	result.Tools = DiscoverTools()

	gitInfo, warnings, err := discoverGit(absRoot)
	if err != nil {
		return DiscoveryResult{}, err
	}
	result.Git = gitInfo
	result.Warnings = append(result.Warnings, warnings...)

	return result, nil
}
