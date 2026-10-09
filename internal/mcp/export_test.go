package mcp

import "github.com/eshmun84/Atlas-CLI/internal/home"

// SetRemoveAttemptBackupForTest installs a test-only seam for attempt-backup cleanup.
func SetRemoveAttemptBackupForTest(fn func(homePath, rel string) error) {
	if fn == nil {
		removeAttemptBackup = home.RemoveContainedRel
		return
	}
	removeAttemptBackup = fn
}

// RestoreOwnershipSnapshotForTest exposes ownership absence/presence restore.
func RestoreOwnershipSnapshotForTest(homePath, projectID string, snap TransactionSnapshot) error {
	return restoreOwnershipSnapshot(homePath, projectID, snap)
}
