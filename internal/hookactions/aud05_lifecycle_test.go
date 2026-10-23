package hookactions_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"gopkg.in/yaml.v3"
)

const (
	aud05EnvMode        = "ATLAS_INTERNAL_MAINTAIN_MODE"
	aud05EnvRoot        = "ATLAS_INTERNAL_MAINTAIN_ROOT"
	aud05EnvHome        = "ATLAS_INTERNAL_MAINTAIN_HOME"
	aud05EnvSession     = "ATLAS_INTERNAL_MAINTAIN_SESSION"
	aud05EnvProjectID   = "ATLAS_INTERNAL_MAINTAIN_PROJECT_ID"
	aud05EnvAdmissionID = "ATLAS_INTERNAL_MAINTAIN_ADMISSION_ID"
	aud05EnvToken       = "ATLAS_INTERNAL_MAINTAIN_TOKEN"
	aud05EnvFence       = "ATLAS_INTERNAL_MAINTAIN_FENCE"
	aud05HelperEnv      = "ATLAS_AUD05_PROCESS_HELPER"
)

type aud05Lease struct {
	SchemaVersion int    `yaml:"schema_version"`
	State         string `yaml:"state"`
	PID           int    `yaml:"pid"`
	At            string `yaml:"at"`
	AdmissionID   string `yaml:"admission_id,omitempty"`
}

type aud05SessionEntry struct {
	State       string `yaml:"state"`
	At          string `yaml:"at"`
	PID         int    `yaml:"pid,omitempty"`
	AdmissionID string `yaml:"admission_id,omitempty"`
	ErrorCode   string `yaml:"error_code,omitempty"`
}

type aud05SessionFile struct {
	SchemaVersion int                          `yaml:"schema_version"`
	Sessions      map[string]aud05SessionEntry `yaml:"sessions"`
}

type aud05CancelAfterAdmissionContext struct {
	context.Context
	errCalls atomic.Int32
}

func (c *aud05CancelAfterAdmissionContext) Err() error {
	if c.errCalls.Add(1) > 1 {
		return context.Canceled
	}
	return nil
}

func aud05LeasePath(ref hookactions.ProjectRef) string {
	return filepath.Join(ref.Home, "projects", ref.ProjectID, "state", "maintain.lease")
}

