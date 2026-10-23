package hookactions_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

func writeAUD04Pending(t *testing.T, ref hookactions.ProjectRef, raw []byte) string {
	t.Helper()
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireAUD04Evidence(t *testing.T, ref hookactions.ProjectRef, raw []byte, want int) {
	t.Helper()
	paths, err := hookactions.ListMaintainPendingCorruptEvidence(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != want {
		t.Fatalf("evidence files=%d want %d", len(paths), want)
	}
	if want == 1 {
		got, err := os.ReadFile(paths[0])
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(raw) {
			t.Fatalf("quarantine body=%q want %q", got, raw)
		}
	}
}

func TestAUD04_PostCodeRecoversMalformedPendingAndResumesScheduler(t *testing.T) {
	ref := c2bRef(t, "aud04-post-code")
	_ = installFakeClock(t, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	raw := []byte("schema_version: 1\npending_generation: nope\n")
	writeAUD04Pending(t, ref, raw)

	var spawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(got hookactions.ProjectRef, token string, fence int64) error {
		spawns.Add(1)
		if got.ProjectID != ref.ProjectID || token == "" || fence != 1 {
			t.Errorf("spawn ref=%q token=%q fence=%d", got.ProjectID, token, fence)
		}
		return nil
	})

	if err := hookactions.BumpPendingAndEnsureScheduler(ref); err != nil {
		t.Fatal(err)
	}
	requireAUD04Evidence(t, ref, raw, 1)
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 {
		t.Fatalf("recovery epoch=%d/%d want 1/0", doc.PendingGeneration, doc.ProcessedGeneration)
	}
	if doc.OwnerToken == "" || doc.OwnerFence != 1 || doc.OwnerState != "reserved" {
		t.Fatalf("scheduler reservation not persisted: %+v", doc)
	}
	if spawns.Load() != 1 {
		t.Fatalf("scheduler spawns=%d want 1", spawns.Load())
	}
}

func TestAUD04_SessionStartRecoversMalformedPending(t *testing.T) {
	ref := c2bRef(t, "aud04-session-start")
	_ = installFakeClock(t, time.Date(2026, 10, 22, 12, 5, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(hookactions.ResetAdmitStateForTest)
	raw := []byte("schema_version: [\n")
	writeAUD04Pending(t, ref, raw)

	hookactions.SetSpawner(func(hookactions.ProjectRef) error { return nil })
	t.Cleanup(func() { hookactions.SetSpawner(nil) })
	var schedulerSpawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(hookactions.ProjectRef, string, int64) error {
		schedulerSpawns.Add(1)
		return nil
	})

	ev := hooks.Event{
		ID:         "aud04-session-event",
		Type:       hooks.EventSessionStart,
		OccurredAt: time.Now().UTC(),
		Adapter:    adapters.Cursor,
		Payload: hooks.Payload{
			SessionID: "aud04-session",
			ProjectID: ref.ProjectID,
		},
	}
	result, err := hookactions.AdmitSessionMaintenance(context.Background(), ev, fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil || result.Outcome != hooks.OutcomeOK {
		t.Fatalf("session admission result=%+v err=%v", result, err)
	}
	requireAUD04Evidence(t, ref, raw, 1)
	if schedulerSpawns.Load() != 1 {
		t.Fatalf("scheduler spawns=%d want 1", schedulerSpawns.Load())
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 {
		t.Fatalf("recovery epoch=%d/%d want 1/0", doc.PendingGeneration, doc.ProcessedGeneration)
	}
}

func TestAUD04_FuturePendingSchemaNeverRecovered(t *testing.T) {
	for _, entrypoint := range []struct {
		name string
		run  func(hookactions.ProjectRef) error
	}{
		{name: "post-code", run: hookactions.BumpPendingAndEnsureScheduler},
		{name: "session-recovery", run: hookactions.EnsurePostCodeSchedulerIfDirty},
		{name: "quarantine-primitive", run: func(ref hookactions.ProjectRef) error {
			return hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID)
		}},
	} {
		t.Run(entrypoint.name, func(t *testing.T) {
			ref := c2bRef(t, "aud04-future-"+entrypoint.name)
			hookactions.SetTriggersEnabled(true)
			// A future schema may legitimately change field shapes. Classification
			// must inspect its schema before decoding the current typed document.
			raw := []byte("schema_version: 99\npending_generation: [7]\nprocessed_generation: {value: 3}\n")
			path := writeAUD04Pending(t, ref, raw)
			var spawns atomic.Int32
			hookactions.SetSchedulerSpawner(func(hookactions.ProjectRef, string, int64) error {
				spawns.Add(1)
				return nil
			})
			t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })

			err := entrypoint.run(ref)
			if err == nil || !hookactions.ErrPendingCorrupt(err) {
				t.Fatalf("error=%v want diagnostic incompatible pending error", err)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != string(raw) {
				t.Fatal("future schema primary was modified")
			}
			requireAUD04Evidence(t, ref, raw, 0)
			if spawns.Load() != 0 {
				t.Fatalf("scheduler spawns=%d want 0", spawns.Load())
			}
		})
	}
}

func TestAUD04_ValidAndSparseLegacyPendingDoNotRecover(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  []byte
	}{
		{
			name: "valid-dirty",
			raw:  []byte("schema_version: 1\npending_generation: 2\nprocessed_generation: 1\ndue_at: 2026-10-22T00:00:00Z\n"),
		},
		{
			name: "sparse-schema-one",
			raw:  []byte("schema_version: 1\n"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ref := c2bRef(t, "aud04-"+test.name)
			hookactions.SetTriggersEnabled(true)
			writeAUD04Pending(t, ref, test.raw)
			var spawns atomic.Int32
			hookactions.SetSchedulerSpawner(func(hookactions.ProjectRef, string, int64) error {
				spawns.Add(1)
				return nil
			})
			t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })

			if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
				t.Fatal(err)
			}
			requireAUD04Evidence(t, ref, test.raw, 0)
			wantSpawns := int32(0)
			if test.name == "valid-dirty" {
				wantSpawns = 1
			}
			if spawns.Load() != wantSpawns {
				t.Fatalf("spawns=%d want %d", spawns.Load(), wantSpawns)
			}
		})
	}
}

