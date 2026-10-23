package hookactions

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// publishExclusiveDurable creates a new evidence file and commits it only
// after the bytes and, when the OS supports it, the directory entry have been
// synced. Any failure before that commit removes the new file.
func publishExclusiveDurable(path string, data []byte) error {
	f, err := createExclusiveNoFollow(path, 0o600)
	if err != nil {
		return err
	}
	committed := false
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		if !committed {
			_ = os.Remove(path)
		}
	}()
	n, err := f.Write(data)
	if err != nil {
		return fmt.Errorf("hookactions: evidence write: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("hookactions: evidence short write: %d/%d", n, len(data))
	}
	if err := fsafety.SyncFileDurable(f); err != nil {
		return fmt.Errorf("hookactions: evidence sync: %w", err)
	}
	if err := f.Close(); err != nil {
		closed = true
		return fmt.Errorf("hookactions: evidence close: %w", err)
	}
	closed = true
	if err := fsafety.SyncDirDurable(filepath.Dir(path)); err != nil {
		if errors.Is(err, fsafety.ErrDirSyncUnsupported) {
			committed = true
			return nil
		}
		return fmt.Errorf("hookactions: evidence sync dir: %w", err)
	}
	committed = true
	return nil
}

func publishUniqueEvidence(homePath, projectID, prefix string, data []byte) (string, error) {
	if err := fsafety.SafeMkdirAll(homePath, pendingStateDirRel(projectID), home.DirPermHome); err != nil {
		return "", fmt.Errorf("hookactions: evidence dir: %w", err)
	}
	for attempt := 0; attempt < 16; attempt++ {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", fmt.Errorf("hookactions: evidence id: %w", err)
		}
		name := prefix + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(suffix[:]) + ".yaml"
		rel := filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, name))
		abs, err := fsafety.ContainedJoin(homePath, rel)
		if err != nil {
			return "", err
		}
		err = publishExclusiveDurable(abs, data)
		if err == nil {
			return rel, nil
		}
		if os.IsExist(err) || errors.Is(err, fsafety.ErrSymlink) {
			continue
		}
		return "", err
	}
	return "", fmt.Errorf("hookactions: evidence create: exhausted unique names")
}

func syncParentDir(path string) error {
	if err := fsafety.SyncDirDurable(filepath.Dir(path)); err != nil {
		if errors.Is(err, fsafety.ErrDirSyncUnsupported) {
			return nil
		}
		return err
	}
	return nil
}

// removePrimaryAfterEvidence deletes rel only when its bytes still match the
// evidence that was just published. The caller must hold the mutation lock.
func removePrimaryAfterEvidence(homePath, rel string, original []byte) error {
	current, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("hookactions: lifecycle document changed during quarantine")
	}
	abs, err := fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("hookactions: remove lifecycle primary after evidence: %w", err)
	}
	if err := syncParentDir(abs); err != nil {
		return fmt.Errorf("hookactions: sync dir after lifecycle remove: %w", err)
	}
	return nil
}

// replacePrimaryAfterEvidence replaces rel only when its bytes still match the
// published evidence. A failure before rename leaves the primary in place.
func replacePrimaryAfterEvidence(homePath, rel string, original, next []byte, tmpPattern string) error {
	current, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("hookactions: lifecycle document changed during quarantine")
	}
	return fsafety.AtomicWriteContainedDirDurable(homePath, rel, next, 0o600, home.DirPermHome, tmpPattern)
}