func readAUD05Lease(t *testing.T, ref hookactions.ProjectRef) (aud05Lease, bool) {
	t.Helper()
	raw, err := os.ReadFile(aud05LeasePath(ref))
	if os.IsNotExist(err) {
		return aud05Lease{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	var doc aud05Lease
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc, true
}

func writeAUD05Lease(t *testing.T, ref hookactions.ProjectRef, doc aud05Lease) {
	t.Helper()
	path := aud05LeasePath(ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readAUD05Session(t *testing.T, ref hookactions.ProjectRef, sessionID string) (aud05SessionEntry, bool) {
	t.Helper()
	raw, err := os.ReadFile(hookactions.SessionMaintainPath(ref.Home, ref.ProjectID))
	if os.IsNotExist(err) {
		return aud05SessionEntry{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	var doc aud05SessionFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ent, ok := doc.Sessions[sessionID]
	return ent, ok
}

func writeAUD05Session(t *testing.T, ref hookactions.ProjectRef, sessionID string, ent aud05SessionEntry) {
	t.Helper()
	path := hookactions.SessionMaintainPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	doc := aud05SessionFile{
		SchemaVersion: 1,
		Sessions:      map[string]aud05SessionEntry{sessionID: ent},
	}
	raw, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func aud05AdmissionID(ref hookactions.ProjectRef) string {
	value := reflect.ValueOf(ref)
	field := value.FieldByName("AdmissionID")
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

func aud05Event(ref hookactions.ProjectRef, id, sessionID string) hooks.Event {
	return hooks.Event{
		ID:         id,
		Type:       hooks.EventSessionStart,
		OccurredAt: time.Now().UTC(),
		Adapter:    adapters.Cursor,
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID,
			SessionID: sessionID,
		},
	}
}

func setAUD05DetachedEnv(t *testing.T, ref hookactions.ProjectRef, sessionID, admissionID string) {
	t.Helper()
	t.Setenv(hookactions.EnvInternalMaintain, "1")
	t.Setenv(aud05EnvMode, "session")
	t.Setenv(aud05EnvRoot, ref.Root)
	t.Setenv(aud05EnvHome, ref.Home)
	t.Setenv(aud05EnvSession, sessionID)
	t.Setenv(aud05EnvProjectID, ref.ProjectID)
	t.Setenv(aud05EnvAdmissionID, admissionID)
}

func aud05ProcessEnv(values map[string]string) []string {
	drop := make(map[string]struct{}, len(values))
	for key := range values {
		drop[key] = struct{}{}
	}
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, omitted := drop[key]; !omitted {
			env = append(env, item)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func TestAUD05_SpawnFailureIsObservableAndRetryableWithoutLoop(t *testing.T) {
	ref := c2bRef(t, "aud05-spawn-retry")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	var spawns atomic.Int32
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		spawns.Add(1)
		if id := aud05AdmissionID(got); id == "" {
			t.Error("admission identity missing")
		}
		return errors.New("injected spawn failure")
	})
	result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-spawn-1", "sess-spawn"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err == nil || result.Outcome != hooks.OutcomeFailed {
		t.Fatalf("result=%+v err=%v want spawn failure", result, err)
	}
	entry, ok := readAUD05Session(t, ref, "sess-spawn")
	if !ok || entry.State != "retryable" || entry.ErrorCode != "spawn_failed" || entry.AdmissionID == "" {
		t.Fatalf("spawn failure lifecycle=%+v ok=%v", entry, ok)
	}
	if _, ok := readAUD05Lease(t, ref); ok {
		t.Fatal("owned lease must be cleared after spawn failure")
	}

	// retryable must permit exactly one later admission; its fresh admitted
	// identity must then suppress an immediate duplicate.
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawns.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	second, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-spawn-2", "sess-spawn"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil || second.Outcome != hooks.OutcomeOK {
		t.Fatalf("retry result=%+v err=%v", second, err)
	}
	hookactions.ResetAdmitStateForTest()
	third, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-spawn-3", "sess-spawn"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil || third.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("duplicate result=%+v err=%v", third, err)
	}
	if spawns.Load() != 2 {
		t.Fatalf("spawns=%d want failed attempt + one retry", spawns.Load())
	}
}

func TestAUD05_CancelAfterAdmissionIsObservableWithoutSpawn(t *testing.T) {
	ref := c2bRef(t, "aud05-cancel")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var spawns atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawns.Add(1)
		return nil
	})
	ctx := &aud05CancelAfterAdmissionContext{Context: context.Background()}
	result, err := hookactions.AdmitSessionMaintenance(ctx, aud05Event(ref, "aud05-cancel", "sess-cancel"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if !errors.Is(err, context.Canceled) || result.Outcome != hooks.OutcomeCancelled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	entry, ok := readAUD05Session(t, ref, "sess-cancel")
	if !ok || entry.State != "retryable" || entry.ErrorCode != "cancelled" || entry.AdmissionID == "" {
		t.Fatalf("cancel lifecycle=%+v ok=%v", entry, ok)
	}
	if _, ok := readAUD05Lease(t, ref); ok {
		t.Fatal("cancelled admission left its lease")
	}
	if spawns.Load() != 0 {
		t.Fatalf("spawns=%d want 0", spawns.Load())
	}
}

func TestAUD05_StaleAdmissionCannotClearNewerLifecycle(t *testing.T) {
	ref := c2bRef(t, "aud05-stale-cleanup")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	const newerID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		if aud05AdmissionID(got) == "" {
			t.Error("admission identity missing")
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		writeAUD05Lease(t, ref, aud05Lease{
			SchemaVersion: 1,
			State:         "admitted",
			PID:           os.Getpid(),
			At:            now,
			AdmissionID:   newerID,
		})
		writeAUD05Session(t, ref, "sess-stale", aud05SessionEntry{
			State:       "admitted",
			At:          now,
			PID:         os.Getpid(),
			AdmissionID: newerID,
		})
		return errors.New("late failure from stale admission")
	})

	_, _ = hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-stale", "sess-stale"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	lease, ok := readAUD05Lease(t, ref)
	if !ok || lease.AdmissionID != newerID {
		t.Fatalf("newer lease was cleared: %+v ok=%v", lease, ok)
	}
	entry, ok := readAUD05Session(t, ref, "sess-stale")
	if !ok || entry.AdmissionID != newerID || entry.State != "admitted" {
		t.Fatalf("newer session lifecycle was changed: %+v ok=%v", entry, ok)
	}
}

func TestAUD05_OwnershipLossIsRetryableWithoutClearingForeignLease(t *testing.T) {
	ref := c2bRef(t, "aud05-ownership-loss")
	const (
		staleID   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		foreignID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	writeAUD05Session(t, ref, "sess-lost", aud05SessionEntry{
		State:       "admitted",
		At:          now,
		PID:         1,
		AdmissionID: staleID,
	})
	writeAUD05Lease(t, ref, aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           2,
		At:            now,
		AdmissionID:   foreignID,
	})

	stale := ref
	stale.SessionID = "sess-lost"
	stale.AdmissionID = staleID
	if err := hookactions.InlineSpawner(stale); err != nil {
		t.Fatal(err)
	}
	entry, ok := readAUD05Session(t, ref, "sess-lost")
	if !ok || entry.State != "retryable" || entry.ErrorCode != "ownership_lost" || entry.AdmissionID != staleID {
		t.Fatalf("ownership-loss lifecycle=%+v ok=%v", entry, ok)
	}
	lease, ok := readAUD05Lease(t, ref)
	if !ok || lease.AdmissionID != foreignID || lease.State != "admitted" {
		t.Fatalf("foreign lease changed: %+v ok=%v", lease, ok)
	}
	if _, ok, _ := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID); ok {
		t.Fatal("ownership-lost worker must not start maintain body")
	}
}

func TestAUD05_LegacyLeaseCannotBeClaimedByNewAdmission(t *testing.T) {
	ref := c2bRef(t, "aud05-legacy-owner")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	var captured hookactions.ProjectRef
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		captured = got
		return nil
	})
	result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-legacy-admit", "sess-legacy"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil || result.Outcome != hooks.OutcomeOK || aud05AdmissionID(captured) == "" {
		t.Fatalf("admit result=%+v err=%v id=%q", result, err, aud05AdmissionID(captured))
	}

	legacy := aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           1,
		At:            time.Now().UTC().Format(time.RFC3339Nano),
	}
	writeAUD05Lease(t, ref, legacy)
	before, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}
	if err := hookactions.InlineSpawner(captured); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatalf("new worker removed legacy lease: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("new admission claimed or rewrote legacy lease")
	}
	if _, ok, _ := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID); ok {
		t.Fatal("ownership-lost worker must not start maintain body")
	}
}

