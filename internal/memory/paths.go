package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// FileMemoryDB is the canonical database filename under a project Memory dir.
const FileMemoryDB = "memory.db"

// DatabaseRel returns the Atlas Home-relative path to memory.db for projectID.
// Format: projects/<project-id>/memory/memory.db
func DatabaseRel(projectID string) (string, error) {
	if err := ValidateProjectID(projectID); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(home.DirProjects, strings.TrimSpace(projectID), home.ProjectDirMemory, FileMemoryDB)), nil
}

// MemoryDirRel returns projects/<project-id>/memory.
func MemoryDirRel(projectID string) (string, error) {
	if err := ValidateProjectID(projectID); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(home.DirProjects, strings.TrimSpace(projectID), home.ProjectDirMemory)), nil
}

// resolveDBPath resolves and validates the contained absolute DB path under Atlas Home.
// Never creates directories. Rejects symlink parents/leaves and traversal.
func resolveDBPath(homePath, projectID string) (abs string, rel string, err error) {
	homePath = filepath.Clean(strings.TrimSpace(homePath))
	if homePath == "" || homePath == "." {
		return "", "", fmt.Errorf("memory: atlas home path is required")
	}
	rel, err = DatabaseRel(projectID)
	if err != nil {
		return "", "", err
	}
	abs, err = fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return "", "", fmt.Errorf("%w: resolve database path: %v", ErrUnsafePath, err)
	}
	return abs, rel, nil
}

// Exists reports whether memory.db is present for the project.
// Read-only: never creates memory/, memory.db, or any parent directories.
func Exists(homePath, projectID string) (bool, error) {
	_, rel, err := resolveDBPath(homePath, projectID)
	if err != nil {
		return false, err
	}
	info, err := fsafety.LstatContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, ErrSymlinkDatabase
	}
	if !info.Mode().IsRegular() {
		return false, ErrNotRegularDatabase
	}
	return true, nil
}

// ensureMemoryDir creates the contained memory directory (mutating).
func ensureMemoryDir(homePath, projectID string) (dirRel string, err error) {
	dirRel, err = MemoryDirRel(projectID)
	if err != nil {
		return "", err
	}
	if _, err := fsafety.ContainedJoin(homePath, dirRel); err != nil {
		return "", fmt.Errorf("memory: memory dir: %w", err)
	}
	if err := fsafety.SafeMkdirAll(homePath, dirRel, home.DirPermHome); err != nil {
		return "", fmt.Errorf("memory: create memory dir: %w", err)
	}
	return dirRel, nil
}
