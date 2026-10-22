package hookactions_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
)

// TestQA04_Stage2B_LockAndGeneration is the deterministic Phase C contention
// proof. The maintain body blocks on a channel after runPostCodeMaintainCycle
// has taken the real maintain.lock. Debounce uses the existing fake clock.
// Flock exclusion and owner fencing are asserted separately.
//
// In-cycle stale success (a blocked body that returns ok after its fence is
// stolen) remains TestC2B_StaleResultFencedAfterReclaim. Subtest StaleCommit
// checks the commit rule against a generation admitted after capture.
func TestQA04_Stage2B_LockAndGeneration(t *testing.T) {
	t.Run("HoldAndDrain", testQA04Stage2BHoldAndDrain)
	t.Run("StaleCommit", testQA04Stage2BStaleCommit)
}

func testQA04Stage2BHoldAndDrain(t *testing.T) {
	ref := c2bRef(t, "qa04-s2b-hold")
	advance := installFakeClock(t, time.Date(2026, 10, 22, 15, 0, 0, 0, time.UTC))
	// pending.lock retries sleep 5ms. Under the shared fake clock those sleeps
	// consume the 2s retry budget immediately when many goroutines collide.
	// Yield instead, and advance the clock only for scheduler debounce slices.
	hookactions.SetPendingSleepForTest(func(d time.Duration) {
		if d <= 5*time.Millisecond {
			runtime.Gosched()
			return
		}
		advance(d)
	})
	hookactions.SetOwnerHeartbeatIntervalForTest(-1)
	hookactions.SetTriggersEnabled(true)

	const extraBumps = 20

	releaseCh := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	entered := make(chan struct{})
	var enteredOnce sync.Once

	var calls atomic.Int32
	var gen0 atomic.Int64
	var processedAtSecond atomic.Int64
	var pendingAtSecond atomic.Int64
	var sawSecond atomic.Bool
	var processedExceeds atomic.Bool

	hookactions.SetPostCodeMaintainForTest(func(_ context.Context, r hookactions.ProjectRef) (string, string, string) {
		n := calls.Add(1)
		doc, err := hookactions.LoadMaintainPending(r.Home, r.ProjectID)
		if err != nil {
			t.Errorf("load pending in body: %v", err)
			return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
		}
		if doc.ProcessedGeneration > doc.PendingGeneration {
			processedExceeds.Store(true)
			t.Errorf("processed %d exceeds pending %d during body call %d", doc.ProcessedGeneration, doc.PendingGeneration, n)
		}
		if n == 1 {
			gen0.Store(doc.PendingGeneration)
			enteredOnce.Do(func() { close(entered) })
			<-releaseCh
			return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
		}
		if n == 2 {
			processedAtSecond.Store(doc.ProcessedGeneration)
			pendingAtSecond.Store(doc.PendingGeneration)
			sawSecond.Store(true)
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})

	var spawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		spawns.Add(1)
		return hookactions.AsyncInlineSchedulerSpawner(r, token, fence)
	})

	// Wait is registered first so cleanup runs release, then join.
	t.Cleanup(func() { hookactions.WaitAsyncSchedulerForTest() })
	t.Cleanup(release)

	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	<-entered

	if calls.Load() != 1 {
		t.Fatalf("active maintain bodies=%d want 1", calls.Load())
	}
	captured := gen0.Load()
	if captured != 1 {
		t.Fatalf("captured generation=%d want 1", captured)
	}

	// A. Second flock acquire while the real cycle holds maintain.lock.
	if _, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID); !hookactions.IsMaintainBusyForTest(err) {
		if err == nil {
			t.Fatal("second TryAcquireMaintainLock succeeded while the cycle holds maintain.lock")
		}
		t.Fatalf("second TryAcquireMaintainLock=%v want maintain busy", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("second acquire started another body, calls=%d", calls.Load())
	}

	held, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if held.OwnerToken == "" || held.OwnerFence == 0 {
		t.Fatalf("owner missing during hold: token=%q fence=%d", held.OwnerToken, held.OwnerFence)
	}
	ownerToken, ownerFence := held.OwnerToken, held.OwnerFence

	// B. Real pending admission while the flock is still held.
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	afterB, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if afterB.PendingGeneration != captured+1 {
		t.Fatalf("pending=%d want %d", afterB.PendingGeneration, captured+1)
	}
	if afterB.ProcessedGeneration >= afterB.PendingGeneration {
		t.Fatalf("additional generation already processed: pending=%d processed=%d", afterB.PendingGeneration, afterB.ProcessedGeneration)
	}
	if afterB.OwnerToken != ownerToken || afterB.OwnerFence != ownerFence {
		t.Fatalf("admission changed owner token=%q fence=%d", afterB.OwnerToken, afterB.OwnerFence)
	}
	if _, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID); !hookactions.IsMaintainBusyForTest(err) {
		t.Fatalf("flock released during admission, err=%v", err)
	}

	// C. Owner reservation is a separate refusal from the flock.
	res, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reserved {
		t.Fatal("ReserveOwner reserved a second owner while the first lease is live")
	}
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if spawns.Load() != 1 {
		t.Fatalf("scheduler spawns=%d want 1", spawns.Load())
	}
	if _, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID); !hookactions.IsMaintainBusyForTest(err) {
		t.Fatalf("reservation check lost the flock, err=%v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("reservation check started another body, calls=%d", calls.Load())
	}

	// F. Concurrent admissions during the same held cycle.
	var bumpWG sync.WaitGroup
	errCh := make(chan error, extraBumps)
	for i := 0; i < extraBumps; i++ {
		bumpWG.Add(1)
		go func() {
			defer bumpWG.Done()
			if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
				errCh <- err
			}
		}()
	}
	bumpWG.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent bump: %v", err)
	}
	if t.Failed() {
		release()
		return
	}

	afterF, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	wantPending := captured + 1 + extraBumps
	if afterF.PendingGeneration != wantPending {
		t.Fatalf("pending=%d want %d", afterF.PendingGeneration, wantPending)
	}
	if afterF.ProcessedGeneration != 0 {
		t.Fatalf("processed=%d want 0 while the first cycle is still inside its body", afterF.ProcessedGeneration)
	}
	if afterF.ProcessedGeneration > afterF.PendingGeneration {
		t.Fatalf("processed %d exceeds pending %d", afterF.ProcessedGeneration, afterF.PendingGeneration)
	}
	if afterF.OwnerToken != ownerToken || afterF.OwnerFence != ownerFence {
		t.Fatalf("concurrent bumps changed owner")
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent bumps overlapped a second body, calls=%d", calls.Load())
	}
	if _, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID); !hookactions.IsMaintainBusyForTest(err) {
		t.Fatalf("flock not held after concurrent bumps, err=%v", err)
	}

	// D. Release the body. The first commit may accredit only the captured generation.
	release()
	hookactions.WaitAsyncSchedulerForTest()

	if !sawSecond.Load() {
		t.Fatal("scheduler did not run a second cycle after release")
	}
	if processedAtSecond.Load() != captured {
		t.Fatalf("first commit processed=%d want captured %d", processedAtSecond.Load(), captured)
	}
	if pendingAtSecond.Load() != wantPending {
		t.Fatalf("pending at second cycle=%d want %d", pendingAtSecond.Load(), wantPending)
	}
	if processedExceeds.Load() {
		t.Fatal("processed exceeded pending inside a maintain body")
	}
	if calls.Load() < 2 {
		t.Fatalf("maintain bodies=%d want at least 2", calls.Load())
	}

	final, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if final.PendingGeneration != final.ProcessedGeneration {
		t.Fatalf("pending=%d processed=%d", final.PendingGeneration, final.ProcessedGeneration)
	}
	if final.ProcessedGeneration != wantPending {
		t.Fatalf("processed=%d want %d", final.ProcessedGeneration, wantPending)
	}
	if final.OwnerToken != "" || final.OwnerState != "none" {
		t.Fatalf("owner still live after drain: token=%q state=%q", final.OwnerToken, final.OwnerState)
	}
	if final.OwnerFence == 0 {
		t.Fatal("owner fence was cleared")
	}
	if spawns.Load() != 1 {
		t.Fatalf("drain spawned another scheduler, spawns=%d", spawns.Load())
	}
}

