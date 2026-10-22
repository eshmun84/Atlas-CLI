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
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

func c2cRef(t *testing.T, name string) hookactions.ProjectRef {
	t.Helper()
	return c2bRef(t, name)
}

func c2cAction(ref hookactions.ProjectRef) *hookactions.PostCodeAction {
	return &hookactions.PostCodeAction{Resolve: fixedResolve(projectFixture{
		Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
	})}
}

func c2cToolEvent(adapter adapters.ID, tool, status, sessionID, projectID, eventID, root string) hooks.Event {
	return hooks.Event{
		ID:         eventID,
		Type:       hooks.EventAfterToolExecution,
		OccurredAt: time.Now().UTC(),
		Adapter:    adapter,
		Payload: hooks.Payload{
			ProjectID:        projectID,
			SessionDirectory: openCodeSessionDir(adapter, root),
			SessionID:        sessionID,
			ToolName:         tool,
			Status:           status,
		},
	}
}

func c2cInstallSchedulerNoop(t *testing.T) *atomic.Int32 {
	t.Helper()
	var spawns atomic.Int32
	hookactions.SetSchedulerSpawner(func(ref hookactions.ProjectRef, token string, fence int64) error {
		spawns.Add(1)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })
	return &spawns
}

func TestC2C_AllowWrite(t *testing.T) {
	ref := c2cRef(t, "c2c-write")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	res, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-write", ref.Root))
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("outcome=%s err=%v", res.Outcome, err)
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 {
		t.Fatalf("pending=%d processed=%d", doc.PendingGeneration, doc.ProcessedGeneration)
	}
}

func TestC2C_AllowEdit(t *testing.T) {
	ref := c2cRef(t, "c2c-edit")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	// OpenCode edit; Cursor Edit maps to Write (covered by AllowWrite).
	res, err := act.Run(context.Background(), c2cToolEvent(adapters.OpenCode, "edit", "success", "s1", ref.ProjectID, "e-edit", ref.Root))
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("outcome=%s err=%v", res.Outcome, err)
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration != 1 {
		t.Fatalf("pending=%d", doc.PendingGeneration)
	}
}

func TestC2C_RejectRead(t *testing.T) {
	ref := c2cRef(t, "c2c-read")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	for _, tc := range []struct {
		adapter adapters.ID
		tool    string
	}{
		{adapters.Cursor, "Read"},
		{adapters.OpenCode, "read"},
		{adapters.Cursor, "Grep"},
		{adapters.OpenCode, "grep"},
	} {
		res, err := act.Run(context.Background(), c2cToolEvent(tc.adapter, tc.tool, "success", "s1", ref.ProjectID, "e-"+tc.tool, ref.Root))
		if err != nil || res.Outcome != hooks.OutcomeSkipped {
			t.Fatalf("%s/%s: outcome=%s err=%v", tc.adapter, tc.tool, res.Outcome, err)
		}
	}
	if _, err := os.Stat(hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)); !os.IsNotExist(err) {
		t.Fatal("read tools must not create pending")
	}
}

func TestC2C_RejectShellBash(t *testing.T) {
	ref := c2cRef(t, "c2c-shell")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	for _, tc := range []struct {
		adapter adapters.ID
		tool    string
	}{
		{adapters.Cursor, "Shell"},
		{adapters.OpenCode, "bash"},
		{adapters.Cursor, "Bash"},
		{adapters.OpenCode, "Shell"},
	} {
		res, err := act.Run(context.Background(), c2cToolEvent(tc.adapter, tc.tool, "success", "s1", ref.ProjectID, "e-"+string(tc.adapter)+"-"+tc.tool, ref.Root))
		if err != nil || res.Outcome != hooks.OutcomeSkipped {
			t.Fatalf("%s/%s: outcome=%s err=%v", tc.adapter, tc.tool, res.Outcome, err)
		}
	}
	if _, err := os.Stat(hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)); !os.IsNotExist(err) {
		t.Fatal("shell/bash must not create pending")
	}
}