func TestAUD05_PartialLifecycleWritesRecoverWithoutDuplicate(t *testing.T) {
	ref := c2bRef(t, "aud05-partial")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var spawns atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawns.Add(1)
		return nil
	})

	const orphanID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	writeAUD05Lease(t, ref, aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           1,
		At:            time.Now().UTC().Format(time.RFC3339Nano),
		AdmissionID:   orphanID,
	})
	fresh, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-partial-1", "sess-partial"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil || fresh.Outcome != hooks.OutcomeSkipped || spawns.Load() != 0 {
		t.Fatalf("fresh lease-only reservation must block: result=%+v err=%v spawns=%d", fresh, err, spawns.Load())
	}

	// A stale lease plus an admitted session is the interruption window after
	// the two writes but before spawn. It must become retryable and progress.
	writeAUD05Lease(t, ref, aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           1,
		At:            time.Now().UTC().Add(-hookactions.AdmitLeaseTTL - time.Second).Format(time.RFC3339Nano),
		AdmissionID:   orphanID,
	})
	writeAUD05Session(t, ref, "sess-partial", aud05SessionEntry{
		State:       "admitted",
		At:          time.Now().UTC().Format(time.RFC3339Nano),
		PID:         1,
		AdmissionID: orphanID,
	})
	hookactions.ResetAdmitStateForTest()
	recovered, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-partial-2", "sess-partial"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil || recovered.Outcome != hooks.OutcomeOK || spawns.Load() != 1 {
		t.Fatalf("stale partial lifecycle did not recover: result=%+v err=%v spawns=%d", recovered, err, spawns.Load())
	}
	entry, ok := readAUD05Session(t, ref, "sess-partial")
	if !ok || entry.State != "admitted" || entry.AdmissionID == "" || entry.AdmissionID == orphanID {
		t.Fatalf("replacement lifecycle=%+v ok=%v", entry, ok)
	}
}

