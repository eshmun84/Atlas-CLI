package hookactions

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

const stabCanonicalID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestLifecycle_AbsenceIsNotCorruption(t *testing.T) {
	home, project := stabState(t)
	lease, ok, err := readMaintainLease(home, project)
	if err != nil || ok || lease.State != "" {
		t.Fatalf("lease=%+v ok=%v err=%v", lease, ok, err)
	}
	doc, err := readSessionMaintain(home, project)
	if err != nil || len(doc.Sessions) != 0 || doc.SchemaVersion != 1 {
		t.Fatalf("session=%+v err=%v", doc, err)
	}
	if n := len(stabEvidence(t, home, project, "maintain.lease.corrupt.")); n != 0 {
		t.Fatalf("absence created %d lease evidence files", n)
	}
	if n := len(stabEvidence(t, home, project, "session-maintain.corrupt.")); n != 0 {
		t.Fatalf("absence created %d session evidence files", n)
	}
}

func TestLifecycle_CorruptLeaseIsQuarantinedNotAbsent(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("lease: [\n")
	writeStab(t, home, project, maintainLeaseName, raw)
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	blocked, err := establishAdmissionLifecycle(home, project, "sess-stab", stabCanonicalID, time.Now().UTC())
	if err != nil || blocked {
		t.Fatalf("blocked=%v err=%v", blocked, err)
	}
	preserved := stabEvidenceBytes(t, home, project, maintainLeaseCorruptPrefix)
	if !bytes.Equal(preserved, raw) {
		t.Fatalf("evidence=%q want %q", preserved, raw)
	}
	lease, ok, err := readMaintainLease(home, project)
	if err != nil || !ok || lease.AdmissionID != stabCanonicalID || lease.State != leaseStateAdmitted {
		t.Fatalf("replacement lease=%+v ok=%v err=%v", lease, ok, err)
	}
}

func TestLifecycle_FutureLeaseSchemaStaysUntouched(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("schema_version: 99\nstate: admitted\nadmission_id: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n")
	writeStab(t, home, project, maintainLeaseName, raw)
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	_, err := establishAdmissionLifecycle(home, project, "sess-stab", stabCanonicalID, time.Now().UTC())
	if !errors.Is(err, errLeaseSchemaIncompatible) {
		t.Fatalf("err=%v", err)
	}
	if got := readStab(t, home, project, maintainLeaseName); !bytes.Equal(got, raw) {
		t.Fatalf("future lease changed to %q", got)
	}
	if n := len(stabEvidence(t, home, project, "maintain.lease.")); n != 0 {
		t.Fatalf("future schema produced evidence: %v", n)
	}
}

func TestLifecycle_FreshInvalidLeaseIsNotRecovered(t *testing.T) {
	home, project := stabState(t)
	before := writeStrictIdentityLease(t, home, project, leaseStateAdmitted, "INVALID", time.Now().UTC())
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	blocked, err := establishAdmissionLifecycle(home, project, "sess-stab", stabCanonicalID, time.Now().UTC())
	if !errors.Is(err, errInvalidPersistedAdmissionID) || blocked {
		t.Fatalf("blocked=%v err=%v", blocked, err)
	}
	if got := readStab(t, home, project, maintainLeaseName); !bytes.Equal(got, before) {
		t.Fatal("fresh invalid lease was modified")
	}
	if n := len(stabEvidence(t, home, project, "maintain.lease.")); n != 0 {
		t.Fatalf("fresh invalid lease produced evidence")
	}
}

func TestLifecycle_CorruptSessionIsNotEmpty(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("sessions: [\n")
	writeStab(t, home, project, sessionMaintainName, raw)
	if _, err := readSessionMaintain(home, project); !errors.Is(err, errSessionMalformed) {
		t.Fatalf("read err=%v want malformed", err)
	}
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	blocked, err := establishAdmissionLifecycle(home, project, "sess-stab", stabCanonicalID, time.Now().UTC())
	if err != nil || blocked {
		t.Fatalf("blocked=%v err=%v", blocked, err)
	}
	if preserved := stabEvidenceBytes(t, home, project, sessionMaintainCorruptPrefix); !bytes.Equal(preserved, raw) {
		t.Fatalf("evidence=%q", preserved)
	}
	doc, err := readSessionMaintain(home, project)
	if err != nil {
		t.Fatal(err)
	}
	ent, ok := doc.Sessions["sess-stab"]
	if !ok || ent.State != sessionStateAdmitted || ent.AdmissionID != stabCanonicalID {
		t.Fatalf("session after recovery=%+v ok=%v", ent, ok)
	}
}