func TestC2C_RejectUnknownTool(t *testing.T) {
	ref := c2cRef(t, "c2c-unk")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	for _, tc := range []struct {
		adapter adapters.ID
		tool    string
	}{
		{adapters.Cursor, "MCP:filesystem"},
		{adapters.Cursor, "Task"},
		{adapters.OpenCode, "webfetch"},
		{adapters.OpenCode, "custom_tool"},
		{adapters.Cursor, "write"}, // case-sensitive; Cursor is PascalCase
		{adapters.OpenCode, "Write"},
	} {
		res, err := act.Run(context.Background(), c2cToolEvent(tc.adapter, tc.tool, "success", "s1", ref.ProjectID, "e-unk", ref.Root))
		if err != nil || res.Outcome != hooks.OutcomeSkipped {
			t.Fatalf("%s/%s: outcome=%s err=%v", tc.adapter, tc.tool, res.Outcome, err)
		}
	}
}

func TestC2C_RejectErrorStatus(t *testing.T) {
	ref := c2cRef(t, "c2c-err")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	for _, status := range []string{"error", "failed", "failure", "denied", "cancelled"} {
		res, err := act.Run(context.Background(), c2cToolEvent(adapters.OpenCode, "write", status, "s1", ref.ProjectID, "e-"+status, ref.Root))
		if err != nil || res.Outcome != hooks.OutcomeSkipped {
			t.Fatalf("status=%s outcome=%s err=%v", status, res.Outcome, err)
		}
	}
	if _, err := os.Stat(hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)); !os.IsNotExist(err) {
		t.Fatal("error status must not bump")
	}
}

func TestC2C_RejectIncompletePayload(t *testing.T) {
	ref := c2cRef(t, "c2c-incomplete")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	res, err := act.Run(context.Background(), hooks.Event{
		ID: "e-empty", Type: hooks.EventAfterToolExecution, OccurredAt: time.Now().UTC(),
		Adapter: adapters.Cursor,
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s1", ToolName: "", Status: "success"},
	})
	if err != nil || res.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("empty tool: outcome=%s err=%v", res.Outcome, err)
	}
	res, err = act.Run(context.Background(), hooks.Event{
		ID: "e-wrong-type", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Adapter: adapters.Cursor,
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s1", ToolName: "Write", Status: "success"},
	})
	if err != nil || res.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("wrong type: outcome=%s err=%v", res.Outcome, err)
	}
}

func TestC2C_TriggersDisabled(t *testing.T) {
	ref := c2cRef(t, "c2c-off")
	hookactions.SetTriggersEnabled(false)
	t.Cleanup(func() { hookactions.SetTriggersEnabled(true) })
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	res, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-off", ref.Root))
	if err != nil || res.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("outcome=%s err=%v", res.Outcome, err)
	}
	if _, err := os.Stat(hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)); !os.IsNotExist(err) {
		t.Fatal("disabled triggers must not write pending")
	}
}

func TestC2C_BumpPendingGreaterThanProcessed(t *testing.T) {
	ref := c2cRef(t, "c2c-bump")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	res, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Delete", "success", "s1", ref.ProjectID, "e-del", ref.Root))
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("outcome=%s err=%v", res.Outcome, err)
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration <= doc.ProcessedGeneration {
		t.Fatalf("pending=%d processed=%d", doc.PendingGeneration, doc.ProcessedGeneration)
	}
}

func TestC2C_ActionDoesNotAlterSessionMaintain(t *testing.T) {
	ref := c2cRef(t, "c2c-sess")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	sessionPath := filepath.Join(ref.Home, "projects", ref.ProjectID, "state", "session-maintain.yaml")
	before, _ := os.ReadFile(sessionPath)
	act := c2cAction(ref)
	_, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "ok", "sess-fence", ref.ProjectID, "e-sess", ref.Root))
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(sessionPath)
	if err == nil || !os.IsNotExist(err) {
		if string(before) != string(after) {
			t.Fatalf("session-maintain mutated\nbefore=%q\nafter=%q", before, after)
		}
	}
	st, ok, err := hookactions.LoadSessionMaintainState(ref.Home, ref.ProjectID, "sess-fence")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("post-code must not create session fence, state=%q", st)
	}
}

