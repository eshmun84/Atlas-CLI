//go:build !darwin && !linux

package cesource

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// OpenStrategy identifies the pack open implementation selected by build tags.
//
// Platforms other than macOS/Linux lack a supported openat+O_NOFOLLOW path in
// this package. The fallback reuses ContainedJoin (symlink parents at check
// time), Lstat (symlink leaf / non-regular), Open, and Fstat. That is not
// race-free against concurrent parent-directory symlink substitution between
// ContainedJoin and Open; Atlas production targets are darwin/linux.
const OpenStrategy = "lstat_open_fallback"

// openContainedRegular opens homePath/rel after ContainedJoin validation.
// Fallback for non-darwin/non-linux hosts only.
func openContainedRegular(homePath, rel string) (*os.File, error) {
	full, err := fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: leaf %s", fsafety.ErrSymlink, filepath.ToSlash(rel))
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, filepath.ToSlash(rel))
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, filepath.ToSlash(rel))
	}
	return f, nil
}
