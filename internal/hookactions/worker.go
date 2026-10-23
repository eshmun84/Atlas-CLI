package hookactions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// RunWorker executes Context Economy then Code Intelligence maintenance under
// the cross-process maintain lock. Fail-open: CE/CI errors are recorded in
// maintain-last.yaml but do not fail the worker (busy/not-initialized → success).
func RunWorker(ctx context.Context, ref ProjectRef) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, WorkerTimeout)
		defer cancel()
	}

	lock, err := acquireMaintainLockForWorker(ref.Home, ref.ProjectID)
	if err != nil {
		errorCode := admissionErrLockFailed
		if errors.Is(err, errMaintainBusy) {
			errorCode = admissionErrLockBusy
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				errorCode = admissionErrTimeout
			} else if errors.Is(ctx.Err(), context.Canceled) {
				errorCode = admissionErrCancelled
			}
			closeAdmissionRetryable(ref.Home, ref.ProjectID, ref.SessionID, ref.AdmissionID, errorCode)
			return nil
		}
		closeAdmissionRetryable(ref.Home, ref.ProjectID, ref.SessionID, ref.AdmissionID, errorCode)
		return err
	}

	claimed, err := claimAdmissionForWorker(ref)
	if err != nil {
		_ = ReleaseMaintainLock(lock)
		closeAdmissionRetryable(ref.Home, ref.ProjectID, ref.SessionID, ref.AdmissionID, admissionErrLockFailed)
		return err
	}
	if !claimed {
		_ = ReleaseMaintainLock(lock)
		closeAdmissionRetryable(ref.Home, ref.ProjectID, ref.SessionID, ref.AdmissionID, admissionErrOwnershipLost)
		return nil
	}
	defer func() {
		clearMaintainLeaseOwned(ref.Home, ref.ProjectID, ref.AdmissionID)
		_ = ReleaseMaintainLock(lock)
	}()

	sessionID := strings.TrimSpace(ref.SessionID)

	live, err := ResolveAt(ref.Root, ref.Home)
	if err != nil {
		_, started := beginMaintainLastEx(ref.Home, ref.ProjectID, maintainLastOpts{
			SessionID:   sessionID,
			AdmissionID: ref.AdmissionID,
		})
		finishMaintainLastEx(ref.Home, ref.ProjectID, maintainLastOpts{
			SessionID:   sessionID,
			AdmissionID: ref.AdmissionID,
		}, started, MaintainOutcomeSkipped, MaintainOutcomeSkipped, MaintainErrResolveFailed)
		return nil
	}
	live.SessionID = sessionID
	live.AdmissionID = ref.AdmissionID

	_, started := beginMaintainLastEx(live.Home, live.ProjectID, maintainLastOpts{
		SessionID:   sessionID,
		AdmissionID: ref.AdmissionID,
	})

	ce := &ContextEconomyAction{}
	ceRes, ceErr := ce.Maintain(ctx, live)
	ceOut := mapCEOutcome(ceRes, ceErr)

	ciOut := MaintainOutcomePending
	var ciErr error
	if ctx.Err() == nil {
		ci := &CodeIntelAction{}
		var ciRes CodeIntelResult
		ciRes, ciErr = ci.Maintain(ctx, live)
		ciOut = mapCIOutcome(ciRes, ciErr)
	} else {
		ciOut = mapCIOutcome(CodeIntelResult{}, ctx.Err())
	}

	errCode := deriveErrorCode(ceOut, ciOut, ctx.Err())
	finishMaintainLastEx(live.Home, live.ProjectID, maintainLastOpts{
		SessionID:   sessionID,
		AdmissionID: ref.AdmissionID,
	}, started, ceOut, ciOut, errCode)
	return ctx.Err()
}