func TestC2C_CursorEngineToPending(t *testing.T) {
	ref := c2cRef(t, "c2c-cursor-eng")
	hookactions.SetTriggersEnabled(true)
	spawns := c2cInstallSchedulerNoop(t)
	cap := sessionCapable(hooks.EventAfterToolExecution)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: "cursor-tool-1", Type: string(hooks.EventAfterToolExecution),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID, SessionID: "c-sess",
				ToolName: "Write", Status: "success",
			},
		},
	})
	if resp.Status != hooksengine.StatusSuccess || resp.Aborted {
		t.Fatalf("resp=%#v", resp)
	}
	if resp.Count != 1 {
		t.Fatalf("count=%d want 1 (postcode only)", resp.Count)
	}
	if resp.Results[0].ActionID != hookactions.ActionIDPostCode || resp.Results[0].Outcome != string(hooks.OutcomeOK) {
		t.Fatalf("result=%#v", resp.Results[0])
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration != 1 {
		t.Fatalf("pending=%d", doc.PendingGeneration)
	}
	if spawns.Load() != 1 {
		t.Fatalf("scheduler spawns=%d", spawns.Load())
	}
}

func TestC2C_OpenCodeEngineToPending(t *testing.T) {
	ref := c2cRef(t, "c2c-oc-eng")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	cap := sessionCapableFor(adapters.OpenCode, hooks.EventAfterToolExecution)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.OpenCode),
		Event: hooksengine.EventInput{
			ID: "oc-tool-1", Type: string(hooks.EventAfterToolExecution),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID, SessionID: "oc-sess",
				ToolName: "apply_patch", Status: "success",
				SessionDirectory: ref.Root,
			},
		},
	})
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("resp=%#v", resp)
	}
	if resp.Count != 1 || resp.Results[0].Outcome != string(hooks.OutcomeOK) {
		t.Fatalf("results=%#v", resp.Results)
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration != 1 {
		t.Fatalf("pending=%d", doc.PendingGeneration)
	}
}

func TestC2C_TenRapidEditsCoalesce(t *testing.T) {
	ref := c2cRef(t, "c2c-coalesce")
	advance := installFakeClock(t, time.Date(2026, 10, 19, 21, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	start := time.Date(2026, 10, 19, 21, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		res, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-"+itoa(i), ref.Root))
		if err != nil || res.Outcome != hooks.OutcomeOK {
			t.Fatalf("i=%d outcome=%s err=%v", i, res.Outcome, err)
		}
		advance(time.Second)
	}
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 10 {
		t.Fatalf("pending=%d want 10", doc.PendingGeneration)
	}
	due, err := time.Parse(time.RFC3339Nano, doc.DueAt)
	if err != nil {
		due, err = time.Parse(time.RFC3339, doc.DueAt)
		if err != nil {
			t.Fatalf("due_at: %v", err)
		}
	}
	if due.After(start.Add(hookactions.MaxCoalesceWindow)) {
		t.Fatalf("due_at %v exceeds coalesce max", due)
	}
}

func TestC2C_EditDuringMaintainSecondCycle(t *testing.T) {
	ref := c2cRef(t, "c2c-during")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 21, 30, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	act := c2cAction(ref)

	var calls atomic.Int32
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		if calls.Add(1) == 1 {
			_, _ = act.Run(context.Background(), c2cToolEvent(adapters.OpenCode, "edit", "success", "s1", r.ProjectID, "e-mid", r.Root))
		}
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(hookactions.InlineSchedulerSpawner)
	t.Cleanup(func() {
		hookactions.SetSchedulerSpawner(nil)
		hookactions.SetPostCodeMaintainForTest(nil)
	})

	res, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-first", ref.Root))
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("outcome=%s err=%v", res.Outcome, err)
	}
	if calls.Load() < 2 {
		t.Fatalf("want second CE/CI cycle, calls=%d", calls.Load())
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration != doc.ProcessedGeneration || doc.ProcessedGeneration < 2 {
		t.Fatalf("pending=%d processed=%d", doc.PendingGeneration, doc.ProcessedGeneration)
	}
}

func TestC2C_ConcurrentAdaptersFencing(t *testing.T) {
	ref := c2cRef(t, "c2c-fence")
	hookactions.SetTriggersEnabled(true)
	var reserved atomic.Int32
	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		reserved.Add(1)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })

	act := c2cAction(ref)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			adapter := adapters.Cursor
			tool := "Write"
			if i%2 == 1 {
				adapter = adapters.OpenCode
				tool = "write"
			}
			_, _ = act.Run(context.Background(), c2cToolEvent(adapter, tool, "success", "s", ref.ProjectID, "e-"+itoa(i), ref.Root))
		}(i)
	}
	wg.Wait()
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.PendingGeneration != 20 {
		t.Fatalf("pending=%d want 20", doc.PendingGeneration)
	}
	if reserved.Load() != 1 {
		t.Fatalf("scheduler reservations/spawns=%d want 1", reserved.Load())
	}
	if doc.OwnerToken == "" || doc.OwnerFence != 1 {
		t.Fatalf("ownership: token=%q fence=%d", doc.OwnerToken, doc.OwnerFence)
	}
}