func testQA04Stage2BStaleCommit(t *testing.T) {
	// The blocked-body form of this rule is TestC2B_StaleResultFencedAfterReclaim.
	ref := c2bRef(t, "qa04-s2b-stale")
	advance := installFakeClock(t, time.Date(2026, 10, 22, 16, 0, 0, 0, time.UTC))
	hookactions.SetOwnerHeartbeatIntervalForTest(-1)

	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}
	res1, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res1.Reserved {
		t.Fatalf("reserve S1: %+v err=%v", res1, err)
	}
	gen0, ok, err := hookactions.BeginMaintainCapture(ref.Home, ref.ProjectID, res1.Token, res1.Fence)
	if err != nil || !ok || gen0 != 1 {
		t.Fatalf("capture gen0=%d ok=%v err=%v", gen0, ok, err)
	}
	// Generation admitted after S1's capture. S1 must not be able to accredit it.
	if _, err := hookactions.BumpPendingEdit(ref.Home, ref.ProjectID); err != nil {
		t.Fatal(err)
	}

	advance(hookactions.OwnerLeaseTTL + time.Second)
	res2, err := hookactions.ReserveOwner(ref.Home, ref.ProjectID)
	if err != nil || !res2.Reserved {
		t.Fatalf("reserve S2: %+v err=%v", res2, err)
	}
	if res2.Fence <= res1.Fence {
		t.Fatalf("S2 fence=%d want > %d", res2.Fence, res1.Fence)
	}

	for _, gen := range []int64{gen0, gen0 + 1} {
		out, err := hookactions.CommitGeneration(ref.Home, ref.ProjectID, res1.Token, res1.Fence, gen, true, hookactions.MaintainErrNone)
		if err != nil {
			t.Fatal(err)
		}
		if out.Applied {
			t.Fatalf("stale commit of generation %d was applied", gen)
		}
	}
	cleared, err := hookactions.ClearOwnReservation(ref.Home, ref.ProjectID, res1.Token, res1.Fence)
	if err != nil {
		t.Fatal(err)
	}
	if cleared {
		t.Fatal("stale S1 cleared S2")
	}

	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ProcessedGeneration != 0 {
		t.Fatalf("processed=%d want 0", doc.ProcessedGeneration)
	}
	if doc.PendingGeneration != 2 {
		t.Fatalf("pending=%d want 2", doc.PendingGeneration)
	}
	if doc.OwnerToken != res2.Token || doc.OwnerFence != res2.Fence {
		t.Fatalf("S2 ownership lost: token=%q fence=%d", doc.OwnerToken, doc.OwnerFence)
	}
}
