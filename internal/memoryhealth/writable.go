package memoryhealth

import (
	"path/filepath"
	"strings"
)

// pathWritable reports whether path appears writable by the current process.
// It inspects permissions only: it never creates a path and never opens Memory
// storage for writing, because a read-write SQLite open would create WAL/SHM
// sidecars. A positive result is not a guarantee that a later write succeeds.
func pathWritable(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return false
	}
	return pathWritablePlatform(path)
}
