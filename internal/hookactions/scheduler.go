package hookactions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Phase C.2b/C.2c post-code scheduler: finite-lifetime detached process.
// Production trigger: PostCodeAction on tool.after_execution (C.2c).

const (
	envMaintainMode  = "ATLAS_INTERNAL_MAINTAIN_MODE"
	envMaintainToken = "ATLAS_INTERNAL_MAINTAIN_TOKEN"
	envMaintainFence = "ATLAS_INTERNAL_MAINTAIN_FENCE"

	maintainModeSession     = "session"
	maintainModePostCode    = "post_code_scheduler"
	maintainTriggerPostCode = "post_code"
)

// SchedulerSpawner starts a detached post-code scheduler for a reservation.
type SchedulerSpawner func(ref ProjectRef, token string, fence int64) error

var (
	schedulerSpawnerMu sync.Mutex
	schedulerSpawner   SchedulerSpawner = spawnDetachedScheduler

	// postCodeMaintainFn runs CE then CI under an already-held maintain lock.
	// Tests inject slow/failing bodies; production uses executeMaintainServices.
	postCodeMaintainFn = executeMaintainServices

	// absoluteProcessDeadline is overridable in tests.
	absoluteProcessDeadline = AbsoluteProcessDeadline

	// ownerHeartbeatInterval is overridable in tests for deterministic cancel paths.
	ownerHeartbeatInterval = OwnerHeartbeatInterval
)

// SetSchedulerSpawner overrides the post-code scheduler spawner (tests).
func SetSchedulerSpawner(fn SchedulerSpawner) {
	schedulerSpawnerMu.Lock()
	defer schedulerSpawnerMu.Unlock()
	if fn == nil {
		schedulerSpawner = spawnDetachedScheduler
		return
	}
	schedulerSpawner = fn
}

func currentSchedulerSpawner() SchedulerSpawner {
	schedulerSpawnerMu.Lock()
	defer schedulerSpawnerMu.Unlock()
	return schedulerSpawner
}

// BumpPendingAndEnsureScheduler records a mutation signal and ensures a scheduler
// owns the work. Called by PostCodeAction (C.2c) after an allowlisted
// tool.after_execution; also used by tests.
func BumpPendingAndEnsureScheduler(ref ProjectRef) error {
	if !TriggersAreEnabled() {
		return nil
	}
	recoveryAttempted := false
	if _, err := BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		if !pendingCorruptionRecoverable(err) {
			return err
		}
		recoveryAttempted = true
		if err := QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
			return err
		}
	}
	return ensurePostCodeSchedulerIfDirty(ref, !recoveryAttempted)
}

// EnsurePostCodeSchedulerIfDirty spawns a scheduler when pending work exists and
// no owner is vigente. Safe to call from session.start recovery; does not alter
// Phase B admission outcomes.
func EnsurePostCodeSchedulerIfDirty(ref ProjectRef) error {
	return ensurePostCodeSchedulerIfDirty(ref, true)
}

func ensurePostCodeSchedulerIfDirty(ref ProjectRef, allowRecovery bool) error {
	if !TriggersAreEnabled() {
		return nil
	}
	ref.Home = strings.TrimSpace(ref.Home)
	ref.ProjectID = strings.TrimSpace(ref.ProjectID)
	if ref.Home == "" || ref.ProjectID == "" {
		return nil
	}
	res, err := ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil {
		if !allowRecovery || !pendingCorruptionRecoverable(err) {
			return err
		}
		if err := QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
			return err
		}
		res, err = ReserveOwner(ref.Home, ref.ProjectID)
		if err != nil {
			return err
		}
	}
	if !res.Reserved {
		return nil
	}
	if err := currentSchedulerSpawner()(ref, res.Token, res.Fence); err != nil {
		_, _ = ClearOwnReservation(ref.Home, ref.ProjectID, res.Token, res.Fence)
		return fmt.Errorf("%w: %v", errAdmitSpawn, err)
	}
	return nil
}

