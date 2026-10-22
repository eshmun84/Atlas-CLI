package hookactions_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

func TestPhaseB_SessionStartMaintains(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "pb-start")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	resp := dispatchSession(t, ref, hooks.EventSessionStart, "evt-start-1")
	if resp.Status != hooksengine.StatusSuccess || resp.Aborted {
		t.Fatalf("%#v", resp)
	}
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
}

func TestPhaseB_SessionResumeNotSubscribed(t *testing.T) {
	// B.2 honest contract: Actions do not subscribe to session.resume.
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "pb-resume")
	for _, reg := range hooksengine.DefaultRegistry().All() {
		for _, et := range reg.Action.Events() {
			if et == hooks.EventSessionResume {
				t.Fatalf("%s must not subscribe to session.resume", reg.Action.ID())
			}
		}
	}
	resp := dispatchSession(t, ref, hooks.EventSessionResume, "evt-resume-1")
	if resp.Count != 0 {
		t.Fatalf("session.resume must not dispatch maintain actions: %#v", resp)
	}
	if spawned.Load() != 0 {
		t.Fatal("spawner must not run for session.resume")
	}
}

func TestPhaseB_UnsubscribedEventNoop(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "pb-unsub")
	cap := sessionCapable(hooks.EventTaskStart)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: "task-1", Type: string(hooks.EventTaskStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID, TaskID: "t1",
				TaskSummaryState: string(hooks.TaskSummaryAbsent),
			},
		},
	})
	if resp.Count != 0 {
		t.Fatalf("task.start must not invoke maintain actions: %#v", resp)
	}
	if spawned.Load() != 0 {
		t.Fatal("spawner must not run")
	}
}

func TestPhaseB_NotInitializedSkip(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	t.Cleanup(func() { hookactions.SetSpawner(nil) })

	root := t.TempDir()
	homeDir := t.TempDir()
	cap := sessionCapable(hooks.EventSessionStart)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: root, Home: homeDir},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), validSession(t, "x", hooks.EventSessionStart))
	if resp.Status != hooksengine.StatusNotInitialized {
		t.Fatalf("status=%q", resp.Status)
	}
	if spawned.Load() != 0 {
		t.Fatal("must not spawn")
	}
}

func TestPhaseB_RepeatedHookDedupes(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil // admission-only; full maintain covered elsewhere
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "pb-dedupe")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	// Same event id → in-process dedupe across the two Actions.
	resp := dispatchSession(t, ref, hooks.EventSessionStart, "same-evt")
	if resp.Count != 2 {
		t.Fatalf("count=%d", resp.Count)
	}
	ok, skipped := 0, 0
	for _, r := range resp.Results {
		switch r.Outcome {
		case string(hooks.OutcomeOK):
			ok++
		case string(hooks.OutcomeSkipped):
			skipped++
		}
	}
	if ok != 1 || skipped != 1 {
		t.Fatalf("want 1 ok + 1 skipped, got ok=%d skipped=%d %#v", ok, skipped, resp.Results)
	}
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
}

func TestPhaseB_ConcurrentProcessesSafe(t *testing.T) {
	ref := seedInitializedProject(t, "pb-conc")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = hookactions.InlineSpawner(mustRef(t, ref))
		}()
	}
	wg.Wait()
	if _, err := os.Stat(atlascontext.IndexPath(ref.Home, ref.ProjectID)); err != nil {
		t.Fatal(err)
	}
}

func TestPhaseB_AdmissionCancel(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	act := &hookactions.ContextEconomyAction{}
	res, err := act.Run(ctx, hooks.Event{
		ID: "c1", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{SessionID: "s"},
	})
	if err == nil || res.Outcome != hooks.OutcomeCancelled {
		t.Fatalf("got %#v err=%v", res, err)
	}
}

func TestPhaseB_WorkerSurvivesAdmissionReturn(t *testing.T) {
	// Simulates: admission returns OK after scheduling work; work completes later.
	hookactions.SetTriggersEnabled(true)
	hookactions.ResetAdmitStateForTest()
	ref := seedInitializedProject(t, "pb-survive")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	done := make(chan struct{})
	hookactions.SetSpawner(func(r hookactions.ProjectRef) error {
		go func() {
			defer close(done)
			_ = hookactions.InlineSpawner(r)
		}()
		return nil
	})
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
	res, err := act.Run(context.Background(), hooks.Event{
		ID: "surv-1", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s"},
	})
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("admission %#v err=%v", res, err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("worker did not finish")
	}
	if _, err := os.Stat(atlascontext.IndexPath(ref.Home, ref.ProjectID)); err != nil {
		t.Fatal("maintenance must complete after admission returns")
	}
}