func TestAUD05_SchedulerDefersWhileAdmittedLeaseFresh(t *testing.T) {
	ref := c2bRef(t, "aud05-phase-c-yield")
	_ = installFakeClock(t, time.Date(2026, 10, 22, 14, 0, 0, 0, time.UTC))
	hookactions.SetOwnerHeartbeatIntervalForTest(-1)
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(hookactions.ResetAdmitStateForTest)

	hookactions.SetSpawner(func(hookactions.ProjectRef) error { return nil })
	if result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-yield-admit", "sess-yield"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	})); err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit result=%+v err=%v", result, err)
	}
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	owner, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !owner.Reserved {
		t.Fatalf("reserve=%+v err=%v", owner, err)
	}
	var bodies atomic.Int32
	hookactions.SetPostCodeMaintainForTest(func(context.Context, hookactions.ProjectRef) (string, string, string) {
		bodies.Add(1)
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})

	err = hookactions.RunPostCodeMaintainCycleForTest(context.Background(), ref, owner.Token, owner.Fence)
	if err == nil {
		t.Fatal("fresh admitted lease must make Phase C yield")
	}
	if bodies.Load() != 0 {
		t.Fatalf("Phase C bodies=%d want 0 while session child is admitted", bodies.Load())
	}
	pending, loadErr := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if pending.ProcessedGeneration != 0 || pending.OwnerToken != owner.Token {
		t.Fatalf("Phase C altered pending while yielding: %+v", pending)
	}
}

func TestAUD05_PhaseCResolveFailurePreservesGenerationAndRetries(t *testing.T) {
	ref := c2bRef(t, "aud05-phase-c-resolve-retry")
	advance := installFakeClock(t, time.Date(2026, 10, 22, 15, 0, 0, 0, time.UTC))
	hookactions.SetOwnerHeartbeatIntervalForTest(-1)
	hookactions.SetTriggersEnabled(true)
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	owner, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !owner.Reserved {
		t.Fatalf("reserve=%+v err=%v", owner, err)
	}
	var calls atomic.Int32
	hookactions.SetPostCodeMaintainForTest(func(context.Context, hookactions.ProjectRef) (string, string, string) {
		if calls.Add(1) == 1 {
			return hookactions.MaintainOutcomeSkipped, hookactions.MaintainOutcomeSkipped, hookactions.MaintainErrResolveFailed
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})

	if err := hookactions.RunPostCodeMaintainCycleForTest(context.Background(), ref, owner.Token, owner.Fence); err != nil {
		t.Fatal(err)
	}
	failed, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.PendingGeneration != 1 || failed.ProcessedGeneration != 0 {
		t.Fatalf("resolve failure credited generation: %+v", failed)
	}
	if failed.LastErrorCode != hookactions.MaintainErrResolveFailed || failed.Attempt != 1 ||
		failed.NextRetryAt == "" || failed.DueAt == "" || failed.OwnerToken != owner.Token {
		t.Fatalf("resolve failure did not schedule owned retry: %+v", failed)
	}

	advance(time.Minute)
	if err := hookactions.RunPostCodeScheduler(context.Background(), ref, owner.Token, owner.Fence); err != nil {
		t.Fatal(err)
	}
	recovered, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.PendingGeneration != 1 || recovered.ProcessedGeneration != 1 ||
		recovered.OwnerToken != "" || recovered.LastErrorCode != hookactions.MaintainErrNone {
		t.Fatalf("successful retry did not drain generation: %+v", recovered)
	}
	if calls.Load() != 2 {
		t.Fatalf("maintain calls=%d want resolve failure + successful retry", calls.Load())
	}
}

func TestAUD05_PostSpawnLockBusyBecomesRetryableWithoutMaintainLastOverwrite(t *testing.T) {
	ref := c2bRef(t, "aud05-lock-busy")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var captured hookactions.ProjectRef
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		captured = got
		return nil
	})
	if result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-busy-admit", "sess-busy"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	})); err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit result=%+v err=%v", result, err)
	}

	sentinel := []byte("schema_version: 1\nstatus: complete\nstarted_at: 2026-10-22T00:00:00Z\nfinished_at: 2026-10-22T00:00:01Z\nduration_ms: 1000\nce_outcome: ok\nci_outcome: ok\nerror_code: none\n")
	if err := os.WriteFile(hookactions.MaintainLastPath(ref.Home, ref.ProjectID), sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hookactions.ReleaseMaintainLock(lock) }()

	if err := hookactions.InlineSpawner(captured); err != nil {
		t.Fatal(err)
	}
	entry, ok := readAUD05Session(t, ref, "sess-busy")
	if !ok || entry.State != "retryable" || entry.ErrorCode != "lock_busy" {
		t.Fatalf("lock-busy lifecycle=%+v ok=%v", entry, ok)
	}
	got, err := os.ReadFile(hookactions.MaintainLastPath(ref.Home, ref.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, sentinel) {
		t.Fatal("pre-body lock-busy overwrote maintain-last")
	}
}