// RunPostCodeScheduler is the detached scheduler main loop (finite lifetime).
func RunPostCodeScheduler(ctx context.Context, ref ProjectRef, token string, fence int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	token = strings.TrimSpace(token)
	if token == "" || fence <= 0 {
		return fmt.Errorf("hookactions: post-code scheduler requires token and fence")
	}
	// Voluntary exit (deadline, cancel, clean idle, fence loss): release only our
	// reservation. Crash paths skip this defer — lease expiry remains the reclaim.
	defer func() {
		_, _ = ReleaseOwner(ref.Home, ref.ProjectID, token, fence)
	}()

	startedAt := pendingNow()
	deadline := startedAt.Add(absoluteProcessDeadline)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if pendingNow().After(deadline) {
			// Absolute process deadline: release own ownership (defer); keep gens.
			return nil
		}

		var (
			hasWork bool
			dueAt   time.Time
			still   bool
		)
		err := withPendingLock(ref.Home, ref.ProjectID, func() error {
			doc, err := loadPending(ref.Home, ref.ProjectID)
			if err != nil {
				return err
			}
			if !ownsPending(doc, token, fence) {
				still = false
				hasWork = pendingHasWork(doc)
				return nil
			}
			if !pendingHasWork(doc) {
				_ = releaseOwner(&doc, token, fence)
				still = false
				hasWork = false
				return savePending(ref.Home, ref.ProjectID, doc)
			}
			if !heartbeat(&doc, token, fence, pendingNow(), ownerStateRunningWait) {
				still = false
				return nil
			}
			still = true
			hasWork = true
			dueAt, _ = parsePendingTime(doc.DueAt)
			return savePending(ref.Home, ref.ProjectID, doc)
		})
		if err != nil {
			return err
		}
		if !still {
			return nil
		}
		if !hasWork {
			return nil
		}

		now := pendingNow()
		if !dueAt.IsZero() && now.Before(dueAt) {
			wait := dueAt.Sub(now)
			if wait > MaxSleepSlice {
				wait = MaxSleepSlice
			}
			// Cap sleep so absolute deadline is respected.
			if remain := deadline.Sub(pendingNow()); remain > 0 && wait > remain {
				wait = remain
			}
			if wait > 0 {
				pendingSleep(wait)
			}
			continue
		}

		// Debounce satisfied — run one maintain cycle.
		if err := runPostCodeMaintainCycle(ctx, ref, token, fence); err != nil {
			if errors.Is(err, errMaintainBusy) {
				// Another maintain worker holds the lock; wait briefly and retry.
				pendingSleep(MaxSleepSlice)
				continue
			}
			return err
		}
	}
}

func runPostCodeMaintainCycle(ctx context.Context, ref ProjectRef, token string, fence int64) error {
	lock, err := acquireMaintainLockForWorker(ref.Home, ref.ProjectID)
	if err != nil {
		return err
	}
	ready, err := prepareSchedulerLease(ref.Home, ref.ProjectID)
	if err != nil {
		_ = ReleaseMaintainLock(lock)
		return err
	}
	if !ready {
		_ = ReleaseMaintainLock(lock)
		return errMaintainBusy
	}
	defer func() {
		clearMaintainLeaseOwned(ref.Home, ref.ProjectID, "")
		_ = ReleaseMaintainLock(lock)
	}()

	gen0, ok, err := BeginMaintainCapture(ref.Home, ref.ProjectID, token, fence)
	if err != nil {
		return err
	}
	if !ok {
		return nil // fenced out or no work
	}

	maintainCtx, cancel := context.WithTimeout(ctx, WorkerTimeout)
	defer cancel()

	// Heartbeat during CE/CI without holding pending.lock across the work.
	// interval < 0 disables the goroutine (tests); 0 means production default.
	hbDone := make(chan struct{})
	interval := ownerHeartbeatInterval
	if interval < 0 {
		close(hbDone)
	} else {
		go func() {
			defer close(hbDone)
			if interval == 0 {
				interval = OwnerHeartbeatInterval
			}
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-maintainCtx.Done():
					return
				case <-t.C:
					alive, herr := HeartbeatOwner(ref.Home, ref.ProjectID, token, fence, ownerStateRunningMaintain)
					if herr != nil || !alive {
						cancel()
						return
					}
				}
			}
		}()
	}

	_, started := beginMaintainLastEx(ref.Home, ref.ProjectID, maintainLastOpts{
		Trigger:    maintainTriggerPostCode,
		Generation: gen0,
	})
	ceOut, ciOut, errCode := postCodeMaintainFn(maintainCtx, ref)
	// Ownership may have been lost while CE/CI ignored cancellation. Never
	// credit success unless we still own the fence at commit time — CommitGeneration
	// enforces token+fence; treat ctx cancel / fence loss as non-success for outcomes.
	stillOwns := false
	if doc, lerr := loadPending(ref.Home, ref.ProjectID); lerr == nil {
		stillOwns = ownsPending(doc, token, fence)
	}
	success := stillOwns &&
		operationalSuccess(ceOut, ciOut) &&
		(errCode == "" || errCode == MaintainErrNone) &&
		maintainCtx.Err() == nil
	if !success && errCode == MaintainErrNone {
		if !stillOwns {
			errCode = MaintainErrCancelled
		} else {
			errCode = deriveErrorCode(ceOut, ciOut, maintainCtx.Err())
		}
	}
	finishMaintainLastEx(ref.Home, ref.ProjectID, maintainLastOpts{
		Trigger:    maintainTriggerPostCode,
		Generation: gen0,
	}, started, ceOut, ciOut, errCode)

	cancel() // stop heartbeat
	<-hbDone

	// CommitGeneration no-ops when fenced out; never advances processed for stale S1.
	_, _ = CommitGeneration(ref.Home, ref.ProjectID, token, fence, gen0, success, errCode)
	return nil
}

