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
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

func c2bRef(t *testing.T, name string) hookactions.ProjectRef {
	t.Helper()
	f := seedInitializedProject(t, name)
	return hookactions.ProjectRef{
		Root: f.Root, Home: f.Home, ProjectID: f.ProjectID, Name: f.Name, State: f.State,
	}
}

func installFakeClock(t *testing.T, start time.Time) (advance func(d time.Duration)) {
	t.Helper()
	now := start.UTC()
	var mu sync.Mutex
	hookactions.SetPendingClockForTest(func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	})
	hookactions.SetPendingSleepForTest(func(d time.Duration) {
		mu.Lock()
		now = now.Add(d)
		mu.Unlock()
	})
	t.Cleanup(func() {
		hookactions.SetPendingClockForTest(nil)
		hookactions.SetPendingSleepForTest(nil)
		hookactions.SetAbsoluteProcessDeadlineForTest(0)
		hookactions.SetOwnerHeartbeatIntervalForTest(0)
		hookactions.SetPostCodeMaintainForTest(nil)
		hookactions.SetSchedulerSpawner(nil)
		hookactions.SetCorruptEvidenceWriterForTest(nil)
		hookactions.SetRecoveryPendingWriterForTest(nil)
	})
	return func(d time.Duration) {
		mu.Lock()
		now = now.Add(d)
		mu.Unlock()
	}
}

func TestC2B_ConcurrentBumps(t *testing.T) {
	ref := c2bRef(t, "c2b-bumps")
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
				t.Errorf("bump: %v", err)
			}
		}()
	}
	wg.Wait()
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != n {
		t.Fatalf("pending_generation=%d want %d", doc.PendingGeneration, n)
	}
	if doc.ProcessedGeneration != 0 {
		t.Fatalf("processed=%d want 0", doc.ProcessedGeneration)
	}
}

func TestC2B_SingleReservationUnderParallel(t *testing.T) {
	ref := c2bRef(t, "c2b-reserve")
	for i := 0; i < 5; i++ {
		if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
			t.Fatal(err)
		}
	}
	var reserved atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			if res.Reserved {
				reserved.Add(1)
			}
		}()
	}
	wg.Wait()
	if reserved.Load() != 1 {
		t.Fatalf("reserved=%d want 1", reserved.Load())
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.OwnerFence != 1 {
		t.Fatalf("fence=%d want 1", doc.OwnerFence)
	}
	if doc.OwnerToken == "" {
		t.Fatal("expected owner token")
	}
}

func TestC2B_SpawnFailureClearsOnlyOwnReservation(t *testing.T) {
	ref := c2bRef(t, "c2b-spawn-fail")
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	hookactions.SetSchedulerSpawner(func(ref hookactions.ProjectRef, token string, fence int64) error {
		return errors.New("spawn boom")
	})
	t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })

	err := hookactions.EnsurePostCodeSchedulerIfDirty(ref)
	if err == nil {
		t.Fatal("expected spawn error")
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.OwnerToken != "" {
		t.Fatalf("ownership should be cleared, token=%q", doc.OwnerToken)
	}
	if doc.PendingGeneration <= doc.ProcessedGeneration {
		t.Fatal("work must remain dirty")
	}

	// Second ensure can reserve again (fence advances).
	var gotFence int64
	hookactions.SetSchedulerSpawner(func(ref hookactions.ProjectRef, token string, fence int64) error {
		gotFence = fence
		return nil
	})
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if gotFence != 2 {
		t.Fatalf("fence=%d want 2", gotFence)
	}
}

