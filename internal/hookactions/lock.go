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

const maintainLockName = "maintain.lock"

// workerMaintainLockBusyObserver is a deterministic test seam. Production nil.
var workerMaintainLockBusyObserver func()

// maintainLockRel is the Home-relative lock leaf for one project.
func maintainLockRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState, maintainLockName))
}

// TryAcquireMaintainLock takes an exclusive non-blocking flock under Atlas Home.
// Callers must Release. Cross-process: session.start from Cursor and OpenCode
// cannot double-run maintenance for the same project.
func TryAcquireMaintainLock(homePath, projectID string) (*os.File, error) {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return nil, fmt.Errorf("hookactions: maintain lock requires home and project id")
	}
	stateRel := filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirState))
	if err := fsafety.SafeMkdirAll(homePath, stateRel, home.DirPermHome); err != nil {
		return nil, fmt.Errorf("hookactions: maintain lock dir: %w", err)
	}
	rel := maintainLockRel(projectID)
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	// No-follow open rejects a symlink leaf. The stamp below is written only
	// to the descriptor returned here, after the flock.
	f, err := openLifecycleLock(abs)
	if err != nil {
		return nil, fmt.Errorf("hookactions: open maintain lock: %w", err)
	}
	if err := flockExclusiveNonBlocking(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	// Best-effort liveness stamp (not authoritative; flock is).
	_, _ = f.Seek(0, 0)
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "pid=%d\nat=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
	return f, nil
}

// ReleaseMaintainLock unlocks and closes. Idempotent on nil.
func ReleaseMaintainLock(f *os.File) error {
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

// acquireMaintainLockForWorker retries while concurrent admission parents briefly
// cycle the flock (lease inspection). Bounded by AdmitLeaseTTL so a duplicate
// spawn against a long-running worker fails open quickly.
func acquireMaintainLockForWorker(homePath, projectID string) (*os.File, error) {
	deadline := time.Now().Add(AdmitLeaseTTL)
	for {
		f, err := TryAcquireMaintainLock(homePath, projectID)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, errMaintainBusy) {
			return nil, err
		}
		if workerMaintainLockBusyObserver != nil {
			workerMaintainLockBusyObserver()
		}
		if time.Now().After(deadline) {
			return nil, errMaintainBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}