func TestC2C_NoFurtherHooksSchedulerRuns(t *testing.T) {
	ref := c2cRef(t, "c2c-idle")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 22, 0, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	_, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-idle", ref.Root))
	if err != nil {
		t.Fatal(err)
	}
	// Drop the noop reservation so a real scheduler can reclaim without more tool events.
	doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.OwnerToken != "" {
		if _, err := hookactions.ClearOwnReservation(ref.Home, ref.ProjectID, doc.OwnerToken, doc.OwnerFence); err != nil {
			t.Fatal(err)
		}
	}
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
		t.Fatal("scheduler must execute without further tool hooks")
	}
}

func TestC2C_SchedulerFailureThenRecovery(t *testing.T) {
	ref := c2cRef(t, "c2c-recover")
	hookactions.SetTriggersEnabled(true)
	act := c2cAction(ref)

	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		return errors.New("spawn boom")
	})
	res, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-fail", ref.Root))
	if err == nil || res.Outcome != hooks.OutcomeFailed {
		t.Fatalf("want failed admit, outcome=%s err=%v", res.Outcome, err)
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration != 1 || doc.OwnerToken != "" {
		t.Fatalf("dirty without owner expected: %+v", doc)
	}

	var recovered atomic.Bool
	hookactions.SetSchedulerSpawner(func(r hookactions.ProjectRef, token string, fence int64) error {
		recovered.Store(true)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSchedulerSpawner(nil) })
	if err := hookactions.EnsurePostCodeSchedulerIfDirty(ref); err != nil {
		t.Fatal(err)
	}
	if !recovered.Load() {
		t.Fatal("expected recovery spawn")
	}
}

func TestC2C_DuplicateEventsSafe(t *testing.T) {
	ref := c2cRef(t, "c2c-dup")
	hookactions.SetTriggersEnabled(true)
	spawns := c2cInstallSchedulerNoop(t)
	act := c2cAction(ref)
	ev := c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "same-id", ref.Root)
	for i := 0; i < 3; i++ {
		res, err := act.Run(context.Background(), ev)
		if err != nil || res.Outcome != hooks.OutcomeOK {
			t.Fatalf("i=%d outcome=%s err=%v", i, res.Outcome, err)
		}
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.PendingGeneration != 3 {
		t.Fatalf("pending=%d want 3 (each signal bumps)", doc.PendingGeneration)
	}
	if spawns.Load() != 1 {
		t.Fatalf("spawns=%d want 1 (first owns)", spawns.Load())
	}
}

