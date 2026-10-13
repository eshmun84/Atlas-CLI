//go:build darwin || linux

package codegraphsource

import (
	"errors"
	"fmt"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"golang.org/x/sys/unix"
)

// SnapshotOpenStrategy identifies how snapshot source leaves are opened.
// Darwin/Linux open the leaf with O_NOFOLLOW so a symlink substitution of the
// final path component cannot be followed (eliminates Lstat→Open TOCTOU on the leaf).
const SnapshotOpenStrategy = "open_nofollow"

// openRegularNoFollow opens path without following a symlink leaf.
// Intermediate directory components of an absolute path may still be resolved
// by the kernel; production callers pass InspectStorageLeaf-validated Home paths.
func openRegularNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, fmt.Errorf("%w: %s", fsafety.ErrSymlink, path)
		}
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