// claimAdmissionForWorker transitions only the matching admitted lease to
// running. It runs while maintain.lock is held and acquires pending.lock second.
func claimAdmissionForWorker(ref ProjectRef) (bool, error) {
	claimed := false
	err := withPendingLock(ref.Home, ref.ProjectID, func() error {
		lease, ok, err := readMaintainLease(ref.Home, ref.ProjectID)
		if err != nil {
			return err
		}
		admissionID := ref.AdmissionID
		if admissionID != "" {
			if !validAdmissionID(admissionID) {
				return fmt.Errorf("hookactions: invalid admission id")
			}
			if !ok || strings.TrimSpace(lease.State) != leaseStateAdmitted || !leaseOwnedBy(lease, admissionID) {
				return nil
			}
			if ValidSessionID(ref.SessionID) {
				sessions, err := readSessionMaintain(ref.Home, ref.ProjectID)
				if err != nil {
					return err
				}
				entry, ok := sessions.Sessions[strings.TrimSpace(ref.SessionID)]
				if !ok || strings.TrimSpace(entry.State) != sessionStateAdmitted || !sessionOwnedBy(entry, admissionID) {
					return nil
				}
			}
		} else if ok && lease.AdmissionID != "" {
			// Legacy workers can operate only on legacy state.
			return nil
		}
		if err := writeMaintainLease(ref.Home, ref.ProjectID, leaseStateRunning, admissionID); err != nil {
			return err
		}
		claimed = true
		return nil
	})
	return claimed, err
}

// MaybeRunInternalMaintain is the process entry for detached workers.
// Returns handled=true when this process is an internal maintain worker.
// Modes: session (default / Phase B) and post_code_scheduler (Phase C.2b).
func MaybeRunInternalMaintain(stdout, stderr io.Writer) (handled bool, err error) {
	if strings.TrimSpace(os.Getenv(EnvInternalMaintain)) != "1" {
		return false, nil
	}
	_ = stdout
	root := strings.TrimSpace(os.Getenv(envMaintainRoot))
	homePath := strings.TrimSpace(os.Getenv(envMaintainHome))
	if root == "" {
		fmt.Fprintln(stderr, "hookactions: internal maintain missing root")
		return true, fmt.Errorf("hookactions: internal maintain missing root")
	}
	if handled, err := maybeRunPostCodeScheduler(stderr, root, homePath); handled {
		return true, err
	}
	sessionID := strings.TrimSpace(os.Getenv(envMaintainSession))
	projectID := strings.TrimSpace(os.Getenv(envMaintainProjectID))
	admissionID := os.Getenv(envMaintainAdmissionID)
	if admissionID != "" && (!validAdmissionID(admissionID) || !validInternalProjectID(projectID)) {
		fmt.Fprintln(stderr, "hookactions: internal maintain invalid admission identity")
		return true, fmt.Errorf("hookactions: internal maintain invalid admission identity")
	}
	if homePath != "" {
		_ = os.Setenv("ATLAS_HOME", homePath)
	}
	ref, err := ResolveAt(root, homePath)
	if err != nil {
		if admissionID != "" {
			closeAdmissionRetryable(homePath, projectID, sessionID, admissionID, admissionErrResolveFailed)
		}
		return true, nil
	}
	if admissionID != "" && ref.ProjectID != projectID {
		closeAdmissionRetryable(homePath, projectID, sessionID, admissionID, admissionErrResolveFailed)
		return true, nil
	}
	ref.SessionID = sessionID
	ref.AdmissionID = admissionID
	ctx, cancel := context.WithTimeout(context.Background(), WorkerTimeout)
	defer cancel()
	_ = RunWorker(ctx, ref)
	return true, nil
}

// InlineSpawner runs maintenance in-process and waits (tests / workers).
// Do not use as the Action admission Spawner under AdmissionTimeout — prefer
// AsyncInlineSpawner so admission stays within adapter subprocess budgets.
func InlineSpawner(ref ProjectRef) error {
	ctx, cancel := context.WithTimeout(context.Background(), WorkerTimeout)
	defer cancel()
	return RunWorker(ctx, ref)
}

var asyncInlineWG sync.WaitGroup

// AsyncInlineSpawner schedules in-process maintenance and returns immediately
// (mirrors detached spawn for tests without exec).
func AsyncInlineSpawner(ref ProjectRef) error {
	asyncInlineWG.Add(1)
	go func() {
		defer asyncInlineWG.Done()
		_ = InlineSpawner(ref)
	}()
	return nil
}

// WaitAsyncInlineForTest blocks until AsyncInlineSpawner work finishes.
func WaitAsyncInlineForTest() {
	asyncInlineWG.Wait()
}