func TestPhaseB_CEErrorFailOpen(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "pb-ce-fail")
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

	resp := dispatchSession(t, ref, hooks.EventSessionStart, "evt-ce-fail")
	if resp.Aborted {
		t.Fatalf("must not abort session: %#v", resp)
	}
	hookactions.WaitAsyncInlineForTest()
}

func TestPhaseB_CodeIntelAbsentSkip(t *testing.T) {
	ref := seedInitializedProject(t, "pb-ci-skip")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	act := &hookactions.CodeIntelAction{
		Service: codeintel.NewService(&refreshStub{cap: codeintel.Capability{
			Provider: codeintel.ProviderCodeGraph, State: codeintel.StateUnavailable,
		}}),
	}
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err != nil || !out.Skipped {
		t.Fatalf("%#v err=%v", out, err)
	}
}

func TestPhaseB_NoSourceMutation(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "pb-nosrc")
	src := filepath.Join(ref.Root, "main.go")
	write(t, src, "package main\n")
	before, _ := os.ReadFile(src)
	_ = dispatchSession(t, ref, hooks.EventSessionStart, "evt-nosrc")
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
	after, _ := os.ReadFile(src)
	if string(before) != string(after) {
		t.Fatal("source mutated")
	}
	if _, err := os.Stat(filepath.Join(ref.Root, "context")); !os.IsNotExist(err) {
		t.Fatal("context/ in repo")
	}
}

func openCodeSessionDir(adapter adapters.ID, root string) string {
	if adapter == adapters.OpenCode {
		return root
	}
	return ""
}

func TestPhaseB_CursorAndOpenCodeAdapters(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
	})

	for _, tc := range []struct {
		name    string
		adapter adapters.ID
	}{
		{"cursor", adapters.Cursor},
		{"opencode", adapters.OpenCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hookactions.ResetAdmitStateForTest()
			ref := seedInitializedProject(t, "pb-"+tc.name)
			write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
			cap := sessionCapableFor(tc.adapter, hooks.EventSessionStart)
			eng := hooksengine.Engine{
				Registry:           hooksengine.DefaultRegistry(),
				Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
				CapabilityOverride: &cap,
			}
			resp := eng.Dispatch(context.Background(), hooksengine.Request{
				SchemaVersion: 1,
				Adapter:       string(tc.adapter),
				Event: hooksengine.EventInput{
					ID: "ad-" + tc.name, Type: string(hooks.EventSessionStart),
					OccurredAt: time.Now().UTC().Format(time.RFC3339),
					Payload: hooksengine.PayloadInput{
						ProjectID: ref.ProjectID, SessionID: "s1",
						SessionDirectory: openCodeSessionDir(tc.adapter, ref.Root),
					},
				},
			})
			if resp.Status != hooksengine.StatusSuccess {
				t.Fatalf("%#v", resp)
			}
			waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
		})
	}
}

func TestPhaseB_BothAdaptersDeduped(t *testing.T) {
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
	t.Cleanup(func() { hookactions.SetSpawner(nil) })

	ref := seedInitializedProject(t, "pb-both")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	var wg sync.WaitGroup
	for _, id := range []string{"cursor-evt", "opencode-evt"} {
		wg.Add(1)
		go func(eid string) {
			defer wg.Done()
			act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
			_, _ = act.Run(context.Background(), hooks.Event{
				ID: eid, Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
				Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s"},
			})
		}(id)
	}
	wg.Wait()
	work.Wait()
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want exactly 1 under lease fence", spawned.Load())
	}
}

func dispatchSession(t *testing.T, ref projectFixture, et hooks.EventType, eventID string) hooksengine.Response {
	t.Helper()
	cap := sessionCapable(et)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	return eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: eventID, Type: string(et),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID, SessionID: "sess-1",
			},
		},
	})
}

func sessionCapable(events ...hooks.EventType) hooks.Capability {
	return sessionCapableFor(adapters.Cursor, events...)
}

func sessionCapableFor(id adapters.ID, events ...hooks.EventType) hooks.Capability {
	cap := hooks.Capability{
		Adapter:         id,
		HooksCapability: true,
		Events:          map[hooks.EventType]hooks.EventSupport{},
	}
	for _, et := range hooks.KnownEventTypes() {
		cap.Events[et] = hooks.SupportUnsupported
	}
	for _, et := range events {
		cap.Events[et] = hooks.SupportSupported
	}
	return cap
}

func validSession(t *testing.T, projectID string, et hooks.EventType) hooksengine.Request {
	t.Helper()
	return hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: "evt", Type: string(et),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload:    hooksengine.PayloadInput{ProjectID: projectID, SessionID: "s"},
		},
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			// Brief settle so async workers release TempDir locks before cleanup.
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
