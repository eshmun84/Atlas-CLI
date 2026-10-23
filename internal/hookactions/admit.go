package hookactions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// AdmissionTimeout bounds the hook-process admission path (resolve + spawn).
// Must stay well under adapter subprocess budgets (OpenCode = 10s).
const AdmissionTimeout = 3 * time.Second

// WorkerTimeout bounds the detached (or inline) maintenance worker.
const WorkerTimeout = 3 * time.Minute

// EnvInternalMaintain is set on detached worker processes. Not a public CLI flag.
const EnvInternalMaintain = "ATLAS_INTERNAL_MAINTAIN"

const (
	envMaintainRoot        = "ATLAS_INTERNAL_MAINTAIN_ROOT"
	envMaintainHome        = "ATLAS_INTERNAL_MAINTAIN_HOME"
	envMaintainSession     = "ATLAS_INTERNAL_MAINTAIN_SESSION"
	envMaintainProjectID   = "ATLAS_INTERNAL_MAINTAIN_PROJECT_ID"
	envMaintainAdmissionID = "ATLAS_INTERNAL_MAINTAIN_ADMISSION_ID"
)

type admitSource string

const (
	admitSourceSessionStart   admitSource = "session.start"
	admitSourceCursorFallback admitSource = "cursor.prompt_fallback"
)

// Spawner starts Home maintenance for a project. Production detaches a child
// process so adapter hook subprocesses can exit without aborting work.
type Spawner func(ref ProjectRef) error

var (
	spawnerMu sync.Mutex
	spawner   Spawner = spawnDetachedWorker

	// inProcessAdmit dedupes CE + CodeIntel Actions within one Dispatch
	// (same process, same event id).
	inProcessAdmit sync.Map // eventID → struct{}
)

// SetSpawner overrides the production detach spawner (tests).
func SetSpawner(fn Spawner) {
	spawnerMu.Lock()
	defer spawnerMu.Unlock()
	if fn == nil {
		spawner = spawnDetachedWorker
		return
	}
	spawner = fn
}

func currentSpawner() Spawner {
	spawnerMu.Lock()
	defer spawnerMu.Unlock()
	return spawner
}

// ResetAdmitStateForTest clears in-process admission dedupe.
func ResetAdmitStateForTest() {
	inProcessAdmit.Range(func(key, _ any) bool {
		inProcessAdmit.Delete(key)
		return true
	})
}

