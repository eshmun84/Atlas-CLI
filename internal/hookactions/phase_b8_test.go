package hookactions_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

func TestPhaseB8_SessionStartMaintains(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-start")
	resp := dispatchCursorSession(t, ref, hooks.EventSessionStart, "b8-s1", "sess-a", "")
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("%#v", resp)
	}
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d", spawned.Load())
	}
	st, ok, err := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-a")
	if err != nil || !ok || st != "admitted" {
		t.Fatalf("session state=%q ok=%v err=%v", st, ok, err)
	}
}

func TestPhaseB8_SessionStartThenPromptSingleMaintain(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-start-prompt")
	_ = dispatchCursorSession(t, ref, hooks.EventSessionStart, "b8-sp-1", "sess-b", "")
	_ = dispatchCursorTask(t, ref, "b8-sp-2", "sess-b", "gen-1")
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
}

func TestPhaseB8_PromptOnlyMaintains(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "b8-prompt-only")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	resp := dispatchCursorTask(t, ref, "b8-po-1", "sess-c", "gen-1")
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("%#v", resp)
	}
	// Actions do not subscribe to task.start — count stays 0; fallback admits.
	if resp.Count != 0 {
		t.Fatalf("task.start must not invoke maintain Actions: count=%d", resp.Count)
	}
	hookactions.WaitAsyncInlineForTest()
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
}

func TestPhaseB8_TwoPromptsSameSessionOneMaintain(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-two-prompt")
	_ = dispatchCursorTask(t, ref, "b8-tp-1", "sess-d", "gen-1")
	_ = dispatchCursorTask(t, ref, "b8-tp-2", "sess-d", "gen-2")
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
}

func TestPhaseB8_TenPromptsSameSessionOneMaintain(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-ten")
	for i := 0; i < 10; i++ {
		_ = dispatchCursorTask(t, ref, "b8-ten-"+itoa(i), "sess-e", "gen-"+itoa(i))
	}
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
}

func TestPhaseB8_NewSessionAllowsAdmit(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		// Run worker so project lease clears; session fence must not block a
		// different conversation id.
		return hookactions.InlineSpawner(ref)
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-new-sess")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	_ = dispatchCursorTask(t, ref, "b8-ns-1", "sess-f1", "g1")
	_ = dispatchCursorTask(t, ref, "b8-ns-2", "sess-f2", "g2")
	if spawned.Load() != 2 {
		t.Fatalf("spawned=%d want 2", spawned.Load())
	}
}

func TestPhaseB8_EmptySessionIDFailOpen(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-empty-sid")
	resp := dispatchCursorTask(t, ref, "b8-es-1", "", "gen-1")
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("must fail-open: %#v", resp)
	}
	if spawned.Load() != 0 {
		t.Fatal("empty session must not admit")
	}
}

func TestPhaseB8_InvalidSessionIDFailOpen(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-bad-sid")
	for _, sid := range []string{"../evil", "a/b", `a\b`, "."} {
		res, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
			ID: "bad-" + sid, Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
			Payload: hooks.Payload{
				ProjectID: ref.ProjectID, SessionID: sid, TaskID: "t1",
				TaskSummaryState: hooks.TaskSummaryAbsent,
			},
		}, fixedResolve(ref))
		if err != nil || res.Outcome != hooks.OutcomeSkipped {
			t.Fatalf("sid=%q outcome=%s err=%v", sid, res.Outcome, err)
		}
	}
	if spawned.Load() != 0 {
		t.Fatal("invalid session must not admit")
	}
	if !hookactions.ValidSessionID("conv-ok-1") {
		t.Fatal("expected valid id")
	}
}

func TestPhaseB8_ConcurrentSameSession(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	var work sync.WaitGroup
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		work.Add(1)
		go func() {
			defer work.Done()
			_ = hookactions.InlineSpawner(ref)
		}()
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		work.Wait()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "b8-conc")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = dispatchCursorTask(t, ref, "b8-c-"+itoa(i), "sess-conc", "g-"+itoa(i))
		}(i)
	}
	wg.Wait()
	work.Wait()
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
}

