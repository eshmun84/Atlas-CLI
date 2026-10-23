package hookactions

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAUD05_PreBodyLoserCannotClearRunningSameAdmission(t *testing.T) {
	homePath := t.TempDir()
	const (
		projectID   = "aud05-0123456789abcdef"
		sessionID   = "sess-duplicate"
		admissionID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	if err := withPendingLock(homePath, projectID, func() error {
		if err := writeMaintainLease(homePath, projectID, leaseStateRunning, admissionID); err != nil {
			return err
		}
		return markSessionAdmittedLocked(homePath, projectID, sessionID, admissionID)
	}); err != nil {
		t.Fatal(err)
	}

	closeAdmissionRetryable(homePath, projectID, sessionID, admissionID, admissionErrLockBusy)

	lease, ok, err := readMaintainLease(homePath, projectID)
	if err != nil || !ok || lease.State != leaseStateRunning || lease.AdmissionID != admissionID {
		t.Fatalf("running owner changed: lease=%+v ok=%v err=%v", lease, ok, err)
	}
	sessions, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := sessions.Sessions[sessionID]
	if !ok || entry.State != sessionStateAdmitted || entry.AdmissionID != admissionID {
		t.Fatalf("running owner's session changed: entry=%+v ok=%v", entry, ok)
	}
	if at, ok := parseSessionAt(entry.At); !ok || time.Since(at) > time.Minute {
		t.Fatalf("unexpected session timestamp %q", entry.At)
	}
}

func TestAUD05_DuplicateChildCannotReopenCompletedAdmission(t *testing.T) {
	homePath := t.TempDir()
	const (
		projectID   = "aud05-duplicate-0123456789abcdef"
		sessionID   = "sess-duplicate-complete"
		admissionID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		nextID      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	ref := ProjectRef{
		Home:        homePath,
		ProjectID:   projectID,
		SessionID:   sessionID,
		AdmissionID: admissionID,
	}
	if err := withPendingLock(homePath, projectID, func() error {
		if err := writeMaintainLease(homePath, projectID, leaseStateAdmitted, admissionID); err != nil {
			return err
		}
		return markSessionAdmittedLocked(homePath, projectID, sessionID, admissionID)
	}); err != nil {
		t.Fatal(err)
	}

	w1Lock, err := TryAcquireMaintainLock(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := claimAdmissionForWorker(ref)
	if err != nil || !claimed {
		_ = ReleaseMaintainLock(w1Lock)
		t.Fatalf("W1 claim=%v err=%v", claimed, err)
	}

	waiting := make(chan struct{})
	var once sync.Once
	workerMaintainLockBusyObserver = func() {
		once.Do(func() { close(waiting) })
	}
	t.Cleanup(func() { workerMaintainLockBusyObserver = nil })

	w2Done := make(chan error, 1)
	go func() {
		w2Done <- RunWorker(context.Background(), ref)
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		_ = ReleaseMaintainLock(w1Lock)
		t.Fatal("W2 did not wait on maintain.lock")
	}

	// W1 completes its effective maintenance lifecycle while W2 is waiting.
	markSessionCompleteOwned(homePath, projectID, sessionID, admissionID)
	clearMaintainLeaseOwned(homePath, projectID, admissionID)
	if err := ReleaseMaintainLock(w1Lock); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-w2Done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("W2 did not finish after W1 released maintain.lock")
	}
	workerMaintainLockBusyObserver = nil

	sessions, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := sessions.Sessions[sessionID]
	if !ok || entry.State != sessionStateComplete || entry.AdmissionID != admissionID {
		t.Fatalf("stale W2 changed terminal state: entry=%+v ok=%v", entry, ok)
	}

	lock, err := TryAcquireMaintainLock(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	blocked, establishErr := establishAdmissionLifecycle(homePath, projectID, sessionID, nextID, time.Now().UTC())
	_ = ReleaseMaintainLock(lock)
	if establishErr != nil || !blocked {
		t.Fatalf("post-complete admission blocked=%v err=%v", blocked, establishErr)
	}
	if _, ok, err := readMaintainLease(homePath, projectID); err != nil || ok {
		t.Fatalf("post-complete event created lease ok=%v err=%v", ok, err)
	}
}
