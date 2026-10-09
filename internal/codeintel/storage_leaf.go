package codeintel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// StorageLeaf is a symlink-safe view of an Atlas Home storage file.
type StorageLeaf struct {
	Present bool
	Path    string // absolute contained path
}

// InspectStorageLeaf inspects graph.db / metadata.json under an explicit Atlas Home.
// Absent → Present=false, nil.
// Symlink, directory, non-regular, or containment failure → error.
func InspectStorageLeaf(homePath, leafPath string) (StorageLeaf, error) {
	homePath = strings.TrimSpace(homePath)
	leafPath = strings.TrimSpace(leafPath)
	if homePath == "" {
		return StorageLeaf{}, fmt.Errorf("codeintel: storage inspect requires Atlas Home path")
	}
	if leafPath == "" {
		return StorageLeaf{}, fmt.Errorf("codeintel: storage leaf path is required")
	}
	homeAbs, err := filepath.Abs(homePath)
	if err != nil {
		return StorageLeaf{}, fmt.Errorf("codeintel: resolve Atlas Home: %w", err)
	}
	leafAbs, err := filepath.Abs(leafPath)
	if err != nil {
		return StorageLeaf{}, fmt.Errorf("codeintel: resolve storage leaf: %w", err)
	}
	rel, err := filepath.Rel(homeAbs, leafAbs)
	if err != nil {
		return StorageLeaf{}, fmt.Errorf("codeintel: storage containment: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return StorageLeaf{}, fmt.Errorf("codeintel: storage leaf escapes Atlas Home")
	}
	full, err := fsafety.ContainedJoin(homeAbs, rel)
	if err != nil {
		return StorageLeaf{}, fmt.Errorf("codeintel: storage leaf unsafe: %w", err)
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return StorageLeaf{Present: false, Path: full}, nil
	}
	if err != nil {
		return StorageLeaf{}, fmt.Errorf("codeintel: lstat storage leaf: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return StorageLeaf{}, fmt.Errorf("codeintel: storage leaf is a symlink: %s", rel)
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return StorageLeaf{}, fmt.Errorf("codeintel: storage leaf is not a regular file: %s", rel)
	}
	return StorageLeaf{Present: true, Path: full}, nil
}

// ReadStorageLeaf reads a regular non-symlink storage leaf under Home.
// Absent → (nil, false, nil). Unsafe → error (never follows symlink target).
func ReadStorageLeaf(homePath, leafPath string) ([]byte, bool, error) {
	leaf, err := InspectStorageLeaf(homePath, leafPath)
	if err != nil {
		return nil, false, err
	}
	if !leaf.Present {
		return nil, false, nil
	}
	data, err := os.ReadFile(leaf.Path)
	if err != nil {
		return nil, false, fmt.Errorf("codeintel: read storage leaf: %w", err)
	}
	return data, true, nil
}