func TestLifecycle_FutureSessionSchemaStaysUntouched(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("schema_version: 99\nsessions:\n  sess-stab:\n    state: admitted\n    at: 2026-10-22T00:00:00Z\n    admission_id: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n")
	writeStab(t, home, project, sessionMaintainName, raw)
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	_, err := establishAdmissionLifecycle(home, project, "sess-stab", stabCanonicalID, time.Now().UTC())
	if !errors.Is(err, errSessionSchemaIncompatible) {
		t.Fatalf("err=%v", err)
	}
	if got := readStab(t, home, project, sessionMaintainName); !bytes.Equal(got, raw) {
		t.Fatalf("future session changed to %q", got)
	}
	if n := len(stabEvidence(t, home, project, "session-maintain.")); n != 0 {
		t.Fatalf("future session produced evidence")
	}
}

func TestLifecycle_InvalidSessionDoesNotDelayAndKeepsSibling(t *testing.T) {
	home, project := stabState(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	keepID := "cccccccccccccccccccccccccccccccc"
	if err := writeSessionMaintain(home, project, sessionMaintainFile{
		SchemaVersion: 1,
		Sessions: map[string]sessionMaintainEntry{
			"sess-keep": {State: sessionStateComplete, At: now, AdmissionID: keepID},
			"sess-stab": {State: sessionStateAdmitted, At: now, AdmissionID: "INVALID"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	original := readStab(t, home, project, sessionMaintainName)
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	blocked, err := establishAdmissionLifecycle(home, project, "sess-stab", stabCanonicalID, time.Now().UTC())
	if err != nil || blocked {
		t.Fatalf("blocked=%v err=%v", blocked, err)
	}
	if preserved := stabEvidenceBytes(t, home, project, sessionMaintainInvalidPrefix); !bytes.Equal(preserved, original) {
		t.Fatal("invalid session evidence does not match the primary")
	}
	doc, err := readSessionMaintain(home, project)
	if err != nil {
		t.Fatal(err)
	}
	keep := doc.Sessions["sess-keep"]
	if keep.State != sessionStateComplete || keep.AdmissionID != keepID {
		t.Fatalf("sibling=%+v", keep)
	}
	got := doc.Sessions["sess-stab"]
	if got.State != sessionStateAdmitted || got.AdmissionID != stabCanonicalID {
		t.Fatalf("recovered session=%+v", got)
	}
	if _, ok := doc.Sessions["sess-stab"]; ok && got.AdmissionID == "INVALID" {
		t.Fatal("invalid identity still present")
	}
}

func TestLifecycle_CanonicalCompleteStillBlocks(t *testing.T) {
	home, project := stabState(t)
	if err := writeSessionMaintain(home, project, sessionMaintainFile{
		SchemaVersion: 1,
		Sessions: map[string]sessionMaintainEntry{
			"sess-stab": {
				State:       sessionStateComplete,
				At:          time.Now().UTC().Format(time.RFC3339Nano),
				AdmissionID: stabCanonicalID,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	before := readStab(t, home, project, sessionMaintainName)
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	blocked, err := establishAdmissionLifecycle(home, project, "sess-stab", "dddddddddddddddddddddddddddddddd", time.Now().UTC())
	if err != nil || !blocked {
		t.Fatalf("blocked=%v err=%v", blocked, err)
	}
	if got := readStab(t, home, project, sessionMaintainName); !bytes.Equal(got, before) {
		t.Fatal("complete session was rewritten")
	}
}

func TestLifecycle_QuarantineFailureKeepsCorruptPrimary(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("lease: [\n")
	writeStab(t, home, project, maintainLeaseName, raw)
	fsafety.SetDurableSyncForTest(func(*os.File) error { return errors.New("injected sync") }, nil)
	t.Cleanup(func() { fsafety.SetDurableSyncForTest(nil, nil) })
	lock := holdMaintain(t, home, project)
	defer releaseMaintain(t, lock)

	_, err := establishAdmissionLifecycle(home, project, "sess-stab", stabCanonicalID, time.Now().UTC())
	if err == nil {
		t.Fatal("sync failure was ignored")
	}
	if got := readStab(t, home, project, maintainLeaseName); !bytes.Equal(got, raw) {
		t.Fatalf("primary=%q", got)
	}
	if n := len(stabEvidence(t, home, project, maintainLeaseCorruptPrefix)); n != 0 {
		t.Fatalf("failed quarantine left %d evidence files", n)
	}
}

func TestLifecycle_ConcurrentSessionRecoveryIsIdempotent(t *testing.T) {
	home, project := stabState(t)
	raw := []byte("sessions: [\n")
	writeStab(t, home, project, sessionMaintainName, raw)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- withPendingLock(home, project, func() error {
				return recoverMalformedSessionLocked(home, project)
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	paths := stabEvidence(t, home, project, sessionMaintainCorruptPrefix)
	if len(paths) != 1 {
		t.Fatalf("evidence files=%d want 1", len(paths))
	}
	if got, err := os.ReadFile(paths[0]); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("evidence=%q err=%v", got, err)
	}
	if _, err := readSessionMaintain(home, project); err != nil {
		t.Fatal(err)
	}
	if err := recoverMalformedSessionLocked(home, project); err != nil {
		t.Fatal(err)
	}
	if again := stabEvidence(t, home, project, sessionMaintainCorruptPrefix); len(again) != 1 {
		t.Fatalf("second recovery created evidence files=%d", len(again))
	}
}

func TestLifecycle_FailedClaimDoesNotStripDecoratedSession(t *testing.T) {
	home, project := stabState(t)
	const sessionID = "sess-claim"
	writeStrictIdentityLease(t, home, project, leaseStateAdmitted, "INVALID", time.Now().UTC())
	writeStrictIdentitySession(t, home, project, sessionID, "INVALID")
	beforeLease := readStab(t, home, project, maintainLeaseName)
	beforeSession := readStab(t, home, project, sessionMaintainName)
	lock := holdMaintain(t, home, project)
	claimed, err := claimAdmissionForWorker(ProjectRef{
		Home: home, ProjectID: project, SessionID: sessionID, AdmissionID: stabCanonicalID,
	})
	releaseMaintain(t, lock)
	if err != nil || claimed {
		t.Fatalf("claimed=%v err=%v", claimed, err)
	}
	if !bytes.Equal(readStab(t, home, project, maintainLeaseName), beforeLease) {
		t.Fatal("failed claim modified lease")
	}
	if !bytes.Equal(readStab(t, home, project, sessionMaintainName), beforeSession) {
		t.Fatal("failed claim modified session")
	}
}

func stabState(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	const project = "stab-proj"
	if err := os.MkdirAll(filepath.Join(home, "projects", project, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, project
}

func writeStab(t *testing.T, home, project, name string, data []byte) {
	t.Helper()
	path := filepath.Join(home, "projects", project, "state", name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readStab(t *testing.T, home, project, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "projects", project, "state", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func holdMaintain(t *testing.T, home, project string) *os.File {
	t.Helper()
	f, err := TryAcquireMaintainLock(home, project)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func releaseMaintain(t *testing.T, f *os.File) {
	t.Helper()
	if err := ReleaseMaintainLock(f); err != nil {
		t.Fatal(err)
	}
}

func stabEvidence(t *testing.T, home, project, prefix string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(home, "projects", project, "state", prefix+"*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func stabEvidenceBytes(t *testing.T, home, project, prefix string) []byte {
	t.Helper()
	paths := stabEvidence(t, home, project, prefix)
	if len(paths) != 1 {
		t.Fatalf("evidence=%v want one %s", paths, prefix)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	return data
}
