package hookactions

import "time"

// Test helpers for Phase B.8 recovery paths.

func BeginMaintainLastForTest(homePath, projectID, sessionID string) (MaintainLastRecord, time.Time) {
	return beginMaintainLast(homePath, projectID, sessionID)
}

func FinishMaintainLastForTest(homePath, projectID, sessionID string, started time.Time, ce, ci, errCode string) {
	finishMaintainLast(homePath, projectID, sessionID, started, ce, ci, errCode)
}

func ClearMaintainLeaseForTest(homePath, projectID string) {
	clearMaintainLease(homePath, projectID)
}