func TestAUD05_DetachedSessionResolveFailureClosesOwnedAdmission(t *testing.T) {
	ref := c2bRef(t, "aud05-detached-resolve")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var captured hookactions.ProjectRef
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		captured = got
		return nil
	})
	if result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-resolve-admit", "sess-resolve"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	})); err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit result=%+v err=%v", result, err)
	}
	admissionID := aud05AdmissionID(captured)
	if admissionID == "" {
		t.Fatal("admission identity missing")
	}
	setProjectSchemaVersion(t, ref.Root, -1)
	setAUD05DetachedEnv(t, ref, "sess-resolve", admissionID)

	handled, err := hookactions.MaybeRunInternalMaintain(os.Stdout, os.Stderr)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	entry, ok := readAUD05Session(t, ref, "sess-resolve")
	if !ok || entry.State != "retryable" || entry.ErrorCode != "resolve_failed" || entry.AdmissionID != admissionID {
		t.Fatalf("resolve lifecycle=%+v ok=%v", entry, ok)
	}
	if _, ok := readAUD05Lease(t, ref); ok {
		t.Fatal("resolve-failed worker must clear only its owned lease")
	}
	if _, ok, _ := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID); ok {
		t.Fatal("pre-body resolve failure must not create maintain-last")
	}
}

func TestAUD05_SecondPhaseBResolveFailureIsRetryable(t *testing.T) {
	ref := c2bRef(t, "aud05-second-resolve")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var captured hookactions.ProjectRef
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		captured = got
		return nil
	})
	if result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-second-resolve-admit", "sess-second-resolve"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	})); err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit result=%+v err=%v", result, err)
	}
	setProjectSchemaVersion(t, ref.Root, -1)

	if err := hookactions.InlineSpawner(captured); err != nil {
		t.Fatal(err)
	}
	entry, ok := readAUD05Session(t, ref, "sess-second-resolve")
	if !ok || entry.State != "retryable" || entry.ErrorCode != "resolve_failed" ||
		entry.AdmissionID != captured.AdmissionID {
		t.Fatalf("second resolve lifecycle=%+v ok=%v", entry, ok)
	}
	if _, ok := readAUD05Lease(t, ref); ok {
		t.Fatal("second resolve failure left owned lease")
	}
	rec, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok || rec.ErrorCode != hookactions.MaintainErrResolveFailed {
		t.Fatalf("maintain-last=%+v ok=%v err=%v", rec, ok, err)
	}
}

func TestAUD05_FreshMalformedPersistedAdmissionIDIsPreserved(t *testing.T) {
	ref := c2bRef(t, "aud05-malformed-fresh")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var spawns atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawns.Add(1)
		return nil
	})
	writeAUD05Lease(t, ref, aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           7,
		At:            time.Now().UTC().Format(time.RFC3339Nano),
		AdmissionID:   "INVALID",
	})
	before, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}

	result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-malformed-fresh", "sess-malformed-fresh"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err == nil || result.Outcome != hooks.OutcomeFailed {
		t.Fatalf("fresh malformed lease result=%+v err=%v", result, err)
	}
	after, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("fresh malformed lease was modified")
	}
	evidence, err := filepath.Glob(filepath.Join(filepath.Dir(aud05LeasePath(ref)), "maintain.lease.invalid.*.yaml"))
	if err != nil || len(evidence) != 0 {
		t.Fatalf("fresh malformed lease evidence=%v err=%v", evidence, err)
	}
	if spawns.Load() != 0 {
		t.Fatalf("spawns=%d want 0", spawns.Load())
	}
}

func TestAUD05_ExpiredMalformedPersistedAdmissionIDIsQuarantinedAndRecovered(t *testing.T) {
	ref := c2bRef(t, "aud05-malformed-expired")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var captured hookactions.ProjectRef
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		captured = got
		return nil
	})
	writeAUD05Lease(t, ref, aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           8,
		At:            time.Now().UTC().Add(-hookactions.AdmitLeaseTTL - time.Second).Format(time.RFC3339Nano),
		AdmissionID:   "INVALID",
	})
	original, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}

	result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-malformed-expired", "sess-malformed-expired"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("expired malformed lease result=%+v err=%v", result, err)
	}
	if captured.AdmissionID == "" || captured.AdmissionID == "INVALID" {
		t.Fatalf("replacement admission id=%q", captured.AdmissionID)
	}
	lease, ok := readAUD05Lease(t, ref)
	if !ok || lease.AdmissionID != captured.AdmissionID || lease.State != "admitted" {
		t.Fatalf("replacement lease=%+v ok=%v", lease, ok)
	}
	evidence, err := filepath.Glob(filepath.Join(filepath.Dir(aud05LeasePath(ref)), "maintain.lease.invalid.*.yaml"))
	if err != nil || len(evidence) != 1 {
		t.Fatalf("expired malformed evidence=%v err=%v", evidence, err)
	}
	preserved, err := os.ReadFile(evidence[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preserved, original) {
		t.Fatal("quarantine did not preserve malformed lease bytes")
	}
}

