package runtime

import (
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// RemoveConflictForTest exposes quarantine path handling for rollback-safety tests.
func RemoveConflictForTest(root, rel string) error {
	return removeConflict(root, rel, fileSnapshotByRel(nil, rel), nil)
}

// RemoveConflictTrackedForTest exercises transactional quarantine with baseline + footprint.
func RemoveConflictTrackedForTest(root string, baseline fsafety.FileSnapshot, fp *fsafety.TransactionFootprint) error {
	return removeConflict(root, baseline.Rel, baseline, fp)
}

// SetRepairAfterWriteHookForTest installs a test-only seam after Atlas-owned writes.
func SetRepairAfterWriteHookForTest(fn func(rel string) error) {
	repairAfterWriteHook = fn
}

// SetRemoveAttemptBackupForTest installs a test-only seam for attempt-backup cleanup.
func SetRemoveAttemptBackupForTest(fn func(homePath, rel string) error) {
	if fn == nil {
		removeAttemptBackup = home.RemoveContainedRel
		return
	}
	removeAttemptBackup = fn
}
