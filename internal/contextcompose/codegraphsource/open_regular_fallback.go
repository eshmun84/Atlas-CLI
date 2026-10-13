//go:build !darwin && !linux

package codegraphsource

import (
	"fmt"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// SnapshotOpenStrategy identifies how snapshot source leaves are opened.
//
// Non-darwin/non-linux hosts lack O_NOFOLLOW in this package. The fallback
// uses Lstat then Open then Fstat. That retains a residual TOCTOU window if
// the leaf is replaced with a symlink between Lstat and Open. Atlas
// production targets are darwin/linux (see SnapshotOpenStrategy on those builds).
const SnapshotOpenStrategy = "lstat_open_fallback"

func openRegularNoFollow(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s", fsafety.ErrSymlink, path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, path)
	}
	f, err := os.Open(path)
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
		return nil, fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, path)
	}
	return f, nil
}
