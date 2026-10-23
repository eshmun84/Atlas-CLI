//go:build unix

package hookactions

import (
	"errors"
	"fmt"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"golang.org/x/sys/unix"
)

// openLifecycleLock opens a lock leaf without following a symlink.
// O_NOFOLLOW applies to the final component. A hostile replacement of a parent
// directory after SafeMkdirAll is outside this defense; see the stabilization
// threat model.
func openLifecycleLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, mapLifecycleOpenErr(path, err)
	}
	return regularFileFromFD(path, fd)
}

// createExclusiveNoFollow creates a new regular file and fails if the leaf
// exists or is a symlink. It does not write any bytes.
func createExclusiveNoFollow(path string, perm os.FileMode) (*os.File, error) {
	if perm == 0 {
		perm = 0o600
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, uint32(perm))
	if err != nil {
		return nil, mapLifecycleOpenErr(path, err)
	}
	return regularFileFromFD(path, fd)
}

func mapLifecycleOpenErr(path string, err error) error {
	switch {
	case errors.Is(err, unix.ELOOP):
		return fmt.Errorf("%w: %s", fsafety.ErrSymlink, path)
	case errors.Is(err, unix.EEXIST):
		return fmt.Errorf("%w: %s", os.ErrExist, path)
	default:
		return err
	}
}

func regularFileFromFD(path string, fd int) (*os.File, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, path)
	}
	return os.NewFile(uintptr(fd), path), nil
}