func TestC2B_StaleSchedulerFenced(t *testing.T) {
	ref := c2bRef(t, "c2b-stale")
	advance := installFakeClock(t, time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	res1, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res1.Reserved {
		t.Fatalf("reserve1: %+v err=%v", res1, err)
	}
	// Expire lease and reclaim via second reservation.
	advance(hookactions.OwnerLeaseTTL + time.Second)
	res2, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res2.Reserved {
		t.Fatalf("reserve2: %+v err=%v", res2, err)
	}
	if res2.Fence <= res1.Fence {
		t.Fatalf("fence2=%d should exceed fence1=%d", res2.Fence, res1.Fence)
	}

	// Stale commit must not apply.
	out, err := hookactions.CommitGeneration(ref.Home, ref.ProjectID, res1.Token, res1.Fence, 1, true, hookactions.MaintainErrNone)
	if err != nil {
		t.Fatal(err)
	}
	if out.Applied {
		t.Fatal("stale commit must not apply")
	}
	cleared, err := hookactions.ClearOwnReservation(ref.Home, ref.ProjectID, res1.Token, res1.Fence)
	if err != nil {
		t.Fatal(err)
	}
	if cleared {
		t.Fatal("stale clear must be no-op")
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.OwnerToken != res2.Token || doc.OwnerFence != res2.Fence {
		t.Fatalf("successor ownership corrupted: %+v", doc)
	}
	if doc.ProcessedGeneration != 0 {
		t.Fatalf("processed advanced by stale commit: %d", doc.ProcessedGeneration)
	}
}

func TestC2B_LeaseExpiryAndHeartbeat(t *testing.T) {
	ref := c2bRef(t, "c2b-lease")
	advance := installFakeClock(t, time.Date(2026, 10, 19, 13, 0, 0, 0, time.UTC))
	start := time.Date(2026, 10, 19, 13, 0, 0, 0, time.UTC)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	res, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res.Reserved {
		t.Fatal(err)
	}
	advance(hookactions.OwnerLeaseTTL - time.Second)
	ok, err := hookactions.HeartbeatOwner(ref.Home, ref.ProjectID, res.Token, res.Fence, "running_wait")
	if err != nil || !ok {
		t.Fatalf("heartbeat: ok=%v err=%v", ok, err)
	}
	advance(2 * time.Second) // past original lease, within renewed
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if !hookactions.OwnerVigenteForTest(doc, start.Add(hookactions.OwnerLeaseTTL-time.Second+2*time.Second)) {
		t.Fatal("owner should remain vigente after heartbeat")
	}
	advance(hookactions.OwnerLeaseTTL + time.Second)
	reclaimed, err := hookactions.ReclaimExpiredOwner(ref.Home, ref.ProjectID)
	if err != nil || !reclaimed {
		t.Fatalf("reclaim: %v %v", reclaimed, err)
	}
}

func TestC2B_CommitGenerationRuleDirect(t *testing.T) {
	ref := c2bRef(t, "c2b-commit-rule")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 14, 0, 0, 0, time.UTC))
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	res, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res.Reserved {
		t.Fatal(err)
	}
	gen0, ok, err := hookactions.BeginMaintainCapture(ref.Home, ref.ProjectID, res.Token, res.Fence)
	if err != nil || !ok || gen0 != 1 {
		t.Fatalf("begin gen0=%d ok=%v err=%v", gen0, ok, err)
	}
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID) // pending=2 during work
	out, err := hookactions.CommitGeneration(ref.Home, ref.ProjectID, res.Token, res.Fence, gen0, true, hookactions.MaintainErrNone)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Applied || !out.StillOwned || !out.HasMore {
		t.Fatalf("commit out=%+v", out)
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.ProcessedGeneration != 1 {
		t.Fatalf("processed=%d want gen0=1 (must not credit pending=2)", doc.ProcessedGeneration)
	}
	if doc.PendingGeneration != 2 {
		t.Fatalf("pending=%d want 2", doc.PendingGeneration)
	}
}