func TestC2C_HookReturnsWithoutWaitingMaintain(t *testing.T) {
	ref := c2cRef(t, "c2c-async")
	_ = installFakeClock(t, time.Date(2026, 10, 19, 22, 30, 0, 0, time.UTC))
	hookactions.SetTriggersEnabled(true)

	started := make(chan struct{})
	release := make(chan struct{})
	hookactions.SetPostCodeMaintainForTest(func(ctx context.Context, r hookactions.ProjectRef) (string, string, string) {
		close(started)
		<-release
		return hookactions.MaintainOutcomeOK, hookactions.MaintainOutcomeOK, hookactions.MaintainErrNone
	})
	hookactions.SetSchedulerSpawner(hookactions.AsyncInlineSchedulerSpawner)
	t.Cleanup(func() {
		close(release)
		hookactions.WaitAsyncSchedulerForTest()
		hookactions.SetSchedulerSpawner(nil)
		hookactions.SetPostCodeMaintainForTest(nil)
	})

	act := c2cAction(ref)
	begin := time.Now()
	res, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-async", ref.Root))
	elapsed := time.Since(begin)
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("outcome=%s err=%v", res.Outcome, err)
	}
	if elapsed > hookactions.AdmissionTimeout {
		t.Fatalf("hook blocked on maintain: %s", elapsed)
	}
	select {
	case <-started:
		// maintain still running after Action returned
	case <-time.After(2 * time.Second):
		t.Fatal("maintain never started")
	}
	doc, _ := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
	if doc.ProcessedGeneration != 0 {
		t.Fatal("Action must return before maintain commits")
	}
}

func TestC2C_UserOwnedHooksIntact(t *testing.T) {
	ref := c2cRef(t, "c2c-userhooks")
	hookactions.SetTriggersEnabled(true)
	_ = c2cInstallSchedulerNoop(t)

	cursorHooks := filepath.Join(ref.Root, ".cursor", "hooks.json")
	ocPlugin := filepath.Join(ref.Root, ".opencode", "plugins", "user-plugin.ts")
	userCursor := `{"version":1,"hooks":{"postToolUse":[{"command":"./user-audit.sh"}]}}`
	userOC := "// user owned plugin\nexport default {}\n"
	write(t, cursorHooks, userCursor)
	write(t, ocPlugin, userOC)

	act := c2cAction(ref)
	_, err := act.Run(context.Background(), c2cToolEvent(adapters.Cursor, "Write", "success", "s1", ref.ProjectID, "e-uh", ref.Root))
	if err != nil {
		t.Fatal(err)
	}
	_, err = act.Run(context.Background(), c2cToolEvent(adapters.OpenCode, "write", "success", "s1", ref.ProjectID, "e-uh2", ref.Root))
	if err != nil {
		t.Fatal(err)
	}

	gotC, err := os.ReadFile(cursorHooks)
	if err != nil || string(gotC) != userCursor {
		t.Fatalf("cursor hooks mutated: %s", gotC)
	}
	gotO, err := os.ReadFile(ocPlugin)
	if err != nil || string(gotO) != userOC {
		t.Fatalf("opencode plugin mutated: %s", gotO)
	}
}

func TestC2C_AllowlistExactAndDocumented(t *testing.T) {
	cursor := hookactions.CursorPostCodeAllowlist()
	if len(cursor) != 2 || cursor[0] != "Write" || cursor[1] != "Delete" {
		t.Fatalf("cursor allowlist=%v", cursor)
	}
	oc := hookactions.OpenCodePostCodeAllowlist()
	if len(oc) != 3 || oc[0] != "write" || oc[1] != "edit" || oc[2] != "apply_patch" {
		t.Fatalf("opencode allowlist=%v", oc)
	}
	if !hookactions.ToolAllowsPostCode(adapters.Cursor, "Write") || hookactions.ToolAllowsPostCode(adapters.Cursor, "Shell") {
		t.Fatal("cursor allowlist mismatch")
	}
	if !hookactions.ToolAllowsPostCode(adapters.OpenCode, "apply_patch") || hookactions.ToolAllowsPostCode(adapters.OpenCode, "bash") {
		t.Fatal("opencode allowlist mismatch")
	}
}

func TestC2C_PostCodeSubscribedOnlyToAfterTool(t *testing.T) {
	act := &hookactions.PostCodeAction{}
	evs := act.Events()
	if len(evs) != 1 || evs[0] != hooks.EventAfterToolExecution {
		t.Fatalf("events=%v", evs)
	}
	if act.Mode() != hooks.ModeNonBlocking {
		t.Fatalf("mode=%s", act.Mode())
	}
	if act.ID() != hookactions.ActionIDPostCode {
		t.Fatalf("id=%s", act.ID())
	}
}