// prepareSchedulerLease runs with maintain.lock held and pending.lock acquired
// second. A fresh Phase B admission always wins; stale admission state is
// reconciled as retryable before Phase C installs its legacy running marker.
func prepareSchedulerLease(homePath, projectID string) (bool, error) {
	ready := false
	err := withPendingLock(homePath, projectID, func() error {
		if err := recoverMalformedLeaseLocked(homePath, projectID); err != nil {
			return err
		}
		if err := recoverMalformedSessionLocked(homePath, projectID); err != nil {
			return err
		}
		lease, ok, err := readMaintainLease(homePath, projectID)
		if err != nil {
			return err
		}
		if ok {
			leaseID := lease.AdmissionID
			if leaseID != "" && !validAdmissionID(leaseID) {
				if !invalidLeaseRecoverable(lease, time.Now().UTC()) {
					return errInvalidPersistedAdmissionID
				}
				if err := quarantineInvalidMaintainLeaseLocked(homePath, projectID); err != nil {
					return fmt.Errorf("%w: %v", errInvalidPersistedAdmissionID, err)
				}
				ok = false
			}
		}
		if ok && strings.TrimSpace(lease.State) == leaseStateAdmitted && leaseBlocksAdmission(lease, time.Now().UTC()) {
			return nil
		}
		if ok {
			leaseID := lease.AdmissionID
			if leaseID != "" {
				if err := markAdmissionSessionsRetryableLocked(homePath, projectID, leaseID, admissionErrWorkerInterrupted); err != nil {
					return err
				}
			}
			clearMaintainLeaseOwnedLocked(homePath, projectID, leaseID)
		}
		if err := writeMaintainLease(homePath, projectID, leaseStateRunning, ""); err != nil {
			return err
		}
		ready = true
		return nil
	})
	return ready, err
}

// executeMaintainServices runs CE then CI without acquiring maintain.lock.
// Caller must already hold maintain.lock.
func executeMaintainServices(ctx context.Context, ref ProjectRef) (ceOut, ciOut, errCode string) {
	live, err := ResolveAt(ref.Root, ref.Home)
	if err != nil {
		return MaintainOutcomeSkipped, MaintainOutcomeSkipped, MaintainErrResolveFailed
	}
	ce := &ContextEconomyAction{}
	ceRes, ceErr := ce.Maintain(ctx, live)
	ceOut = mapCEOutcome(ceRes, ceErr)

	ciOut = MaintainOutcomePending
	if ctx.Err() == nil {
		ci := &CodeIntelAction{}
		ciRes, ciErr := ci.Maintain(ctx, live)
		ciOut = mapCIOutcome(ciRes, ciErr)
	} else {
		ciOut = mapCIOutcome(CodeIntelResult{}, ctx.Err())
	}
	errCode = deriveErrorCode(ceOut, ciOut, ctx.Err())
	return ceOut, ciOut, errCode
}

var asyncSchedulerWG sync.WaitGroup

// AsyncInlineSchedulerSpawner runs the scheduler in-process (tests).
func AsyncInlineSchedulerSpawner(ref ProjectRef, token string, fence int64) error {
	asyncSchedulerWG.Add(1)
	go func() {
		defer asyncSchedulerWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), absoluteProcessDeadline)
		defer cancel()
		_ = RunPostCodeScheduler(ctx, ref, token, fence)
	}()
	return nil
}

// WaitAsyncSchedulerForTest blocks until AsyncInlineSchedulerSpawner work finishes.
func WaitAsyncSchedulerForTest() {
	asyncSchedulerWG.Wait()
}

// InlineSchedulerSpawner runs the scheduler synchronously (tests).
func InlineSchedulerSpawner(ref ProjectRef, token string, fence int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), absoluteProcessDeadline)
	defer cancel()
	return RunPostCodeScheduler(ctx, ref, token, fence)
}

func parseFenceEnv(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty fence")
	}
	return strconv.ParseInt(raw, 10, 64)
}

// maybeRunPostCodeScheduler handles ATLAS_INTERNAL_MAINTAIN_MODE=post_code_scheduler.
func maybeRunPostCodeScheduler(stderr io.Writer, root, homePath string) (handled bool, err error) {
	mode := strings.TrimSpace(os.Getenv(envMaintainMode))
	if mode != maintainModePostCode {
		return false, nil
	}
	token := strings.TrimSpace(os.Getenv(envMaintainToken))
	fence, ferr := parseFenceEnv(os.Getenv(envMaintainFence))
	if token == "" || ferr != nil {
		fmt.Fprintln(stderr, "hookactions: post-code scheduler missing token/fence")
		return true, fmt.Errorf("hookactions: post-code scheduler missing token/fence")
	}
	if homePath != "" {
		_ = os.Setenv("ATLAS_HOME", homePath)
	}
	ref, err := ResolveAt(root, homePath)
	if err != nil {
		projectID := strings.TrimSpace(os.Getenv(envMaintainProjectID))
		if validInternalProjectID(projectID) {
			_, _ = ClearOwnReservation(homePath, projectID, token, fence)
		}
		return true, nil
	}
	if projectID := strings.TrimSpace(os.Getenv(envMaintainProjectID)); projectID != "" && ref.ProjectID != projectID {
		if validInternalProjectID(projectID) {
			_, _ = ClearOwnReservation(homePath, projectID, token, fence)
		}
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), absoluteProcessDeadline)
	defer cancel()
	_ = RunPostCodeScheduler(ctx, ref, token, fence)
	return true, nil
}