func TestAUD05_PhaseCRecoversExpiredMalformedPersistedAdmissionID(t *testing.T) {
	ref := c2bRef(t, "aud05-malformed-phase-c")
	hookactions.SetTriggersEnabled(true)
	hookactions.SetOwnerHeartbeatIntervalForTest(-1)
	t.Cleanup(func() {
		hookactions.SetPostCodeMaintainForTest(nil)
		hookactions.SetOwnerHeartbeatIntervalForTest(0)
	})
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	owner, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !owner.Reserved {
		t.Fatalf("reserve=%+v err=%v", owner, err)
	}
	writeAUD05Lease(t, ref, aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           9,
		At:            time.Now().UTC().Add(-hookactions.AdmitLeaseTTL - time.Second).Format(time.RFC3339Nano),
		AdmissionID:   "INVALID",
	})
	original, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}
	var bodies atomic.Int32
	hookactions.SetPostCodeMaintainForTest(func(context.Context, hookactions.ProjectRef) (string, string, string) {
		bodies.Add(1)
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})

	if err := hookactions.RunPostCodeMaintainCycleForTest(context.Background(), ref, owner.Token, owner.Fence); err != nil {
		t.Fatal(err)
	}
	if bodies.Load() != 1 {
		t.Fatalf("Phase C bodies=%d want 1", bodies.Load())
	}
	pending, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.PendingGeneration != 1 || pending.ProcessedGeneration != 1 {
		t.Fatalf("Phase C did not drain recovered work: %+v", pending)
	}
	evidence, err := filepath.Glob(filepath.Join(filepath.Dir(aud05LeasePath(ref)), "maintain.lease.invalid.*.yaml"))
	if err != nil || len(evidence) != 1 {
		t.Fatalf("Phase C malformed evidence=%v err=%v", evidence, err)
	}
	preserved, err := os.ReadFile(evidence[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preserved, original) {
		t.Fatal("Phase C quarantine did not preserve malformed lease bytes")
	}
}

func TestAUD05_PhaseCPreservesFreshMalformedPersistedAdmissionID(t *testing.T) {
	ref := c2bRef(t, "aud05-malformed-phase-c-fresh")
	hookactions.SetTriggersEnabled(true)
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	owner, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !owner.Reserved {
		t.Fatalf("reserve=%+v err=%v", owner, err)
	}
	writeAUD05Lease(t, ref, aud05Lease{
		SchemaVersion: 1,
		State:         "admitted",
		PID:           10,
		At:            time.Now().UTC().Format(time.RFC3339Nano),
		AdmissionID:   "INVALID",
	})
	before, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}
	var bodies atomic.Int32
	hookactions.SetPostCodeMaintainForTest(func(context.Context, hookactions.ProjectRef) (string, string, string) {
		bodies.Add(1)
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	t.Cleanup(func() { hookactions.SetPostCodeMaintainForTest(nil) })

	if err := hookactions.RunPostCodeMaintainCycleForTest(context.Background(), ref, owner.Token, owner.Fence); err == nil {
		t.Fatal("fresh malformed lease must fail closed")
	}
	after, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("Phase C modified fresh malformed lease")
	}
	if bodies.Load() != 0 {
		t.Fatalf("Phase C bodies=%d want 0", bodies.Load())
	}
	evidence, err := filepath.Glob(filepath.Join(filepath.Dir(aud05LeasePath(ref)), "maintain.lease.invalid.*.yaml"))
	if err != nil || len(evidence) != 0 {
		t.Fatalf("fresh Phase C malformed evidence=%v err=%v", evidence, err)
	}
	pending, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.PendingGeneration != 1 || pending.ProcessedGeneration != 0 || pending.OwnerToken != owner.Token {
		t.Fatalf("fresh malformed lease changed pending: %+v", pending)
	}
}

