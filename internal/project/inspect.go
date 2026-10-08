package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Inspect builds a read-only project Snapshot for root.
// It never mutates the filesystem or Git and never probes Atlas runtime health
// (that belongs to internal/runtime via the inspect.Inspect composition).
func Inspect(root string) (Snapshot, error) {
	if strings.TrimSpace(root) == "" {
		return Snapshot{}, fmt.Errorf("root path is required")
	}

	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, fmt.Errorf("root path does not exist: %s", root)
		}
		return Snapshot{}, fmt.Errorf("stat root path: %w", err)
	}
	if !info.IsDir() {
		return Snapshot{}, fmt.Errorf("root path is not a directory: %s", root)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve root path: %w", err)
	}

	files, err := DiscoverFiles(absRoot)
	if err != nil {
		return Snapshot{}, err
	}

	gitInfo, warnings, err := discoverGit(absRoot)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		RootPath:         absRoot,
		Git:              gitInfo,
		Files:            files,
		Technologies:     DiscoverTechnologies(files),
		Libraries:        DiscoverLibraries(absRoot, files),
		RuntimeArtifacts: DiscoverRuntimeArtifacts(absRoot),
		Atlas:            EvaluateAtlasStatus(absRoot, files),
		Tools:            DiscoverTools(),
		Warnings:         warnings,
	}, nil
}
