package memoryhealth

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// pathWritable reports whether path is writable by the current process
// without opening SQLite (opening RW would create WAL/SHM sidecars).
func pathWritable(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return false
	}
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