func TestAUD05_InvalidDetachedAdmissionIdentityDoesNotMutateLifecycle(t *testing.T) {
	ref := c2bRef(t, "aud05-invalid-identity")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	hookactions.SetSpawner(func(hookactions.ProjectRef) error { return nil })
	if result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-invalid-admit", "sess-invalid"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	})); err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit result=%+v err=%v", result, err)
	}
	leaseBefore, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}
	sessionBefore, err := os.ReadFile(hookactions.SessionMaintainPath(ref.Home, ref.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	setAUD05DetachedEnv(t, ref, "sess-invalid", "../not-an-admission-id")

	handled, err := hookactions.MaybeRunInternalMaintain(os.Stdout, os.Stderr)
	if !handled || err == nil {
		t.Fatalf("handled=%v err=%v want strict identity rejection", handled, err)
	}
	leaseAfter, err := os.ReadFile(aud05LeasePath(ref))
	if err != nil {
		t.Fatal(err)
	}
	sessionAfter, err := os.ReadFile(hookactions.SessionMaintainPath(ref.Home, ref.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leaseAfter, leaseBefore) || !bytes.Equal(sessionAfter, sessionBefore) {
		t.Fatal("invalid detached identity mutated lifecycle")
	}
}

func TestAUD05_DetachedAdmissionIdentityMustBeCanonical(t *testing.T) {
	for _, tc := range []struct {
		name     string
		identity func(string) string
	}{
		{name: "whitespace-only", identity: func(string) string { return "   " }},
		{name: "decorated", identity: func(canonical string) string { return " " + canonical + " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := c2bRef(t, "aud05-detached-strict-"+tc.name)
			hookactions.SetTriggersEnabled(true)
			hookactions.ResetAdmitStateForTest()
			t.Cleanup(func() {
				hookactions.SetSpawner(nil)
				hookactions.ResetAdmitStateForTest()
			})

			var captured hookactions.ProjectRef
			hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
				captured = got
				return nil
			})
			if result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-detached-strict-"+tc.name, "sess-detached-strict"), fixedResolve(projectFixture{
				Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
			})); err != nil || result.Outcome != hooks.OutcomeOK {
				t.Fatalf("admit result=%+v err=%v", result, err)
			}
			canonical := aud05AdmissionID(captured)
			if canonical == "" {
				t.Fatal("canonical admission identity missing")
			}
			leaseBefore, err := os.ReadFile(aud05LeasePath(ref))
			if err != nil {
				t.Fatal(err)
			}
			sessionBefore, err := os.ReadFile(hookactions.SessionMaintainPath(ref.Home, ref.ProjectID))
			if err != nil {
				t.Fatal(err)
			}

			setAUD05DetachedEnv(t, ref, "sess-detached-strict", tc.identity(canonical))
			handled, runErr := hookactions.MaybeRunInternalMaintain(os.Stdout, os.Stderr)

			leaseAfter, leaseErr := os.ReadFile(aud05LeasePath(ref))
			sessionAfter, sessionErr := os.ReadFile(hookactions.SessionMaintainPath(ref.Home, ref.ProjectID))
			if leaseErr != nil || sessionErr != nil {
				t.Fatalf("read lifecycle after strict rejection: lease=%v session=%v", leaseErr, sessionErr)
			}
			if !bytes.Equal(leaseAfter, leaseBefore) || !bytes.Equal(sessionAfter, sessionBefore) {
				t.Error("non-canonical detached identity mutated lifecycle")
			}
			if !handled || runErr == nil {
				t.Fatalf("handled=%v err=%v want strict identity rejection", handled, runErr)
			}
		})
	}
}

func TestAUD05_DetachedSchedulerResolveFailureReleasesOwnReservation(t *testing.T) {
	ref := c2bRef(t, "aud05-scheduler-resolve")
	hookactions.SetTriggersEnabled(true)
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	owner, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !owner.Reserved {
		t.Fatalf("reserve=%+v err=%v", owner, err)
	}
	setProjectSchemaVersion(t, ref.Root, -1)
	t.Setenv(hookactions.EnvInternalMaintain, "1")
	t.Setenv(aud05EnvMode, "post_code_scheduler")
	t.Setenv(aud05EnvRoot, ref.Root)
	t.Setenv(aud05EnvHome, ref.Home)
	t.Setenv(aud05EnvProjectID, ref.ProjectID)
	t.Setenv(aud05EnvToken, owner.Token)
	t.Setenv(aud05EnvFence, strconv.FormatInt(owner.Fence, 10))

	handled, err := hookactions.MaybeRunInternalMaintain(os.Stdout, os.Stderr)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	pending, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.OwnerToken != "" {
		t.Fatalf("resolve-failed scheduler left owner=%q fence=%d", pending.OwnerToken, pending.OwnerFence)
	}
	if pending.PendingGeneration != 1 || pending.ProcessedGeneration != 0 {
		t.Fatalf("resolve failure must preserve dirty work: %+v", pending)
	}
}

