//go:build unix

package fsafety

import "golang.org/x/sys/unix"

func syncDirProd(dir string) error {
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	syncErr := unix.Fsync(fd)
	closeErr := unix.Close(fd)
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
