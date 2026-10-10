package openspec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// readDirContained lists direct children under root/rel using fsafety only.
// Symlink parents or leaves fail closed. Does not follow symlinks.
func readDirContained(root, rel string) ([]os.DirEntry, error) {
	abs, err := fsafety.ContainedJoin(root, rel)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var out []os.DirEntry
	for _, e := range ents {
		childRel := filepath.ToSlash(filepath.Join(rel, e.Name()))
		info, err := fsafety.LstatContained(root, childRel)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("openspec: refusing symlink %s", childRel)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

// dirExistsContained reports whether rel is a real (non-symlink) directory.
// Missing path → (false, nil). Unsafe symlink/traversal → error.
func dirExistsContained(root, rel string) (bool, error) {
	info, err := fsafety.LstatContained(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// fileExistsContained reports whether rel is a regular non-symlink file.
func fileExistsContained(root, rel string) (bool, error) {
	info, err := fsafety.LstatContained(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}