func TestC2B_EditDuringCE(t *testing.T) {
	ref := c2bRef(t, "c2b-edit-ce")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 14, 5, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	var calls atomic.Int32
	var genAtFirst atomic.Int64
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		n := calls.Add(1)
		if n == 1 {
			doc, _ := hookactions.LoadMaintainPending(r.Home, r.ProjectID)
			genAtFirst.Store(doc.PendingGeneration)
			_, _ = hookactions.BumpPendingEdit(r.Home, r.ProjectID) // edit during CE
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if genAtFirst.Load() != 1 {
		t.Fatalf("gen at first CE=%d want 1", genAtFirst.Load())
	}
	if calls.Load() < 2 {
		t.Fatalf("expected requeue second cycle, calls=%d", calls.Load())
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != doc.ProcessedGeneration || doc.ProcessedGeneration != 2 {
		t.Fatalf("final pending=%d processed=%d", doc.PendingGeneration, doc.ProcessedGeneration)
	}
}

func TestC2B_EditDuringCI(t *testing.T) {
	ref := c2bRef(t, "c2b-edit-ci")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 14, 10, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	var calls atomic.Int32
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		if calls.Add(1) == 1 {
			_, _ = hookactions.BumpPendingEdit(r.Home, r.ProjectID)
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if calls.Load() < 2 {
		t.Fatalf("requeue expected, calls=%d", calls.Load())
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.ProcessedGeneration != 2 || doc.PendingGeneration != 2 {
		t.Fatalf("processed=%d pending=%d", doc.ProcessedGeneration, doc.PendingGeneration)
	}
}

func TestC2B_EditImmediatelyBeforeCommit(t *testing.T) {
	ref := c2bRef(t, "c2b-precommit")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 14, 20, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	// Bump at end of maintain body — CommitGeneration re-reads pending under lock.
	var calls atomic.Int32
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		if calls.Add(1) == 1 {
			_, _ = hookactions.BumpPendingEdit(r.Home, r.ProjectID)
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	_ = hookactions.EnsurePostCodeSchedulerIfDirty(ref)
	if calls.Load() < 2 {
		t.Fatalf("requeue expected, calls=%d", calls.Load())
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.ProcessedGeneration != 2 {
		t.Fatalf("processed=%d", doc.ProcessedGeneration)
	}
}

func TestC2B_PartialMaintainFailure(t *testing.T) {
	ref := c2bRef(t, "c2b-partial")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 14, 30, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeFailed, hookactions.MaintainErrCIFailed
	})
	// Avoid infinite retry loop: short absolute deadline + sleep advances clock.
	hookactions.SetAbsoluteProcessDeadlineForTest(5 * time.Second)
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	_ = hookactions.EnsurePostCodeSchedulerIfDirty(ref)

	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.ProcessedGeneration != 0 {
		t.Fatalf("processed advanced on failure: %d", doc.ProcessedGeneration)
	}
	if doc.PendingGeneration != 1 {
		t.Fatalf("pending=%d", doc.PendingGeneration)
	}
}

func TestC2B_CrashRecovery(t *testing.T) {
	ref := c2bRef(t, "c2b-crash")
	advance := installFakeClock(t, time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	res, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res.Reserved {
		t.Fatal(err)
	}
	gen0, ok, err := hookactions.BeginMaintainCapture(ref.Home, ref.ProjectID, res.Token, res.Fence)
	if err != nil || !ok || gen0 != 1 {
		t.Fatalf("begin: gen0=%d ok=%v err=%v", gen0, ok, err)
	}
	// Simulate crash: leave running_maintain, expire lease, no commit.
	advance(hookactions.OwnerLeaseTTL + time.Second)

	var spawned atomic.Bool
	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		spawned.Store(true)
		if fence < 2 {
			t.Errorf("expected reclaim fence>=2 got %d", fence)
		}
		return nil
	})
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if !spawned.Load() {
		t.Fatal("expected recovery spawn")
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.ProcessedGeneration != 0 {
		t.Fatal("crash must not advance processed")
	}
}

func TestC2B_DebounceWithoutFurtherEvents(t *testing.T) {
	ref := c2bRef(t, "c2b-debounce")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 15, 30, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	var ran atomic.Bool
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		ran.Store(true)
		return hookactions.MaintainOutcomeSkipped, hookactions.MaintainOutcomeUnavailable, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if !ran.Load() {
		t.Fatal("scheduler must run maintain after debounce without further events")
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration != doc.ProcessedGeneration {
		t.Fatalf("expected clean pending=%d processed=%d", doc.PendingGeneration, doc.ProcessedGeneration)
	}
	if doc.OwnerToken != "" {
		t.Fatal("owner should be released when clean")
	}
}

func TestC2B_MaxCoalesceWindow(t *testing.T) {
	ref := c2bRef(t, "c2b-coalesce")
	advance := installFakeClock(t, time.Date(2026, 10, 19, 16, 0, 0, 0, time.UTC))
	start := time.Date(2026, 10, 19, 16, 0, 0, 0, time.UTC)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	// Continuous bumps every 1s — due_at must not exceed burst+MaxCoalesceWindow.
	for i := 0; i < 40; i++ {
		advance(time.Second)
		_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	due, err := time.Parse(time.RFC3339Nano, doc.DueAt)
	if err != nil {
		due, err = time.Parse(time.RFC3339, doc.DueAt)
		if err != nil {
			t.Fatalf("parse due_at: %v", err)
		}
	}
	maxDue := start.Add(hookactions.MaxCoalesceWindow)
	if due.After(maxDue) {
		t.Fatalf("due_at %v exceeds coalesce max %v", due, maxDue)
	}
}

func TestC2B_AbsoluteProcessDeadline(t *testing.T) {
	ref := c2bRef(t, "c2b-deadline")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 16, 30, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	hookactions.SetAbsoluteProcessDeadlineForTest(2 * time.Second)
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		return hookactions.MaintainOutcomeFailed, hookactions.MaintainOutcomeFailed, hookactions.MaintainErrCEFailed
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	_ = hookactions.EnsurePostCodeSchedulerIfDirty(ref)

	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration <= doc.ProcessedGeneration {
		t.Fatal("deadline exit must leave work recoverable")
	}
	if doc.OwnerToken != "" {
		t.Fatalf("graceful deadline exit must release ownership, token=%q", doc.OwnerToken)
	}
	if doc.ProcessedGeneration != 0 {
		t.Fatalf("deadline must not advance processed: %d", doc.ProcessedGeneration)
	}
	// EnsureIfDirty can reclaim immediately (no lease wait).
	var respawned atomic.Bool
	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		respawned.Store(true)
		return nil
	})
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if !respawned.Load() {
		t.Fatal("expected EnsureIfDirty recovery spawn after graceful release")
	}
}

func TestC2B_RenewalDuringMaintain(t *testing.T) {
	ref := c2bRef(t, "c2b-renew")
	advance := installFakeClock(t, time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)

	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		doc, _ := hookactions.LoadMaintainPending(r.Home, r.ProjectID)
		token := doc.OwnerToken
		fence := doc.OwnerFence
		lease1 := doc.OwnerLeaseUntil
		// Advance close to expiry then heartbeat (simulates in-maintain renewal).
		advance(hookactions.OwnerLeaseTTL - time.Second)
		ok, err := hookactions.HeartbeatOwner(r.Home, r.ProjectID, token, fence, "running_maintain")
		if err != nil || !ok {
			t.Errorf("heartbeat during maintain: %v %v", ok, err)
		}
		doc2, _ := hookactions.LoadMaintainPending(r.Home, r.ProjectID)
		if doc2.OwnerLeaseUntil == lease1 {
			t.Error("lease was not renewed")
		}
		advance(2 * time.Second)
		if !hookactions.OwnerVigenteForTest(doc2, time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC).Add(hookactions.OwnerLeaseTTL-time.Second+2*time.Second)) {
			doc2, _ = hookactions.LoadMaintainPending(r.Home, r.ProjectID)
			if !hookactions.OwnerVigenteForTest(doc2, time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC).Add(hookactions.OwnerLeaseTTL-time.Second+2*time.Second)) {
				t.Error("owner lost during maintain despite heartbeat")
			}
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.ProcessedGeneration != 1 {
		t.Fatalf("processed=%d after renewed maintain", doc.ProcessedGeneration)
	}
}

func TestC2B_PhaseBCompatibleSessionFenceUntouched(t *testing.T) {
	ref := c2bRef(t, "c2b-phaseb")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(hookactions.ResetAdmitStateForTest)

	var sessionSpawns atomic.Int32
	hookactions.SetSpawner(func(r hookactions.ProjectRef) error {
		sessionSpawns.Add(1)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSpawner(nil) })

	// Dirty pending + session admit must not use session fence for post-code.
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	var schedSpawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		schedSpawns.Add(1)
		return nil // do not run inline — only observe recovery spawn from defer
	})

	ev := hooks.Event{
		ID: "e-c2b-b", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Adapter: adapters.Cursor,
		Payload: hooks.Payload{SessionID: "sess-c2b-1", ProjectID: ref.ProjectID},
	}
	res, err := hookactions.AdmitSessionMaintenance(context.Background(), ev, fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit outcome=%s", res.Outcome)
	}
	if sessionSpawns.Load() != 1 {
		t.Fatalf("session spawns=%d", sessionSpawns.Load())
	}
	if schedSpawns.Load() != 1 {
		t.Fatalf("scheduler recovery spawns=%d want 1", schedSpawns.Load())
	}

	// Session fence admitted; second admit skips — Phase B unchanged.
	res2, err := hookactions.AdmitSessionMaintenance(context.Background(), ev, fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res2.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("second admit=%s want skipped", res2.Outcome)
	}
	if sessionSpawns.Load() != 1 {
		t.Fatalf("session must not re-spawn, got %d", sessionSpawns.Load())
	}

	// Post-code pending still independent.
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration < 1 {
		t.Fatal("pending lost")
	}
	st, ok, err := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-c2b-1")
	if err != nil || !ok || st != "admitted" {
		t.Fatalf("session fence: state=%q ok=%v err=%v", st, ok, err)
	}
}

func TestC2B_PendingFilePermissionsAndNoSecrets(t *testing.T) {
	ref := c2bRef(t, "c2b-perms")
	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("pending perms too open: %v", info.Mode())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, bad := range []string{"tool_input", "password", "secret", "-----begin"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("sensitive content in pending: %s", raw)
		}
	}
}

