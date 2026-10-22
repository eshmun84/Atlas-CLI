package hookactions

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

// Ownership and generation operations for maintain-pending.yaml.
// All mutators that touch ownership validate token+fence when required.
// Callers must hold pending.lock (via withPendingLock) unless noted.

func newOwnerToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; still opaque and non-sensitive.
		return fmt.Sprintf("tok-%d", pendingNow().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func ownsPending(doc MaintainPendingDocument, token string, fence int64) bool {
	token = strings.TrimSpace(token)
	if token == "" || fence <= 0 {
		return false
	}
	return strings.TrimSpace(doc.OwnerToken) == token && doc.OwnerFence == fence
}

func ownerVigente(doc MaintainPendingDocument, now time.Time) bool {
	if strings.TrimSpace(doc.OwnerToken) == "" {
		return false
	}
	switch strings.TrimSpace(doc.OwnerState) {
	case ownerStateReserved, ownerStateRunningWait, ownerStateRunningMaintain:
		// ok
	default:
		return false
	}
	until, ok := parsePendingTime(doc.OwnerLeaseUntil)
	if !ok {
		return false
	}
	return now.Before(until)
}

func clearOwnershipFields(doc *MaintainPendingDocument) {
	doc.OwnerToken = ""
	doc.OwnerLeaseUntil = ""
	doc.OwnerState = ownerStateNone
	doc.SchedulerPID = 0
	// OwnerFence is monotonic and retained.
}

// bumpEdit increments pending_generation and refreshes debounce deadlines.
// Does not touch ownership.
func bumpEdit(doc *MaintainPendingDocument, now time.Time) {
	wasClean := !pendingHasWork(*doc)
	doc.PendingGeneration++
	doc.LastEditAt = formatPendingTime(now)
	if wasClean || strings.TrimSpace(doc.BurstStartedAt) == "" {
		doc.BurstStartedAt = formatPendingTime(now)
	}
	due := now.Add(DebounceDelay)
	burstStart, ok := parsePendingTime(doc.BurstStartedAt)
	if !ok {
		burstStart = now
		doc.BurstStartedAt = formatPendingTime(now)
	}
	maxDue := burstStart.Add(MaxCoalesceWindow)
	if due.After(maxDue) {
		due = maxDue
	}
	doc.DueAt = formatPendingTime(due)
}

// reclaimExpired clears ownership when the lease is not vigente.
func reclaimExpired(doc *MaintainPendingDocument, now time.Time) bool {
	if strings.TrimSpace(doc.OwnerToken) == "" {
		return false
	}
	if ownerVigente(*doc, now) {
		return false
	}
	clearOwnershipFields(doc)
	return true
}

// reserveOwner claims scheduler ownership. Returns token, fence, ok.
func reserveOwner(doc *MaintainPendingDocument, now time.Time) (token string, fence int64, ok bool) {
	reclaimExpired(doc, now)
	if !pendingHasWork(*doc) {
		return "", 0, false
	}
	if ownerVigente(*doc, now) {
		return "", 0, false
	}
	doc.OwnerFence++
	token = newOwnerToken()
	doc.OwnerToken = token
	fence = doc.OwnerFence
	doc.OwnerLeaseUntil = formatPendingTime(now.Add(OwnerLeaseTTL))
	doc.OwnerState = ownerStateReserved
	doc.SchedulerPID = os.Getpid()
	return token, fence, true
}

// clearOwnReservation clears ownership only when token+fence match.
func clearOwnReservation(doc *MaintainPendingDocument, token string, fence int64) bool {
	if !ownsPending(*doc, token, fence) {
		return false
	}
	clearOwnershipFields(doc)
	return true
}

// heartbeat extends the owner lease when token+fence match.
func heartbeat(doc *MaintainPendingDocument, token string, fence int64, now time.Time, state string) bool {
	if !ownsPending(*doc, token, fence) {
		return false
	}
	doc.OwnerLeaseUntil = formatPendingTime(now.Add(OwnerLeaseTTL))
	if state != "" {
		doc.OwnerState = state
	}
	doc.SchedulerPID = os.Getpid()
	return true
}

// beginMaintain captures gen0 and marks running_maintain. Returns gen0, ok.
func beginMaintain(doc *MaintainPendingDocument, token string, fence int64, now time.Time) (gen0 int64, ok bool) {
	if !ownsPending(*doc, token, fence) {
		return 0, false
	}
	if !pendingHasWork(*doc) {
		return 0, false
	}
	gen0 = doc.PendingGeneration
	doc.OwnerState = ownerStateRunningMaintain
	doc.OwnerLeaseUntil = formatPendingTime(now.Add(OwnerLeaseTTL))
	doc.SchedulerPID = os.Getpid()
	return gen0, true
}

// commitGeneration applies the gen0 accreditation rule.
// On operational success: processed := gen0 (never > gen0); requeue if pending > gen0.
// On failure: processed unchanged; schedule retry backoff.
// Returns stillOwned, hasMoreWork.
func commitGeneration(doc *MaintainPendingDocument, token string, fence int64, gen0 int64, success bool, errCode string, now time.Time) (stillOwned bool, hasMore bool) {
	if !ownsPending(*doc, token, fence) {
		return false, pendingHasWork(*doc)
	}
	if !success {
		doc.Attempt++
		backoff := retryBackoff(doc.Attempt)
		retryAt := now.Add(backoff)
		doc.NextRetryAt = formatPendingTime(retryAt)
		doc.DueAt = formatPendingTime(retryAt)
		if errCode == "" {
			errCode = MaintainErrNone
		}
		doc.LastErrorCode = errCode
		doc.OwnerState = ownerStateRunningWait
		doc.OwnerLeaseUntil = formatPendingTime(now.Add(OwnerLeaseTTL))
		return true, true
	}
	// Success: advance at most to gen0.
	if gen0 < doc.ProcessedGeneration {
		gen0 = doc.ProcessedGeneration
	}
	if gen0 > doc.PendingGeneration {
		gen0 = doc.PendingGeneration
	}
	doc.ProcessedGeneration = gen0
	doc.Attempt = 0
	doc.NextRetryAt = ""
	doc.LastErrorCode = MaintainErrNone
	if doc.PendingGeneration > gen0 {
		doc.BurstStartedAt = formatPendingTime(now)
		due := now.Add(DebounceDelay)
		maxDue := now.Add(MaxCoalesceWindow)
		if due.After(maxDue) {
			due = maxDue
		}
		doc.DueAt = formatPendingTime(due)
		doc.OwnerState = ownerStateRunningWait
		doc.OwnerLeaseUntil = formatPendingTime(now.Add(OwnerLeaseTTL))
		return true, true
	}
	doc.BurstStartedAt = ""
	doc.DueAt = ""
	clearOwnershipFields(doc)
	return false, false
}

// releaseOwner clears ownership when token+fence match (idle path).
func releaseOwner(doc *MaintainPendingDocument, token string, fence int64) bool {
	return clearOwnReservation(doc, token, fence)
}

// operationalSuccess reports whether CE/CI outcomes allow generation advance.
func operationalSuccess(ceOut, ciOut string) bool {
	ceOK := ceOut == MaintainOutcomeOK || ceOut == MaintainOutcomeSkipped
	ciOK := ciOut == MaintainOutcomeOK || ciOut == MaintainOutcomeSkipped || ciOut == MaintainOutcomeUnavailable
	return ceOK && ciOK
}

// --- Public locked wrappers ---

// BumpPendingEdit records a possible mutation signal and returns the new pending generation.
func BumpPendingEdit(homePath, projectID string) (pendingGen int64, err error) {
	err = withPendingLock(homePath, projectID, func() error {
		doc, err := loadPending(homePath, projectID)
		if err != nil {
			return err
		}
		bumpEdit(&doc, pendingNow())
		pendingGen = doc.PendingGeneration
		return savePending(homePath, projectID, doc)
	})
	return pendingGen, err
}

// ReserveOwnerResult is the outcome of a reservation attempt.
type ReserveOwnerResult struct {
	Token    string
	Fence    int64
	Reserved bool
	HasWork  bool
}

// ReserveOwner claims scheduler ownership when work is dirty and no owner is vigente.
func ReserveOwner(homePath, projectID string) (ReserveOwnerResult, error) {
	var out ReserveOwnerResult
	err := withPendingLock(homePath, projectID, func() error {
		doc, err := loadPending(homePath, projectID)
		if err != nil {
			return err
		}
		out.HasWork = pendingHasWork(doc)
		tok, fence, ok := reserveOwner(&doc, pendingNow())
		if ok {
			out.Token = tok
			out.Fence = fence
			out.Reserved = true
			return savePending(homePath, projectID, doc)
		}
		return savePending(homePath, projectID, doc) // persist reclaim if any
	})
	return out, err
}

// ClearOwnReservation drops ownership only for the matching token+fence.
func ClearOwnReservation(homePath, projectID, token string, fence int64) (cleared bool, err error) {
	err = withPendingLock(homePath, projectID, func() error {
		doc, err := loadPending(homePath, projectID)
		if err != nil {
			return err
		}
		cleared = clearOwnReservation(&doc, token, fence)
		if cleared {
			return savePending(homePath, projectID, doc)
		}
		return nil
	})
	return cleared, err
}

// HeartbeatOwner renews the owner lease when token+fence match.
func HeartbeatOwner(homePath, projectID, token string, fence int64, state string) (ok bool, err error) {
	err = withPendingLock(homePath, projectID, func() error {
		doc, err := loadPending(homePath, projectID)
		if err != nil {
			return err
		}
		ok = heartbeat(&doc, token, fence, pendingNow(), state)
		if ok {
			return savePending(homePath, projectID, doc)
		}
		return nil
	})
	return ok, err
}

// BeginMaintainCapture captures gen0 for a maintain cycle.
func BeginMaintainCapture(homePath, projectID, token string, fence int64) (gen0 int64, ok bool, err error) {
	err = withPendingLock(homePath, projectID, func() error {
		doc, err := loadPending(homePath, projectID)
		if err != nil {
			return err
		}
		gen0, ok = beginMaintain(&doc, token, fence, pendingNow())
		if ok {
			return savePending(homePath, projectID, doc)
		}
		return nil
	})
	return gen0, ok, err
}

// CommitGenerationResult summarizes a commit attempt.
type CommitGenerationResult struct {
	StillOwned bool
	HasMore    bool
	Applied    bool // false when fenced out
}

// CommitGeneration applies the gen0 accreditation rule under ownership check.
func CommitGeneration(homePath, projectID, token string, fence int64, gen0 int64, success bool, errCode string) (CommitGenerationResult, error) {
	var out CommitGenerationResult
	err := withPendingLock(homePath, projectID, func() error {
		doc, err := loadPending(homePath, projectID)
		if err != nil {
			return err
		}
		if !ownsPending(doc, token, fence) {
			out.Applied = false
			out.HasMore = pendingHasWork(doc)
			return nil
		}
		still, more := commitGeneration(&doc, token, fence, gen0, success, errCode, pendingNow())
		out.StillOwned = still
		out.HasMore = more
		out.Applied = true
		return savePending(homePath, projectID, doc)
	})
	return out, err
}

// ReleaseOwner clears ownership for the matching token+fence.
func ReleaseOwner(homePath, projectID, token string, fence int64) (released bool, err error) {
	return ClearOwnReservation(homePath, projectID, token, fence)
}

// ReclaimExpiredOwner clears expired ownership (no token required).
func ReclaimExpiredOwner(homePath, projectID string) (reclaimed bool, err error) {
	err = withPendingLock(homePath, projectID, func() error {
		doc, err := loadPending(homePath, projectID)
		if err != nil {
			return err
		}
		reclaimed = reclaimExpired(&doc, pendingNow())
		if reclaimed {
			return savePending(homePath, projectID, doc)
		}
		return nil
	})
	return reclaimed, err
}