func TestAUD04_ProductiveRecoveryPropagatesQuarantineFailure(t *testing.T) {
	ref := c2bRef(t, "aud04-quarantine-failure")
	hookactions.SetTriggersEnabled(true)
	raw := []byte("bad: [[")
	path := writeAUD04Pending(t, ref, raw)
	var calls atomic.Int32
	hookactions.SetCorruptEvidenceWriterForTest(func(string, string, []byte) (string, error) {
		calls.Add(1)
		return "", errors.New("aud04 injected quarantine failure")
	})
	t.Cleanup(func() { hookactions.SetCorruptEvidenceWriterForTest(nil) })

	err := hookactions.EnsurePostCodeSchedulerIfDirty(ref)
	if err == nil || !strings.Contains(err.Error(), "aud04 injected quarantine failure") {
		t.Fatalf("error=%v want injected quarantine failure", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("quarantine attempts=%d want 1", calls.Load())
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(raw) {
		t.Fatal("primary changed after quarantine failure")
	}
	requireAUD04Evidence(t, ref, raw, 0)
}

func TestAUD04_ProductiveRecoveryPropagatesResetFailure(t *testing.T) {
	ref := c2bRef(t, "aud04-reset-failure")
	hookactions.SetTriggersEnabled(true)
	raw := []byte("bad: [[")
	path := writeAUD04Pending(t, ref, raw)
	var calls atomic.Int32
	hookactions.SetRecoveryPendingWriterForTest(func(string, string, []byte) error {
		calls.Add(1)
		return errors.New("aud04 injected reset failure")
	})
	t.Cleanup(func() { hookactions.SetRecoveryPendingWriterForTest(nil) })

	err := hookactions.BumpPendingAndEnsureScheduler(ref)
	if err == nil || !strings.Contains(err.Error(), "aud04 injected reset failure") {
		t.Fatalf("error=%v want injected reset failure", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("reset attempts=%d want 1", calls.Load())
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(raw) {
		t.Fatal("primary changed after reset failure")
	}
	requireAUD04Evidence(t, ref, raw, 1)
}

func TestAUD04_ConcurrentProductiveRecoveryCreatesOneQuarantine(t *testing.T) {
	ref := c2bRef(t, "aud04-concurrent")
	hookactions.SetTriggersEnabled(true)
	raw := []byte("bad: [[")
	writeAUD04Pending(t, ref, raw)
	var spawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(hookactions.ProjectRef, string, int64) error {
		spawns.Add(1)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })

	const n = 8
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- hookactions.EnsurePostCodeSchedulerIfDirty(ref)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent recovery: %v", err)
		}
	}
	requireAUD04Evidence(t, ref, raw, 1)
	if spawns.Load() != 1 {
		t.Fatalf("scheduler spawns=%d want 1", spawns.Load())
	}
}

func TestAUD04_RecoveryRetryIsBoundedPerEntrypoint(t *testing.T) {
	for _, entrypoint := range []struct {
		name string
		run  func(hookactions.ProjectRef) error
	}{
		{name: "post-code", run: hookactions.BumpPendingAndEnsureScheduler},
		{name: "session-recovery", run: hookactions.EnsurePostCodeSchedulerIfDirty},
	} {
		t.Run(entrypoint.name, func(t *testing.T) {
			ref := c2bRef(t, "aud04-bounded-"+entrypoint.name)
			hookactions.SetTriggersEnabled(true)
			raw := []byte("bad: [[")
			writeAUD04Pending(t, ref, raw)
			var resets atomic.Int32
			hookactions.SetRecoveryPendingWriterForTest(func(homePath, rel string, _ []byte) error {
				resets.Add(1)
				path := filepath.Join(homePath, filepath.FromSlash(rel))
				return os.WriteFile(path, []byte("still: [["), 0o600)
			})
			t.Cleanup(func() { hookactions.SetRecoveryPendingWriterForTest(nil) })

			err := entrypoint.run(ref)
			if err == nil || !hookactions.ErrPendingCorrupt(err) {
				t.Fatalf("error=%v want corrupt after one recovery attempt", err)
			}
			if resets.Load() != 1 {
				t.Fatalf("recovery attempts=%d want 1", resets.Load())
			}
			requireAUD04Evidence(t, ref, raw, 1)
		})
	}
}

func TestAUD04_AUD01GatePrecedesPendingRecovery(t *testing.T) {
	ref := c2bRef(t, "aud04-aud01-gate")
	setProjectSchemaVersion(t, ref.Root, config.CurrentProjectSchemaVersion+1)
	hookactions.SetTriggersEnabled(true)
	raw := []byte("bad: [[")
	path := writeAUD04Pending(t, ref, raw)
	var spawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(hookactions.ProjectRef, string, int64) error {
		spawns.Add(1)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })

	action := &hookactions.PostCodeAction{}
	result, err := action.Run(context.Background(), c2cToolEvent(
		adapters.Cursor, "Write", "success", "aud04-session", ref.ProjectID, "aud04-gate-event", ref.Root,
	))
	if err != nil || result.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("result=%+v err=%v want fail-open schema rejection", result, err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(raw) {
		t.Fatal("pending changed before project schema validation")
	}
	requireAUD04Evidence(t, ref, raw, 0)
	if spawns.Load() != 0 {
		t.Fatalf("scheduler spawns=%d want 0", spawns.Load())
	}
}

func TestAUD04_TwoCorruptBumpsCoalesceAndLateBumpRequeues(t *testing.T) {
	ref := c2bRef(t, "aud04-two-corrupt-bumps")
	_ = installFakeClock(t, time.Date(2026, 10, 22, 13, 0, 0, 0, time.UTC))
	hookactions.SetOwnerHeartbeatIntervalForTest(-1)
	hookactions.SetTriggersEnabled(true)
	raw := []byte("schema_version: 1\npending_generation: nope\n")
	writeAUD04Pending(t, ref, raw)

	// Deterministically exercise the prefix of two concurrent
	// BumpPendingAndEnsureScheduler calls. BumpPendingEdit cannot alter a
	// corrupt primary, so both signals must observe corruption before recovery.
	start := make(chan struct{})
	bumpErrs := make(chan error, 2)
	var bumps sync.WaitGroup
	for i := 0; i < 2; i++ {
		bumps.Add(1)
		go func() {
			defer bumps.Done()
			<-start
			_, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
			bumpErrs <- err
		}()
	}
	close(start)
	bumps.Wait()
	close(bumpErrs)
	for err := range bumpErrs {
		if !hookactions.ErrPendingCorrupt(err) {
			t.Fatalf("concurrent bump error=%v want corrupt pending", err)
		}
	}

	// Exercise the recovery suffixes in a legal order: the winner installs the
	// dirty epoch and the loser revalidates the now-valid primary and no-ops.
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	requireAUD04Evidence(t, ref, raw, 1)
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 || doc.OwnerToken != "" {
		t.Fatalf("recovered pending=%+v want dirty unowned epoch 1/0", doc)
	}

	type reservation struct {
		token string
		fence int64
	}
	reservations := make(chan reservation, 1)
	var spawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(_ hookactions.ProjectRef, token string, fence int64) error {
		if spawns.Add(1) == 1 {
			reservations <- reservation{token: token, fence: fence}
		}
		return nil
	})

	// These are the two wrapper suffixes. Exactly one may reserve/spawn.
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if spawns.Load() != 1 {
		t.Fatalf("scheduler spawns=%d want 1", spawns.Load())
	}
	res := <-reservations

	maintainEntered := make(chan struct{})
	releaseMaintain := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseMaintain) }) }
	t.Cleanup(release)

	var cycles atomic.Int32
	var firstGeneration atomic.Int64
	hookactions.SetPostCodeMaintainForTest(func(context.Context, hookactions.ProjectRef) (string, string, string) {
		n := cycles.Add(1)
		rec, ok, loadErr := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
		if loadErr != nil {
			t.Errorf("load maintain-last: %v", loadErr)
		} else if !ok {
			t.Error("maintain-last missing after generation capture")
		} else if n == 1 {
			firstGeneration.Store(rec.Generation)
			close(maintainEntered)
			<-releaseMaintain
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})

	schedulerDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	go func() {
		schedulerDone <- hookactions.RunPostCodeScheduler(ctx, ref, res.token, res.fence)
	}()

	select {
	case <-maintainEntered:
	case <-ctx.Done():
		t.Fatalf("scheduler did not enter first maintain cycle: %v", ctx.Err())
	}
	if firstGeneration.Load() != 1 {
		t.Fatalf("first captured generation=%d want 1", firstGeneration.Load())
	}

	// This signal is strictly after gen0=1 was captured. It must become a
	// distinct pending generation and must not spawn a second scheduler.
	if err := hookactions.BumpPendingAndEnsureScheduler(ref); err != nil {
		t.Fatal(err)
	}
	during, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if during.PendingGeneration != 2 || during.ProcessedGeneration != 0 {
		t.Fatalf("during first cycle pending=%d processed=%d want 2/0", during.PendingGeneration, during.ProcessedGeneration)
	}
	if spawns.Load() != 1 {
		t.Fatalf("late bump spawned another scheduler: spawns=%d", spawns.Load())
	}

	release()
	select {
	case err := <-schedulerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatalf("scheduler did not drain generations: %v", ctx.Err())
	}

	final, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if cycles.Load() != 2 {
		t.Fatalf("maintain cycles=%d want 2", cycles.Load())
	}
	if final.PendingGeneration != 2 || final.ProcessedGeneration != 2 {
		t.Fatalf("final pending=%d processed=%d want 2/2", final.PendingGeneration, final.ProcessedGeneration)
	}
	requireAUD04Evidence(t, ref, raw, 1)
}
