//go:build unix

package memoryhealth

import (
	"os"

	"golang.org/x/sys/unix"
)

func pathWritablePlatform(path string) bool {
	if err := unix.Access(path, unix.W_OK); err != nil {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return true
}