// AdmitSessionMaintenance is the Phase B/B.2 hook admission path for session.start.
// Fast: resolve → same-event dedupe → session fence → flock + spawn lease → spawn worker.
//
// Cross-process fence (FIX-B2-01):
//  1. Acquire maintain.lock
//  2. If a fresh "admitted" lease exists → skip (another spawn in flight)
//  3. Write "admitted" lease
//  4. Release lock BEFORE spawn (so the worker can acquire immediately)
//  5. Spawn worker; on spawn failure clear the lease
//
// The lease closes the TOCTOU window between release and worker lock acquire.
// Holding the flock across spawn caused workers to observe BUSY and exit no-op.
//
// Phase B.8: also records per-session admission so Cursor beforeSubmitPrompt
// fallback does not spawn a second worker for the same conversation.
func AdmitSessionMaintenance(ctx context.Context, ev hooks.Event, resolve ResolveFunc) (hooks.Result, error) {
	switch ev.Type {
	case hooks.EventSessionStart:
		return admitSessionMaintenance(ctx, ev, resolve, admitSourceSessionStart)
	default:
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
}

func admitSessionMaintenance(ctx context.Context, ev hooks.Event, resolve ResolveFunc, source admitSource) (hooks.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return hooks.Result{Outcome: hooks.OutcomeCancelled}, err
	}
	if !TriggersAreEnabled() {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}

	ref, err := resolveForAdmission(ctx, resolve)
	if err != nil {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	// Phase C.2b: recover dirty post-code work without altering Phase B admit
	// semantics. Best-effort; never changes the returned Result.
	defer func() { _ = EnsurePostCodeSchedulerIfDirty(ref) }()

	if payloadID := strings.TrimSpace(ev.Payload.ProjectID); payloadID != "" && payloadID != ref.ProjectID {
		return hooks.Result{Outcome: hooks.OutcomeFailed}, fmt.Errorf("%w: id mismatch", errAdmitProject)
	}
	if err := openCodeSessionAllowed(ev.Adapter, ref.Root, ev.Payload.SessionDirectory); err != nil {
		return hooks.Result{Outcome: hooks.OutcomeFailed}, err
	}

	sessionID := strings.TrimSpace(ev.Payload.SessionID)
	if source == admitSourceCursorFallback && !ValidSessionID(sessionID) {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	// session.start always carries a validated session_id; still refuse path-like IDs.
	if source == admitSourceSessionStart && sessionID != "" && !ValidSessionID(sessionID) {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}

	// Same-event dedupe across CE + CodeIntel Actions in one Dispatch.
	// Cross-event / cross-process session dedupe uses session-maintain.yaml + lease.
	eventKey := strings.TrimSpace(ev.ID)
	if eventKey == "" {
		eventKey = ref.ProjectID + "|" + string(ev.Type) + "|" + string(source)
	}
	if _, loaded := inProcessAdmit.LoadOrStore(eventKey, struct{}{}); loaded {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}

	lock, err := TryAcquireMaintainLock(ref.Home, ref.ProjectID)
	if err != nil {
		if errors.Is(err, errMaintainBusy) {
			return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
		}
		return hooks.Result{Outcome: hooks.OutcomeFailed}, fmt.Errorf("%w: %v", errAdmitUnexpected, err)
	}

	admissionID, err := newAdmissionID()
	if err != nil {
		_ = ReleaseMaintainLock(lock)
		return hooks.Result{Outcome: hooks.OutcomeFailed}, fmt.Errorf("%w: %v", errAdmitUnexpected, err)
	}
	blocked, err := establishAdmissionLifecycle(ref.Home, ref.ProjectID, sessionID, admissionID, time.Now().UTC())
	if err != nil {
		_ = ReleaseMaintainLock(lock)
		return hooks.Result{Outcome: hooks.OutcomeFailed}, fmt.Errorf("%w: %v", errAdmitUnexpected, err)
	}
	if blocked {
		_ = ReleaseMaintainLock(lock)
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}

	// Release before spawn so the worker never observes BUSY from this parent.
	_ = ReleaseMaintainLock(lock)

	if err := ctx.Err(); err != nil {
		closeAdmissionRetryable(ref.Home, ref.ProjectID, sessionID, admissionID, admissionErrCancelled)
		return hooks.Result{Outcome: hooks.OutcomeCancelled}, err
	}

	ref.SessionID = sessionID
	ref.AdmissionID = admissionID
	if err := currentSpawner()(ref); err != nil {
		closeAdmissionRetryable(ref.Home, ref.ProjectID, sessionID, admissionID, admissionErrSpawnFailed)
		return hooks.Result{Outcome: hooks.OutcomeFailed}, fmt.Errorf("%w: %v", errAdmitSpawn, err)
	}
	return hooks.Result{Outcome: hooks.OutcomeOK}, nil
}

// establishAdmissionLifecycle performs the lease→session write sequence under
// pending.lock while the caller holds maintain.lock. There is deliberately no
// inverse path: code holding pending.lock must never acquire maintain.lock.
func establishAdmissionLifecycle(homePath, projectID, sessionID, admissionID string, now time.Time) (bool, error) {
	blocked := false
	err := withPendingLock(homePath, projectID, func() error {
		if err := recoverMalformedLeaseLocked(homePath, projectID); err != nil {
			return err
		}
		if err := recoverMalformedSessionLocked(homePath, projectID); err != nil {
			return err
		}
		if err := recoverNonCooperativeSessionLocked(homePath, projectID); err != nil {
			return err
		}
		lease, leaseOK, err := readMaintainLease(homePath, projectID)
		if err != nil {
			return err
		}
		if leaseOK {
			leaseID := lease.AdmissionID
			if leaseID != "" && !validAdmissionID(leaseID) {
				if !invalidLeaseRecoverable(lease, now) {
					return errInvalidPersistedAdmissionID
				}
				if err := quarantineInvalidMaintainLeaseLocked(homePath, projectID); err != nil {
					return fmt.Errorf("%w: %v", errInvalidPersistedAdmissionID, err)
				}
				leaseOK = false
			}
		}
		if leaseOK && leaseBlocksAdmission(lease, now) {
			blocked = true
			return nil
		}
		if leaseOK {
			leaseID := lease.AdmissionID
			if leaseID != "" {
				if err := markAdmissionSessionsRetryableLocked(homePath, projectID, leaseID, admissionErrWorkerInterrupted); err != nil {
					return err
				}
			}
			clearMaintainLeaseOwnedLocked(homePath, projectID, leaseID)
		}

		if ValidSessionID(sessionID) {
			doc, err := readSessionMaintain(homePath, projectID)
			if err != nil {
				return err
			}
			if ent, ok := doc.Sessions[strings.TrimSpace(sessionID)]; ok {
				at, parsed := parseSessionAt(ent.At)
				age := now.Sub(at)
				if age < 0 {
					age = 0
				}
				switch strings.TrimSpace(ent.State) {
				case sessionStateComplete:
					if parsed && age <= SessionRecordTTL {
						blocked = true
						return nil
					}
				case sessionStateAdmitted:
					if parsed && age <= SessionAdmittedTTL {
						blocked = true
						return nil
					}
				}
			}
		}

		if err := writeMaintainLease(homePath, projectID, leaseStateAdmitted, admissionID); err != nil {
			return err
		}
		if ValidSessionID(sessionID) {
			if err := markSessionAdmittedLocked(homePath, projectID, sessionID, admissionID); err != nil {
				clearMaintainLeaseOwnedLocked(homePath, projectID, admissionID)
				return err
			}
		}
		return nil
	})
	return blocked, err
}

// closeAdmissionRetryable records the observable pre-body outcome first, then
// clears only the matching lease. A crash between writes leaves a retryable
// session plus a short-lived lease; later admission waits for lease expiry.
func closeAdmissionRetryable(homePath, projectID, sessionID, admissionID, errorCode string) {
	if !validAdmissionID(admissionID) || !validInternalProjectID(projectID) {
		return
	}
	_ = withPendingLock(homePath, projectID, func() error {
		lease, ok, err := readMaintainLease(homePath, projectID)
		if err != nil {
			return err
		}
		if ok {
			// A running lease may belong to another process executing the same
			// admitted job. A pre-body loser must not close that worker's state.
			if leaseOwnedBy(lease, admissionID) && strings.TrimSpace(lease.State) != leaseStateAdmitted {
				return nil
			}
		}
		if ValidSessionID(sessionID) {
			if err := markSessionStateOwnedLocked(homePath, projectID, sessionID, admissionID, sessionStateAdmitted, sessionStateRetryable, errorCode); err != nil {
				return err
			}
		}
		if ok && leaseOwnedBy(lease, admissionID) {
			clearMaintainLeaseOwnedLocked(homePath, projectID, admissionID)
		}
		return nil
	})
}

func resolveForAdmission(ctx context.Context, resolve ResolveFunc) (ProjectRef, error) {
	if resolve != nil {
		return resolve()
	}
	if ref, ok := ProjectFromContext(ctx); ok {
		return ref, nil
	}
	return ProjectRef{}, fmt.Errorf("%w: project context required", errAdmitProject)
}
