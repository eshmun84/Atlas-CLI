package fsafety

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrDirSyncUnsupported is returned when the operating system cannot sync a
// directory. File-data sync remains mandatory. Callers must not describe the
// directory entry as crash-durable in that case.
var ErrDirSyncUnsupported = errors.New("fsafety: directory sync unsupported")

var syncFileFn = func(f *os.File) error { return f.Sync() }

var syncDirFn = syncDirProd

// SetDurableSyncForTest replaces file and directory sync used by durable
// publication. A nil function restores the production implementation.
func SetDurableSyncForTest(fileFn func(*os.File) error, dirFn func(string) error) {
	if fileFn == nil {
		syncFileFn = func(f *os.File) error { return f.Sync() }
	} else {
		syncFileFn = fileFn
	}
	if dirFn == nil {
		syncDirFn = syncDirProd
	} else {
		syncDirFn = dirFn
	}
}

// SyncFileDurable syncs file data through the process-wide seam.
func SyncFileDurable(f *os.File) error {
	if f == nil {
		return fmt.Errorf("fsafety: sync file: nil")
	}
	return syncFileFn(f)
}

// SyncDirDurable syncs the directory that contains a committed name.
func SyncDirDurable(dir string) error {
	return syncDirFn(dir)
}

// AtomicReplaceDurable is AtomicReplace plus file and directory sync.
// The rename is still the atomicity boundary. Sync failure before rename
// leaves the destination untouched. Sync failure after a successful rename
// returns the error and does not try to undo the replacement.
func AtomicReplaceDurable(path string, data []byte, perm os.FileMode, tmpPattern string) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("fsafety write: path must be absolute")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fsafety write: refusing to write through symlink %s", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("fsafety write: lstat: %w", err)
	}
	dir := filepath.Dir(path)
	if strings.TrimSpace(tmpPattern) == "" {
		tmpPattern = ".atlas-write-*.tmp"
	}
	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return fmt.Errorf("fsafety write: temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsafety write: temp write: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsafety write: chmod: %w", err)
	}
	if err := syncFileFn(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsafety write: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("fsafety write: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("fsafety write: rename: %w", err)
	}
	cleanup = false
	if err := syncDirFn(dir); err != nil {
		if errors.Is(err, ErrDirSyncUnsupported) {
			return nil
		}
		return fmt.Errorf("fsafety write: sync dir: %w", err)
	}
	return nil
}

// AtomicWriteContainedDirDurable is AtomicWriteContainedDir with
// AtomicReplaceDurable. Containment rules are unchanged.
func AtomicWriteContainedDirDurable(root, rel string, data []byte, filePerm, dirPerm os.FileMode, tmpPattern string) error {
	full, _, err := EnsureContainedDirCreated(root, rel, dirPerm)
	if err != nil {
		return err
	}
	return AtomicReplaceDurable(full, data, filePerm, tmpPattern)
}