func TestC2B_CorruptPendingFailClosed(t *testing.T) {
	ref := c2bRef(t, "c2b-corrupt")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("schema_version: 1\n\x00BROKEN")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if !hookactions.ErrPendingCorrupt(err) {
		t.Fatalf("want ErrPendingCorrupt, got %v", err)
	}
	// Evidence must remain untouched.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("corrupt evidence overwritten")
	}
	// Mutators must not invent work or overwrite.
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); !hookactions.ErrPendingCorrupt(err) {
		t.Fatalf("bump over corrupt: %v", err)
	}
	if _, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID); !hookactions.ErrPendingCorrupt(err) {
		t.Fatalf("reserve over corrupt: %v", err)
	}
}

func TestC2B_IncompatibleSchemaFailClosed(t *testing.T) {
	ref := c2bRef(t, "c2b-schema")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("schema_version: 99\npending_generation: 7\nprocessed_generation: 0\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if !hookactions.ErrPendingCorrupt(err) {
		t.Fatalf("want corrupt/incompatible, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(raw) {
		t.Fatal("incompatible schema file overwritten")
	}
}

func TestC2B_CorruptPendingControlledRecovery(t *testing.T) {
	ref := c2bRef(t, "c2b-qreset")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("not: valid: yaml: [[")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	ev, err := hookactions.ListMaintainPendingCorruptEvidence(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 1 {
		t.Fatalf("evidence files=%d want 1", len(ev))
	}
	qraw, err := os.ReadFile(ev[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(qraw) != string(raw) {
		t.Fatal("quarantine missing original evidence")
	}
	info, err := os.Stat(ev[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("quarantine perms too open: %v", info.Mode())
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 {
		t.Fatalf("recovery must be dirty epoch pending=1 processed=0, got %+v", doc)
	}
	if doc.OwnerToken != "" {
		t.Fatal("recovery must clear ownership")
	}
}

func TestC2B_FIX04_TwoRecoveriesPreserveDistinctEvidence(t *testing.T) {
	ref := c2bRef(t, "c2b-q2")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw1 := []byte("corrupt-one: [[[")
	if err := os.WriteFile(path, raw1, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	// Force primary corrupt again after dirty reset.
	raw2 := []byte("corrupt-two: {{{")
	if err := os.WriteFile(path, raw2, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	ev, err := hookactions.ListMaintainPendingCorruptEvidence(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 2 {
		t.Fatalf("evidence=%d want 2 (no overwrite)", len(ev))
	}
	seen := map[string]bool{}
	for _, p := range ev {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		seen[string(b)] = true
	}
	if !seen[string(raw1)] || !seen[string(raw2)] {
		t.Fatalf("missing distinct evidence bodies: %v", seen)
	}
}

func TestC2B_FIX04_QuarantineFailureLeavesOriginal(t *testing.T) {
	ref := c2bRef(t, "c2b-qfail")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("still-corrupt: [[")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	hookactions.SetCorruptEvidenceWriterForTest(func(homePath, projectID string, data []byte) (string, error) {
		return "", errors.New("injected quarantine failure")
	})
	t.Cleanup(func() { hookactions.SetCorruptEvidenceWriterForTest(nil) })

	err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID)
	if err == nil {
		t.Fatal("expected quarantine failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatal("primary must be unchanged when quarantine fails")
	}
	if _, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID); !hookactions.ErrPendingCorrupt(err) {
		t.Fatalf("primary must remain corrupt, got %v", err)
	}
	ev, _ := hookactions.ListMaintainPendingCorruptEvidence(ref.Home, ref.ProjectID)
	if len(ev) != 0 {
		t.Fatalf("no evidence should be written on failure, got %d", len(ev))
	}
}

func TestC2B_FIX04_RecoveryDirtyEnsureAndCECI(t *testing.T) {
	ref := c2bRef(t, "c2b-qdirty")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bad: [["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration <= doc.ProcessedGeneration {
		t.Fatal("recovery must leave dirty state")
	}

	var admitted atomic.Bool
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		admitted.Store(true)
		return hookactions.InlineSchedulerSpawner(r, token, fence)
	})
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if !admitted.Load() {
		t.Fatal("EnsureIfDirty must admit scheduler after recovery")
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ProcessedGeneration != 1 || doc.PendingGeneration != 1 {
		t.Fatalf("successful CE/CI must credit new epoch: %+v", doc)
	}
}

func TestC2B_FIX04_RecoveryCECIFailureKeepsDirty(t *testing.T) {
	ref := c2bRef(t, "c2b-qfailci")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 20, 10, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte("bad: [["), 0o600)
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	hookactions.SetAbsoluteProcessDeadlineForTest(2 * time.Second)
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		return hookactions.MaintainOutcomeFailed, hookactions.MaintainOutcomeFailed, hookactions.MaintainErrCEFailed
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	_ = hookactions.EnsurePostCodeSchedulerIfDirty(ref)
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ProcessedGeneration != 0 || doc.PendingGeneration != 1 {
		t.Fatalf("failed CE/CI must keep dirty: %+v", doc)
	}
}

func TestC2B_FIX04_ResetFailureKeepsCorruptPrimary(t *testing.T) {
	ref := c2bRef(t, "c2b-qcrash")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("mid-crash: [[")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	hookactions.SetRecoveryPendingWriterForTest(func(homePath, rel string, data []byte) error {
		return errors.New("injected reset failure")
	})
	t.Cleanup(func() { hookactions.SetRecoveryPendingWriterForTest(nil) })

	err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID)
	if err == nil {
		t.Fatal("expected reset failure")
	}
	// Evidence preserved, primary still corrupt — not a false clean state.
	ev, err := hookactions.ListMaintainPendingCorruptEvidence(ref.Home, ref.ProjectID)
	if err != nil || len(ev) != 1 {
		t.Fatalf("evidence=%v err=%v", ev, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatal("primary must remain corrupt after reset failure")
	}
	if _, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID); !hookactions.ErrPendingCorrupt(err) {
		t.Fatalf("load must still report corrupt, got %v", err)
	}
}

func TestC2B_FIX04_ConcurrentRecoverySerialized(t *testing.T) {
	ref := c2bRef(t, "c2b-qconc")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("conc: [["), 0o600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent recovery: %v", err)
		}
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 {
		t.Fatalf("want dirty epoch, got %+v", doc)
	}
	ev, err := hookactions.ListMaintainPendingCorruptEvidence(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly one quarantine: first wins; others see valid dirty and no-op.
	if len(ev) != 1 {
		t.Fatalf("evidence=%d want 1", len(ev))
	}
}

func TestC2B_FIX04_IncompatibleSchemaRequiresExplicitRecovery(t *testing.T) {
	ref := c2bRef(t, "c2b-schema-rec")
	path := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("schema_version: 99\npending_generation: 7\nprocessed_generation: 3\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID); !hookactions.ErrPendingCorrupt(err) {
		t.Fatalf("want corrupt, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(raw) {
		t.Fatal("silent overwrite of incompatible schema")
	}
	if err := hookactions.QuarantineAndResetCorruptPending(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	ev, err := hookactions.ListMaintainPendingCorruptEvidence(ref.Home, ref.ProjectID)
	if err != nil || len(ev) != 1 {
		t.Fatalf("evidence=%v err=%v", ev, err)
	}
	qraw, _ := os.ReadFile(ev[0])
	if string(qraw) != string(raw) {
		t.Fatal("schema evidence not preserved")
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 {
		t.Fatalf("explicit recovery dirty epoch: %+v", doc)
	}
	// Must not reconstruct historical gens 7/3.
	if doc.PendingGeneration == 7 || doc.ProcessedGeneration == 3 {
		t.Fatal("must not invent historical generations")
	}
}

func TestC2B_AbsentPendingIsEmptyNotCorrupt(t *testing.T) {
	ref := c2bRef(t, "c2b-absent")
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 0 {
		t.Fatalf("absent should be empty queue, got %+v", doc)
	}
}

func TestC2B_StaleResultFencedAfterReclaim(t *testing.T) {
	ref := c2bRef(t, "c2b-stale-result")
	advance := installFakeClock(t, time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	// Disable auto-heartbeat so reclaim is not raced by a renewing S1 heartbeat.
	hookactions.SetOwnerHeartbeatIntervalForTest(-1)

	_, _ = hookactions.BumpPendingEdit(ref.Home, ref.ProjectID)
	res1, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res1.Reserved {
		t.Fatalf("S1 reserve: %+v err=%v", res1, err)
	}

	entered := make(chan struct{})
	releaseMaintain := make(chan struct{})
	var sawCancel atomic.Bool

	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		close(entered)
		select {
		case <-ctx.Done():
			sawCancel.Store(true)
		case <-time.After(3 * time.Second):
			t.Error("S1 maintain did not observe cancellation")
		}
		<-releaseMaintain
		// Ignore cancellation: report operational success (stale S1).
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = hookactions.RunPostCodeMaintainCycleForTest(ctx, ref, res1.Token, res1.Fence)
	}()

	<-entered

	// No concurrent CE/CI: maintain.lock held by S1.
	if _, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID); !hookactions.IsMaintainBusyForTest(err) {
		t.Fatalf("expected maintain.lock busy during S1 CE/CI, got %v", err)
	}

	// Reclaim as S2 with higher fence (no competing heartbeat renew).
	advance(hookactions.OwnerLeaseTTL + time.Second)
	res2, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res2.Reserved {
		t.Fatalf("S2 reserve: %+v err=%v", res2, err)
	}
	if res2.Fence <= res1.Fence {
		t.Fatalf("S2 fence=%d must exceed S1 fence=%d", res2.Fence, res1.Fence)
	}

	// S1 receives cancellation while still inside CE/CI.
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for !sawCancel.Load() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for S1 cancel")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(releaseMaintain)
	<-done

	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ProcessedGeneration != 0 {
		t.Fatalf("S1 must not credit generations: processed=%d", doc.ProcessedGeneration)
	}
	if doc.PendingGeneration != 1 {
		t.Fatalf("pending work must remain: pending=%d", doc.PendingGeneration)
	}
	if doc.OwnerToken != res2.Token || doc.OwnerFence != res2.Fence {
		t.Fatalf("S2 ownership corrupted: token=%q fence=%d want %q/%d", doc.OwnerToken, doc.OwnerFence, res2.Token, res2.Fence)
	}
	cleared, err := hookactions.ClearOwnReservation(ref.Home, ref.ProjectID, res1.Token, res1.Fence)
	if err != nil {
		t.Fatal(err)
	}
	if cleared {
		t.Fatal("stale S1 must not clear S2 ownership")
	}
}

func TestC2B_NoProductionToolTriggerRegistered(t *testing.T) {
	// CE/CI must never subscribe to tool.after_execution (post-code is separate).
	for _, a := range []hooks.Action{&hookactions.ContextEconomyAction{}, &hookactions.CodeIntelAction{}} {
		for _, ev := range a.Events() {
			if ev == hooks.EventAfterToolExecution {
				t.Fatalf("%s must not subscribe to tool.after_execution", a.ID())
			}
		}
	}
}