func TestPhaseB8_SessionStartAndPromptConcurrent(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-race")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = dispatchCursorSession(t, ref, hooks.EventSessionStart, "b8-race-s", "sess-race", "")
	}()
	go func() {
		defer wg.Done()
		_ = dispatchCursorTask(t, ref, "b8-race-p", "sess-race", "gen-race")
	}()
	wg.Wait()
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
}

func TestPhaseB8_LockBusySkips(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-busy")
	lock, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hookactions.ReleaseMaintainLock(lock) })

	res, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "busy-prompt", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-busy", TaskID: "t",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("outcome=%s err=%v", res.Outcome, err)
	}
	if spawned.Load() != 0 {
		t.Fatal("busy lock must not spawn")
	}
}

func TestPhaseB8_ActiveLeaseSkips(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-lease")
	// First admit writes lease+session; second must skip.
	res1, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "lease-1", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-lease", TaskID: "t1",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res1.Outcome != hooks.OutcomeOK {
		t.Fatalf("first %#v err=%v", res1, err)
	}
	hookactions.ResetAdmitStateForTest()
	res2, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "lease-2", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-lease", TaskID: "t2",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res2.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("second %#v err=%v", res2, err)
	}
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d", spawned.Load())
	}
}

func TestPhaseB8_SpawnFailureAllowsRetry(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return errors.New("spawn boom")
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-spawn-fail")
	res, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "sf-1", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-sf", TaskID: "t1",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err == nil || res.Outcome != hooks.OutcomeFailed {
		t.Fatalf("want failed spawn: %#v err=%v", res, err)
	}
	if hookactions.ClassifyAdmitError(err) != hookactions.AdmitClassSpawnFail {
		t.Fatalf("class=%s", hookactions.ClassifyAdmitError(err))
	}
	st, ok, _ := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-sf")
	if !ok || st != "retryable" {
		t.Fatalf("spawn failure must leave an observable retryable fence, state=%q ok=%v", st, ok)
	}

	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	res2, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "sf-2", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-sf", TaskID: "t2",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res2.Outcome != hooks.OutcomeOK {
		t.Fatalf("retry %#v err=%v", res2, err)
	}
	if spawned.Load() != 2 {
		t.Fatalf("spawned=%d want 2", spawned.Load())
	}
}

func TestPhaseB8_WorkerOperationalFailureMarksComplete(t *testing.T) {
	// CE/CI operational failures still mark the session complete so prompts
	// do not re-spawn forever. Retry is for timeout/cancel/incomplete/spawn.
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-wfail")
	ctxDir := filepath.Join(ref.Home, "projects", ref.ProjectID, "context")
	if err := os.MkdirAll(ctxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	ext := filepath.Join(outside, "index.yaml")
	write(t, ext, "schema_version: 1\n")
	if err := os.Symlink(ext, filepath.Join(ctxDir, "index.yaml")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ctxDir, "capsule.md"), "# c\n")

	var spawned atomic.Int32
	hookactions.SetSpawner(func(r hookactions.ProjectRef) error {
		spawned.Add(1)
		return hookactions.InlineSpawner(r)
	})

	res, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "wf-1", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-wf", TaskID: "t1",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit %#v err=%v", res, err)
	}
	rec, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok {
		t.Fatalf("maintain-last ok=%v err=%v", ok, err)
	}
	if rec.ErrorCode == hookactions.MaintainErrNone {
		t.Fatalf("expected worker error, got %#v", rec)
	}
	st, sok, _ := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-wf")
	if !sok || st != "complete" {
		t.Fatalf("operational failure must mark session complete, got state=%q ok=%v", st, sok)
	}

	hookactions.ResetAdmitStateForTest()
	res2, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "wf-2", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-wf", TaskID: "t2",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res2.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("second prompt must skip: %#v err=%v", res2, err)
	}
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
}

