package cesource

import (
	"errors"
	"fmt"
	"io"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// MaxStoredPackBytes caps on-disk Context Economy pack YAML reads.
//
// Justification: a legitimate pack has at most MaxStoredPackCandidates (24)
// entries. Worst-case UTF-8 field mass for path/reason/kind plus YAML structure
// and a long objective is on the order of ~100 KiB. 256 KiB provides headroom
// for Unicode and metadata without permitting multi-megabyte YAML bombs.
const MaxStoredPackBytes = 256 * 1024

var errPackOversized = errors.New("cesource: context economy pack exceeds maximum size")

// readPackBounded reads a contained regular pack file with a hard byte bound.
//
// Sequence:
//  1. fsafety.ContainedJoin — path shape + containment policy (rejects absolute,
//     traversal, and symlink path components observed at check time).
//  2. openContainedRegular — platform open (darwin/linux: openat+O_NOFOLLOW walk;
//     other: documented Lstat/Open/Fstat fallback).
//  3. Fstat — reject non-regular; size is a fast-reject hint only.
//  4. LimitReader(max+1) — authoritative bound against concurrent growth.
//
// Pre-open path checks alone are not treated as race-free containment; on
// darwin/linux the openat walk is what closes leaf and parent symlink swaps.
func readPackBounded(homePath, rel string) ([]byte, error) {
	if _, err := fsafety.ContainedJoin(homePath, rel); err != nil {
		return nil, err
	}

	f, err := openContainedRegular(homePath, rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, rel)
	}
	// Fast reject when the opened inode already reports an oversized size.
	// Not sufficient alone — LimitReader below remains authoritative.
	if st.Size() > MaxStoredPackBytes {
		return nil, errPackOversized
	}

	data, err := io.ReadAll(io.LimitReader(f, int64(MaxStoredPackBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxStoredPackBytes {
		return nil, errPackOversized
	}
	return data, nil
}
