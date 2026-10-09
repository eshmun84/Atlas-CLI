package mutatelock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalTarget returns a stable physical path key for lock naming.
//
// When path exists: Abs → Clean → EvalSymlinks.
// When path does not exist: canonicalize the nearest existing ancestor, then
// append the remaining clean path components.
func CanonicalTarget(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("mutatelock: path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("mutatelock: abs: %w", err)
	}
	abs = filepath.Clean(abs)

	info, err := os.Lstat(abs)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(abs)
			if err != nil {
				return "", fmt.Errorf("mutatelock: eval symlinks: %w", err)
			}
			return filepath.Clean(resolved), nil
		}
		// Resolve symlink parents even when the leaf is a real file/dir.
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("mutatelock: eval symlinks: %w", err)
		}
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("mutatelock: lstat: %w", err)
	}

	// Missing target: walk up to nearest existing ancestor.
	cur := abs
	var missing []string
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("mutatelock: no existing ancestor for %s", abs)
		}
		base := filepath.Base(cur)
		missing = append([]string{base}, missing...)
		info, err := os.Lstat(parent)
		if os.IsNotExist(err) {
			cur = parent
			continue
		}
		if err != nil {
			return "", fmt.Errorf("mutatelock: lstat ancestor: %w", err)
		}
		_ = info
		resolved, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return "", fmt.Errorf("mutatelock: eval ancestor: %w", err)
		}
		return filepath.Clean(filepath.Join(append([]string{resolved}, missing...)...)), nil
	}
}