func TestPhaseB8_TimeoutClearsSessionForRetry(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-timeout")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	var spawned atomic.Int32
	var timeoutAdmissionID string
	hookactions.SetSpawner(func(r hookactions.ProjectRef) error {
		spawned.Add(1)
		timeoutAdmissionID = r.AdmissionID
		// Simulate worker timeout finalize path (no full CE/CI work).
		_, started := hookactions.BeginMaintainLastForTest(r.Home, r.ProjectID, "sess-to", r.AdmissionID)
		hookactions.FinishMaintainLastForTest(r.Home, r.ProjectID, "sess-to", r.AdmissionID, started,
			hookactions.MaintainOutcomeTimeout, hookactions.MaintainOutcomePending, hookactions.MaintainErrTimeout)
		return nil
	})

	res, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "to-1", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-to", TaskID: "t1",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("admit %#v err=%v", res, err)
	}
	st, ok, _ := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-to")
	if !ok || st != "retryable" {
		t.Fatalf("timeout must leave a retryable session fence, state=%q ok=%v", st, ok)
	}
	hookactions.ClearMaintainLeaseForTest(ref.Home, ref.ProjectID, timeoutAdmissionID)

	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	res2, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "to-2", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-to", TaskID: "t2",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil || res2.Outcome != hooks.OutcomeOK {
		t.Fatalf("retry after timeout %#v err=%v", res2, err)
	}
	if spawned.Load() != 2 {
		t.Fatalf("spawned=%d want 2", spawned.Load())
	}
}

func TestPhaseB8_WorkerCompleteMarksSession(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "b8-complete")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	_ = dispatchCursorTask(t, ref, "b8-comp-1", "sess-comp", "g1")
	hookactions.WaitAsyncInlineForTest()
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
	st, ok, err := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-comp")
	if err != nil || !ok || st != "complete" {
		t.Fatalf("state=%q ok=%v err=%v", st, ok, err)
	}
}

func TestPhaseB8_IncompleteClearsSession(t *testing.T) {
	ref := seedInitializedProject(t, "b8-incomplete")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	// Simulate prior admitted session + interrupted worker record.
	if err := os.MkdirAll(filepath.Dir(hookactions.SessionMaintainPath(ref.Home, ref.ProjectID)), 0o700); err != nil {
		t.Fatal(err)
	}
	// Admit once so session fence exists, then simulate crash via maintain-last running.
	hookactions.SetTriggersEnabled(true)
	var admitted hookactions.ProjectRef
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		admitted = ref
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	_, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "inc-1", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-inc", TaskID: "t1",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	if err != nil {
		t.Fatal(err)
	}
	// Write a running maintain-last with session id (crash mid-flight).
	path := hookactions.MaintainLastPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, path, "schema_version: 1\nstatus: running\nstarted_at: "+time.Now().UTC().Format(time.RFC3339Nano)+"\nduration_ms: 0\nce_outcome: pending\nci_outcome: pending\nerror_code: none\nsession_id: sess-inc\nadmission_id: "+admitted.AdmissionID+"\npid: 1\n")

	// Next worker start marks incomplete and clears session fence.
	_ = hookactions.InlineSpawner(admitted)
	st, ok, _ := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-inc")
	if !ok || st != "complete" {
		t.Fatalf("recovered worker must complete its owned session, state=%q ok=%v", st, ok)
	}
}

func TestPhaseB8_CursorOnlyOpenCodeUnaffected(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-oc")
	cap := sessionCapableFor(adapters.OpenCode, hooks.EventTaskStart)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.OpenCode),
		Event: hooksengine.EventInput{
			ID: "oc-task", Type: string(hooks.EventTaskStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID, SessionID: "sess-oc", TaskID: "t1",
				TaskSummaryState: string(hooks.TaskSummaryAbsent),
				SessionDirectory: ref.Root,
			},
		},
	})
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("%#v", resp)
	}
	if spawned.Load() != 0 {
		t.Fatal("OpenCode task.start must not trigger Cursor fallback")
	}
}

