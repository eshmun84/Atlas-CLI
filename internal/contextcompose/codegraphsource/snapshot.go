package codegraphsource

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

const (
	// maxSnapshotBytes bounds a single snapshot leaf copy to limit temp disk use.
	maxSnapshotBytes = 512 << 20 // 512 MiB
)

// querySnapshot is an isolated CodeGraph DB copy used for retrieval so the
// external CLI never opens Atlas-managed live storage (which would create
// SQLite WAL/SHM sidecars beside graph.db).
type querySnapshot struct {
	Dir string
	DB  string
}

// prepareQuerySnapshot copies the live graph.db (and any existing regular
// -wal/-shm sidecars) into a uniquely owned temp directory. The caller MUST
// invoke cleanup, which removes only that temp directory.
//
// Source leaves are opened without following symlink leaves (see
// SnapshotOpenStrategy / openRegularNoFollow). Missing sidecars are skipped;
// present symlink or non-regular sidecars fail closed.
func prepareQuerySnapshot(liveDB string) (snap querySnapshot, cleanup func(), err error) {
	liveDB = strings.TrimSpace(liveDB)
	noop := func() {}
	if liveDB == "" {
		return querySnapshot{}, noop, fmt.Errorf("codegraphsource: live graph.db path is empty")
	}

	dir, err := os.MkdirTemp("", "atlas-cg-query-*")
	if err != nil {
		return querySnapshot{}, noop, fmt.Errorf("codegraphsource: create query snapshot dir: %w", err)
	}
	cleanup = func() {
		_ = os.RemoveAll(dir)
	}
	ok := false
	defer func() {
		if !ok {
			cleanup()
			cleanup = noop
		}
	}()

	dstDB := filepath.Join(dir, "graph.db")
	if err := copyRegularFileBounded(liveDB, dstDB, maxSnapshotBytes); err != nil {
		return querySnapshot{}, noop, fmt.Errorf("codegraphsource: snapshot graph.db: %w", err)
	}

	// Copy existing SQLite sidecars when present so the snapshot matches the
	// on-disk WAL-mode state. Absent sidecars are fine (fresh post-build DB).
	for _, suffix := range []string{"-wal", "-shm"} {
		src := liveDB + suffix
		dst := filepath.Join(dir, "graph.db"+suffix)
		cerr := copyRegularFileBounded(src, dst, maxSnapshotBytes)
		if cerr == nil {
			continue
		}
		if isNotExist(cerr) {
			continue
		}
		return querySnapshot{}, noop, fmt.Errorf("codegraphsource: snapshot %s: %w", filepath.Base(src), cerr)
	}

	ok = true
	return querySnapshot{Dir: dir, DB: dstDB}, cleanup, nil
}

// copyRegularFileBounded opens src without following a symlink leaf, Fstats
// the opened descriptor, requires a regular file within maxBytes, and copies
// through LimitReader(maxBytes+1). Destination is created exclusively.
func copyRegularFileBounded(src, dst string, maxBytes int64) error {
	if maxBytes <= 0 {
		return fmt.Errorf("invalid snapshot bound")
	}
	in, err := openRegularNoFollow(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat() // Fstat of the opened descriptor
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, src)
	}
	if info.Size() > maxBytes {
		return fmt.Errorf("file exceeds snapshot bound (%d bytes)", maxBytes)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	copied, copyErr := io.Copy(out, io.LimitReader(in, maxBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if copied > maxBytes {
		_ = os.Remove(dst)
		return fmt.Errorf("file exceeds snapshot bound (%d bytes)", maxBytes)
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}

func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	// unix.Open wraps ENOENT; unwrap for missing sidecars.
	return errors.Is(err, os.ErrNotExist)
}
