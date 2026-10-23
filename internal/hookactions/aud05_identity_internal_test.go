package hookactions

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	strictCanonicalID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	strictDecoratedID = " aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa "
	strictWhitespace  = "   "
)

func writeStrictIdentityLease(t *testing.T, homePath, projectID, state, admissionID string, at time.Time) []byte {
	t.Helper()
	if err := writeMaintainLeaseDocument(homePath, projectID, maintainLease{
		SchemaVersion: 1,
		State:         state,
		PID:           101,
		At:            at.UTC().Format(time.RFC3339Nano),
		AdmissionID:   admissionID,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(homePath, filepath.FromSlash(maintainLeaseRel(projectID))))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeStrictIdentitySession(t *testing.T, homePath, projectID, sessionID, admissionID string) {
	t.Helper()
	if err := writeSessionMaintain(homePath, projectID, sessionMaintainFile{
		SchemaVersion: 1,
		Sessions: map[string]sessionMaintainEntry{
			sessionID: {
				State:       sessionStateAdmitted,
				At:          time.Now().UTC().Format(time.RFC3339Nano),
				PID:         102,
				AdmissionID: admissionID,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func strictIdentityEvidence(t *testing.T, homePath, projectID string) []string {
	t.Helper()
	pattern := filepath.Join(
		homePath,
		filepath.FromSlash(pendingStateDirRel(projectID)),
		maintainLeaseInvalidPrefix+"*.yaml",
	)
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestAUD05_StrictAdmissionIdentityCanonicalOnly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "canonical", value: strictCanonicalID, valid: true},
		{name: "empty", value: "", valid: false},
		{name: "whitespace-only", value: strictWhitespace, valid: false},
		{name: "decorated", value: strictDecoratedID, valid: false},
		{name: "uppercase", value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validAdmissionID(tc.value); got != tc.valid {
				t.Fatalf("validAdmissionID(%q)=%v want %v", tc.value, got, tc.valid)
			}
		})
	}
}

func TestAUD05_NonCanonicalIdentityNeverOwnsLeaseOrSession(t *testing.T) {
	for _, tc := range []struct {
		name      string
		persisted string
		caller    string
	}{
		{name: "whitespace-is-not-legacy", persisted: strictWhitespace, caller: ""},
		{name: "decorated-does-not-match-canonical", persisted: strictDecoratedID, caller: strictCanonicalID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if leaseOwnedBy(maintainLease{AdmissionID: tc.persisted}, tc.caller) {
				t.Fatalf("non-canonical lease identity %q owned by %q", tc.persisted, tc.caller)
			}
			if sessionOwnedBy(sessionMaintainEntry{AdmissionID: tc.persisted}, tc.caller) {
				t.Fatalf("non-canonical session identity %q owned by %q", tc.persisted, tc.caller)
			}
		})
	}
}

func TestAUD05_CanonicalWorkerRejectsDecoratedLifecycle(t *testing.T) {
	homePath := t.TempDir()
	const (
		projectID = "aud05-strict-worker-0123456789"
		sessionID = "sess-strict-worker"
	)
	leaseBefore := writeStrictIdentityLease(t, homePath, projectID, leaseStateAdmitted, strictDecoratedID, time.Now())
	writeStrictIdentitySession(t, homePath, projectID, sessionID, strictDecoratedID)

	lock, err := TryAcquireMaintainLock(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, claimErr := claimAdmissionForWorker(ProjectRef{
		Home:        homePath,
		ProjectID:   projectID,
		SessionID:   sessionID,
		AdmissionID: strictCanonicalID,
	})
	_ = ReleaseMaintainLock(lock)
	if claimErr != nil || claimed {
		t.Fatalf("decorated lifecycle claim=%v err=%v want unclaimed", claimed, claimErr)
	}

	leaseAfter, err := os.ReadFile(filepath.Join(homePath, filepath.FromSlash(maintainLeaseRel(projectID))))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leaseAfter, leaseBefore) {
		t.Fatal("failed decorated claim modified lease")
	}
	sessions, err := readSessionMaintain(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessions.Sessions[sessionID].AdmissionID; got != strictDecoratedID {
		t.Fatalf("failed decorated claim changed session identity to %q", got)
	}
}

func TestAUD05_PhaseBStrictInvalidIdentityPolicy(t *testing.T) {
	for _, identity := range []string{strictWhitespace, strictDecoratedID} {
		name := "decorated"
		if identity == strictWhitespace {
			name = "whitespace-only"
		}
		t.Run(name+"/fresh", func(t *testing.T) {
			homePath := t.TempDir()
			const projectID = "aud05-strict-phaseb-fresh"
			before := writeStrictIdentityLease(t, homePath, projectID, leaseStateAdmitted, identity, time.Now())

			lock, err := TryAcquireMaintainLock(homePath, projectID)
			if err != nil {
				t.Fatal(err)
			}
			blocked, establishErr := establishAdmissionLifecycle(homePath, projectID, "", strictCanonicalID, time.Now().UTC())
			_ = ReleaseMaintainLock(lock)
			if !errors.Is(establishErr, errInvalidPersistedAdmissionID) || blocked {
				t.Fatalf("fresh non-canonical blocked=%v err=%v", blocked, establishErr)
			}
			after, err := os.ReadFile(filepath.Join(homePath, filepath.FromSlash(maintainLeaseRel(projectID))))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) || len(strictIdentityEvidence(t, homePath, projectID)) != 0 {
				t.Fatal("fresh non-canonical lease was modified or quarantined")
			}
		})

		t.Run(name+"/expired", func(t *testing.T) {
			homePath := t.TempDir()
			const projectID = "aud05-strict-phaseb-expired"
			before := writeStrictIdentityLease(
				t,
				homePath,
				projectID,
				leaseStateAdmitted,
				identity,
				time.Now().Add(-AdmitLeaseTTL-time.Second),
			)

			lock, err := TryAcquireMaintainLock(homePath, projectID)
			if err != nil {
				t.Fatal(err)
			}
			blocked, establishErr := establishAdmissionLifecycle(homePath, projectID, "", strictCanonicalID, time.Now().UTC())
			_ = ReleaseMaintainLock(lock)
			if establishErr != nil || blocked {
				t.Fatalf("expired non-canonical blocked=%v err=%v", blocked, establishErr)
			}
			lease, ok, err := readMaintainLease(homePath, projectID)
			if err != nil || !ok || lease.State != leaseStateAdmitted || lease.AdmissionID != strictCanonicalID {
				t.Fatalf("replacement lease=%+v ok=%v err=%v", lease, ok, err)
			}
			evidence := strictIdentityEvidence(t, homePath, projectID)
			if len(evidence) != 1 {
				t.Fatalf("evidence=%v want one", evidence)
			}
			preserved, err := os.ReadFile(evidence[0])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(preserved, before) {
				t.Fatal("Phase B quarantine did not preserve non-canonical lease")
			}
		})
	}
}

func TestAUD05_PhaseCStrictInvalidIdentityPolicy(t *testing.T) {
	for _, identity := range []string{strictWhitespace, strictDecoratedID} {
		name := "decorated"
		if identity == strictWhitespace {
			name = "whitespace-only"
		}
		t.Run(name+"/fresh", func(t *testing.T) {
			homePath := t.TempDir()
			const projectID = "aud05-strict-phasec-fresh"
			before := writeStrictIdentityLease(t, homePath, projectID, leaseStateAdmitted, identity, time.Now())

			lock, err := TryAcquireMaintainLock(homePath, projectID)
			if err != nil {
				t.Fatal(err)
			}
			ready, prepareErr := prepareSchedulerLease(homePath, projectID)
			_ = ReleaseMaintainLock(lock)
			if !errors.Is(prepareErr, errInvalidPersistedAdmissionID) || ready {
				t.Fatalf("fresh non-canonical ready=%v err=%v", ready, prepareErr)
			}
			after, err := os.ReadFile(filepath.Join(homePath, filepath.FromSlash(maintainLeaseRel(projectID))))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) || len(strictIdentityEvidence(t, homePath, projectID)) != 0 {
				t.Fatal("fresh non-canonical Phase C lease was modified or quarantined")
			}
		})

		t.Run(name+"/expired", func(t *testing.T) {
			homePath := t.TempDir()
			const projectID = "aud05-strict-phasec-expired"
			before := writeStrictIdentityLease(
				t,
				homePath,
				projectID,
				leaseStateAdmitted,
				identity,
				time.Now().Add(-AdmitLeaseTTL-time.Second),
			)

			lock, err := TryAcquireMaintainLock(homePath, projectID)
			if err != nil {
				t.Fatal(err)
			}
			ready, prepareErr := prepareSchedulerLease(homePath, projectID)
			_ = ReleaseMaintainLock(lock)
			if prepareErr != nil || !ready {
				t.Fatalf("expired non-canonical ready=%v err=%v", ready, prepareErr)
			}
			lease, ok, err := readMaintainLease(homePath, projectID)
			if err != nil || !ok || lease.State != leaseStateRunning || lease.AdmissionID != "" {
				t.Fatalf("Phase C replacement lease=%+v ok=%v err=%v", lease, ok, err)
			}
			evidence := strictIdentityEvidence(t, homePath, projectID)
			if len(evidence) != 1 {
				t.Fatalf("evidence=%v want one", evidence)
			}
			preserved, err := os.ReadFile(evidence[0])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(preserved, before) {
				t.Fatal("Phase C quarantine did not preserve non-canonical lease")
			}
		})
	}
}

func TestAUD05_ExactEmptyAdmissionIdentityRemainsLegacy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		explicit bool
	}{
		{name: "absent"},
		{name: "exact-empty", explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			homePath := t.TempDir()
			const (
				projectID = "aud05-strict-legacy-0123456789"
				sessionID = "sess-strict-legacy"
			)
			if tc.explicit {
				stateDir := filepath.Join(homePath, filepath.FromSlash(pendingStateDirRel(projectID)))
				if err := os.MkdirAll(stateDir, 0o700); err != nil {
					t.Fatal(err)
				}
				leaseRaw := []byte(
					"schema_version: 1\nstate: admitted\npid: 201\nat: " +
						time.Now().UTC().Format(time.RFC3339Nano) +
						"\nadmission_id: \"\"\n",
				)
				if err := os.WriteFile(filepath.Join(homePath, filepath.FromSlash(maintainLeaseRel(projectID))), leaseRaw, 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				writeStrictIdentityLease(t, homePath, projectID, leaseStateAdmitted, "", time.Now())
			}
			writeStrictIdentitySession(t, homePath, projectID, sessionID, "")

			lock, err := TryAcquireMaintainLock(homePath, projectID)
			if err != nil {
				t.Fatal(err)
			}
			claimed, claimErr := claimAdmissionForWorker(ProjectRef{
				Home:      homePath,
				ProjectID: projectID,
				SessionID: sessionID,
			})
			_ = ReleaseMaintainLock(lock)
			if claimErr != nil || !claimed {
				t.Fatalf("legacy claim=%v err=%v", claimed, claimErr)
			}
		})
	}
}