func TestPhaseB8_ActionsStillSessionStartOnly(t *testing.T) {
	for _, reg := range hooksengine.DefaultRegistry().All() {
		for _, et := range reg.Action.Events() {
			if et == hooks.EventTaskStart {
				t.Fatalf("%s must not subscribe to task.start", reg.Action.ID())
			}
		}
	}
}

func TestPhaseB8_CursorNativeTranslationUnchanged(t *testing.T) {
	now := time.Now().UTC()
	d, ok, err := adaptercursor.TranslateToCanonical(adaptercursor.NativePayload{
		HookEventName:  adaptercursor.NativeBeforeSubmitPrompt,
		ConversationID: "conv-1",
		GenerationID:   "gen-1",
	}, now)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if d.Event.Type != hooks.EventTaskStart {
		t.Fatalf("type=%s", d.Event.Type)
	}
	if d.Event.Payload.SessionID != "conv-1" {
		t.Fatalf("session=%q", d.Event.Payload.SessionID)
	}
	if d.Event.Payload.TaskID != "gen-1" {
		t.Fatalf("task must be generation_id, got %q", d.Event.Payload.TaskID)
	}
}

func TestPhaseB8_GovernanceResponsePreserved(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-gov")
	resp := dispatchCursorTask(t, ref, "b8-gov-1", "sess-gov", "g1")
	if resp.Status != hooksengine.StatusSuccess || resp.Aborted {
		t.Fatalf("%#v", resp)
	}
	// Fallback must not invent Action results or denials.
	if resp.Error != nil {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d", spawned.Load())
	}
}

func TestPhaseB8_NoRepoContamination(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "b8-clean")
	src := filepath.Join(ref.Root, "main.go")
	write(t, src, "package main\n")
	before, _ := os.ReadFile(src)
	_ = dispatchCursorTask(t, ref, "b8-clean-1", "sess-clean", "g1")
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
	after, _ := os.ReadFile(src)
	if string(before) != string(after) {
		t.Fatal("source mutated")
	}
	if _, err := os.Stat(filepath.Join(ref.Root, "context")); !os.IsNotExist(err) {
		t.Fatal("context/ leaked into repo")
	}
	if _, err := os.Stat(hookactions.SessionMaintainPath(ref.Home, ref.ProjectID)); err != nil {
		t.Fatal("session-maintain must live under Home")
	}
}

func TestPhaseB8_AdmissionLatencyBounded(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error { return nil })
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b8-lat")
	start := time.Now()
	res, err := hookactions.AdmitCursorPromptFallback(context.Background(), hooks.Event{
		ID: "lat-1", Type: hooks.EventTaskStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{
			ProjectID: ref.ProjectID, SessionID: "sess-lat", TaskID: "t1",
			TaskSummaryState: hooks.TaskSummaryAbsent,
		},
	}, fixedResolve(ref))
	elapsed := time.Since(start)
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("%#v err=%v", res, err)
	}
	if elapsed > hookactions.AdmissionTimeout {
		t.Fatalf("admission took %s > %s", elapsed, hookactions.AdmissionTimeout)
	}
	t.Logf("phase_b8_admission_latency_ms=%d", elapsed.Milliseconds())
}

func dispatchCursorSession(t *testing.T, ref projectFixture, et hooks.EventType, eventID, sessionID, taskID string) hooksengine.Response {
	t.Helper()
	cap := sessionCapable(et)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	payload := hooksengine.PayloadInput{ProjectID: ref.ProjectID, SessionID: sessionID}
	if taskID != "" {
		payload.TaskID = taskID
		payload.TaskSummaryState = string(hooks.TaskSummaryAbsent)
	}
	return eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: eventID, Type: string(et),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload:    payload,
		},
	})
}

func dispatchCursorTask(t *testing.T, ref projectFixture, eventID, sessionID, taskID string) hooksengine.Response {
	t.Helper()
	cap := sessionCapable(hooks.EventTaskStart)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	return eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: eventID, Type: string(hooks.EventTaskStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID:        ref.ProjectID,
				SessionID:        sessionID,
				TaskID:           taskID,
				TaskSummaryState: string(hooks.TaskSummaryAbsent),
			},
		},
	})
}
