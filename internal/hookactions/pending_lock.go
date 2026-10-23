package hookactions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// pendingLockBusyObserver is a deterministic test seam. Production nil.
var pendingLockBusyObserver func()

// TryAcquirePendingLock takes an exclusive non-blocking flock for pending RMW.
// Callers must ReleasePendingLock. Never held across CE/CI work.
func TryAcquirePendingLock(homePath, projectID string) (*os.File, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return nil, fmt.Errorf("hookactions: pending lock requires home and project id")
	}
	stateRel := filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState))
	if err := fsafety.SafeMkdirAll(homePath, stateRel, home.DirPermHome); err != nil {
		return nil, fmt.Errorf("hookactions: pending lock dir: %w", err)
	}
	rel := pendingLockRel(projectID)
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	// Same no-follow leaf open as maintain.lock. Callers acquire maintain.lock
	// first; this function never takes that lock.
	f, err := openLifecycleLock(abs)
	if err != nil {
		return nil, fmt.Errorf("hookactions: open pending lock: %w", err)
	}
	if err := flockExclusiveNonBlocking(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	_, _ = f.Seek(0, 0)
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "pid=%d\nat=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
	return f, nil
}

// ReleasePendingLock unlocks and closes. Idempotent on nil.
func ReleasePendingLock(f *os.File) error {
	if f == nil {
		return nil
	}
	errUnlock := flockUnlock(f)
	errClose := f.Close()
	if errUnlock != nil {
		return errUnlock
	}
	return errClose
}

// withPendingLock runs fn while holding pending.lock, retrying briefly on busy.
// Lock hierarchy: maintain.lock may be acquired before pending.lock, never the
// reverse. Therefore fn must not attempt to acquire maintain.lock.
func withPendingLock(homePath, projectID string, fn func() error) error {
	deadline := pendingNow().Add(2 * time.Second)
	for {
		f, err := TryAcquirePendingLock(homePath, projectID)
		if err == nil {
			defer func() { _ = ReleasePendingLock(f) }()
			return fn()
		}
		if !errors.Is(err, errMaintainBusy) {
			return err
		}
		if pendingLockBusyObserver != nil {
			pendingLockBusyObserver()
		}
		if pendingNow().After(deadline) {
			return errMaintainBusy
		}
		pendingSleep(5 * time.Millisecond)
	}
}
