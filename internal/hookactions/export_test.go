package hookactions

import (
	"context"
	"errors"
	"time"
)

// Test helpers for Phase B.8 recovery paths and Phase C.2b seams.

func BeginMaintainLastForTest(homePath, projectID, sessionID string) (MaintainLastRecord, time.Time) {
	return beginMaintainLast(homePath, projectID, sessionID)
}

func FinishMaintainLastForTest(homePath, projectID, sessionID string, started time.Time, ce, ci, errCode string) {
	finishMaintainLast(homePath, projectID, sessionID, started, ce, ci, errCode)
}

func ClearMaintainLeaseForTest(homePath, projectID string) {
	clearMaintainLease(homePath, projectID)
}

// SetPendingClockForTest overrides pendingNow. Pass nil to restore.
func SetPendingClockForTest(fn func() time.Time) {
	if fn == nil {
		pendingNow = func() time.Time { return time.Now().UTC() }
		return
	}
	pendingNow = fn
}

// SetPendingSleepForTest overrides pendingSleep. Pass nil to restore.
func SetPendingSleepForTest(fn func(time.Duration)) {
	if fn == nil {
		pendingSleep = time.Sleep
		return
	}
	pendingSleep = fn
}

// SetAbsoluteProcessDeadlineForTest overrides scheduler wall-clock cap. 0 restores.
func SetAbsoluteProcessDeadlineForTest(d time.Duration) {
	if d <= 0 {
		absoluteProcessDeadline = AbsoluteProcessDeadline
		return
	}
	absoluteProcessDeadline = d
}

// SetOwnerHeartbeatIntervalForTest overrides in-maintain heartbeat period.
// Pass 0 to restore production default. Pass a negative duration to disable.
func SetOwnerHeartbeatIntervalForTest(d time.Duration) {
	if d == 0 {
		ownerHeartbeatInterval = OwnerHeartbeatInterval
		return
	}
	ownerHeartbeatInterval = d
}

// RunPostCodeMaintainCycleForTest runs one locked maintain cycle (tests).
func RunPostCodeMaintainCycleForTest(ctx context.Context, ref ProjectRef, token string, fence int64) error {
	return runPostCodeMaintainCycle(ctx, ref, token, fence)
}

// SetPostCodeMaintainForTest injects the CE/CI body used by the post-code scheduler.
// Pass nil to restore production executeMaintainServices.
func SetPostCodeMaintainForTest(fn func(ctx context.Context, ref ProjectRef) (ceOut, ciOut, errCode string)) {
	if fn == nil {
		postCodeMaintainFn = executeMaintainServices
		return
	}
	postCodeMaintainFn = fn
}

// OwnerVigenteForTest reports whether ownership is live at now.
func OwnerVigenteForTest(doc MaintainPendingDocument, now time.Time) bool {
	return ownerVigente(doc, now)
}

// OwnsPendingForTest reports token+fence match.
func OwnsPendingForTest(doc MaintainPendingDocument, token string, fence int64) bool {
	return ownsPending(doc, token, fence)
}

// IsMaintainBusyForTest reports maintain.lock contention.
func IsMaintainBusyForTest(err error) bool {
	return errors.Is(err, errMaintainBusy)
}

// SetCorruptEvidenceWriterForTest overrides quarantine evidence writer.
// Pass nil to restore. Injected failures must leave the primary file untouched.
func SetCorruptEvidenceWriterForTest(fn func(homePath, projectID string, data []byte) (rel string, err error)) {
	if fn == nil {
		writeCorruptEvidenceExclusive = writeCorruptEvidenceExclusiveProd
		return
	}
	writeCorruptEvidenceExclusive = fn
}

// SetRecoveryPendingWriterForTest overrides the primary reset write after quarantine.
// Pass nil to restore.
func SetRecoveryPendingWriterForTest(fn func(homePath, rel string, data []byte) error) {
	if fn == nil {
		writeRecoveryPendingPrimary = writeRecoveryPendingPrimaryProd
		return
	}
	writeRecoveryPendingPrimary = fn
}