func TestAUD05_DetachedResolveFailureAcrossProcessBoundary(t *testing.T) {
	ref := c2bRef(t, "aud05-process-boundary")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	var captured hookactions.ProjectRef
	hookactions.SetSpawner(func(got hookactions.ProjectRef) error {
		captured = got
		return nil
	})
	if result, err := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-process-admit", "sess-process"), fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	})); err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit result=%+v err=%v", result, err)
	}
	setProjectSchemaVersion(t, ref.Root, -1)

	cmd := exec.Command(os.Args[0], "-test.run=^TestAUD05_InternalProcessHelper$")
	cmd.Env = aud05ProcessEnv(map[string]string{
		aud05HelperEnv:                  "1",
		hookactions.EnvInternalMaintain: "1",
		aud05EnvMode:                    "session",
		aud05EnvRoot:                    ref.Root,
		aud05EnvHome:                    ref.Home,
		aud05EnvSession:                 "sess-process",
		aud05EnvProjectID:               ref.ProjectID,
		aud05EnvAdmissionID:             captured.AdmissionID,
		"ATLAS_HOME":                    ref.Home,
	})
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("detached helper: %v\n%s", err, output)
	}
	entry, ok := readAUD05Session(t, ref, "sess-process")
	if !ok || entry.State != "retryable" || entry.ErrorCode != "resolve_failed" || entry.AdmissionID != captured.AdmissionID {
		t.Fatalf("process lifecycle=%+v ok=%v", entry, ok)
	}
	if _, ok := readAUD05Lease(t, ref); ok {
		t.Fatal("process-boundary resolve failure left its lease")
	}
}

func TestAUD05_InternalProcessHelper(t *testing.T) {
	if os.Getenv(aud05HelperEnv) != "1" {
		t.Skip("subprocess helper")
	}
	handled, err := hookactions.MaybeRunInternalMaintain(os.Stdout, os.Stderr)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}

func TestAUD05_LockOrderMaintainBeforePendingCompletesWithoutDeadlock(t *testing.T) {
	ref := c2bRef(t, "aud05-lock-order")
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	pendingLock, err := hookactions.TryAcquirePendingLock(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = hookactions.ReleasePendingLock(pendingLock)
		}
	})

	spawned := make(chan struct{}, 1)
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawned <- struct{}{}
		return nil
	})
	waitingPending := make(chan struct{})
	var once sync.Once
	hookactions.SetPendingLockBusyObserverForTest(func() {
		once.Do(func() { close(waitingPending) })
	})
	t.Cleanup(func() { hookactions.SetPendingLockBusyObserverForTest(nil) })
	done := make(chan error, 1)
	go func() {
		result, admitErr := hookactions.AdmitSessionMaintenance(context.Background(), aud05Event(ref, "aud05-lock-order", "sess-lock-order"), fixedResolve(projectFixture{
			Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
		}))
		if admitErr == nil && result.Outcome != hooks.OutcomeOK {
			admitErr = errors.New("unexpected admission outcome: " + string(result.Outcome))
		}
		done <- admitErr
	}()

	// While pending.lock is held, admission must first hold maintain.lock and
	// then wait for pending.lock. No production path may acquire in reverse.
	select {
	case <-waitingPending:
	case <-time.After(time.Second):
		t.Fatal("admission did not wait on pending.lock")
	}
	probe, probeErr := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID)
	if !hookactions.IsMaintainBusyForTest(probeErr) {
		if probeErr == nil {
			_ = hookactions.ReleaseMaintainLock(probe)
		}
		t.Fatalf("admission did not hold maintain.lock before pending.lock: %v", probeErr)
	}

	if err := hookactions.ReleasePendingLock(pendingLock); err != nil {
		t.Fatal(err)
	}
	released = true
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("admission deadlocked after pending.lock release")
	}
	select {
	case <-spawned:
	default:
		t.Fatal("admission completed without spawning")
	}
}
