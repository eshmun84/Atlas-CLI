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
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
	"gopkg.in/yaml.v3"
)

func TestPhaseB2_AdmitLeaseSingleSpawnUnderParallel(t *testing.T) {
	for _, n := range []int{2, 8, 20} {
		t.Run(itoa(n), func(t *testing.T) {
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

			ref := seedInitializedProject(t, "b2-conc-"+itoa(n))
			write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
			write(t, filepath.Join(ref.Root, "hello.go"), "package demo\n")

			var wg sync.WaitGroup
			var okN, skipN, failN atomic.Int32
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
					res, err := act.Run(context.Background(), hooks.Event{
						ID: "b2-" + itoa(i), Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
						Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s"},
					})
					switch res.Outcome {
					case hooks.OutcomeOK:
						okN.Add(1)
					case hooks.OutcomeSkipped:
						skipN.Add(1)
					default:
						failN.Add(1)
						t.Logf("admit i=%d outcome=%s err=%v class=%s", i, res.Outcome, err, hookactions.ClassifyAdmitError(err))
					}
				}(i)
			}
			wg.Wait()
			work.Wait()

			if spawned.Load() != 1 {
				t.Fatalf("n=%d spawned=%d want 1 (ok=%d skip=%d fail=%d)", n, spawned.Load(), okN.Load(), skipN.Load(), failN.Load())
			}
			if okN.Load() != 1 {
				t.Fatalf("n=%d ok admissions=%d want 1", n, okN.Load())
			}
			if failN.Load() != 0 {
				t.Fatalf("n=%d unexpected failures=%d", n, failN.Load())
			}
			waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
			rec, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
			if err != nil || !ok {
				t.Fatalf("maintain-last: ok=%v err=%v", ok, err)
			}
			if rec.Status != hookactions.MaintainStatusComplete {
				t.Fatalf("status=%q want complete: %#v", rec.Status, rec)
			}
			if rec.FinishedAt == "" || rec.DurationMS < 0 {
				t.Fatalf("incomplete record: %#v", rec)
			}
		})
	}
}

func TestPhaseB2_LockBusySkips(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b2-busy")
	lock, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hookactions.ReleaseMaintainLock(lock) })

	act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
	res, err := act.Run(context.Background(), hooks.Event{
		ID: "busy-1", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s"},
	})
	if err != nil || res.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("want skipped, got %#v err=%v", res, err)
	}
	if spawned.Load() != 0 {
		t.Fatal("must not spawn while lock held")
	}
}

func TestPhaseB2_FreshLeaseBlocksSecondAdmit(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	var spawned atomic.Int32
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		spawned.Add(1)
		return nil
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b2-lease")
	// Simulate parent A: holds nothing now but left a fresh admitted lease
	// (as after release-before-spawn). Write lease then try admit without lock held by us.
	// First admit will write its own lease; we need: write lease while we briefly hold lock then release,
	// then second admit sees it.
	lock, err := hookactions.TryAcquireMaintainLock(ref.Home, ref.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	// Use package-level write via first admission path: manually place lease file.
	leasePath := filepath.Join(ref.Home, "projects", ref.ProjectID, "state", "maintain.lease")
	if err := os.MkdirAll(filepath.Dir(leasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	body, _ := yaml.Marshal(map[string]any{
		"schema_version": 1,
		"state":          "admitted",
		"pid":            os.Getpid(),
		"at":             time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err := os.WriteFile(leasePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = hookactions.ReleaseMaintainLock(lock)

	act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
	res, err := act.Run(context.Background(), hooks.Event{
		ID: "lease-1", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s"},
	})
	if err != nil || res.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("fresh lease must skip: %#v err=%v", res, err)
	}
	if spawned.Load() != 0 {
		t.Fatal("must not spawn under fresh lease")
	}
}

func TestPhaseB2_MaintainLastSuccessAndNoop(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "b2-last")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	write(t, filepath.Join(ref.Root, "a.go"), "package demo\n")

	_ = dispatchSession(t, ref, hooks.EventSessionStart, "b2-last-1")
	waitForFile(t, hookactions.MaintainLastPath(ref.Home, ref.ProjectID), 30*time.Second)
	hookactions.WaitAsyncInlineForTest()

	rec, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok {
		t.Fatalf("load: %v ok=%v", err, ok)
	}
	if rec.Status != hookactions.MaintainStatusComplete {
		t.Fatalf("%#v", rec)
	}
	if rec.CEOutcome != hookactions.MaintainOutcomeOK {
		t.Fatalf("ce=%q", rec.CEOutcome)
	}
	if rec.StartedAt == "" || rec.FinishedAt == "" {
		t.Fatalf("timestamps missing: %#v", rec)
	}
	// No secrets / source content
	raw, _ := os.ReadFile(hookactions.MaintainLastPath(ref.Home, ref.ProjectID))
	if containsAny(string(raw), []string{"package demo", "secret", "token=", "password"}) {
		t.Fatalf("sensitive content in maintain-last: %s", raw)
	}

	// Second session → noop CE still complete
	hookactions.ResetAdmitStateForTest()
	_ = dispatchSession(t, ref, hooks.EventSessionStart, "b2-last-2")
	hookactions.WaitAsyncInlineForTest()
	rec2, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok || rec2.Status != hookactions.MaintainStatusComplete {
		t.Fatalf("%#v ok=%v err=%v", rec2, ok, err)
	}
	if rec2.CEOutcome != hookactions.MaintainOutcomeOK {
		t.Fatalf("noop ce=%q", rec2.CEOutcome)
	}
}

func TestPhaseB2_MaintainLastIncompleteAfterCrashMarker(t *testing.T) {
	ref := seedInitializedProject(t, "b2-crash")
	// Simulate SIGKILL mid-run: running record with no finished_at.
	running := hookactions.MaintainLastRecord{
		SchemaVersion: 1,
		Status:        hookactions.MaintainStatusRunning,
		StartedAt:     time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		CEOutcome:     hookactions.MaintainOutcomePending,
		CIOutcome:     hookactions.MaintainOutcomePending,
		ErrorCode:     hookactions.MaintainErrNone,
		PID:           1,
	}
	data, _ := yaml.Marshal(&running)
	path := hookactions.MaintainLastPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	_ = dispatchSession(t, ref, hooks.EventSessionStart, "b2-crash-1")
	hookactions.WaitAsyncInlineForTest()

	// Final record must be complete for the recovery run (prior incomplete was overwritten
	// after being marked). Verify we never leave status=running without finished_at.
	rec, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok {
		t.Fatalf("load: %v ok=%v", err, ok)
	}
	if rec.Status == hookactions.MaintainStatusRunning {
		t.Fatalf("must not leave running after recover: %#v", rec)
	}
	if rec.Status != hookactions.MaintainStatusComplete {
		t.Fatalf("want complete after recover: %#v", rec)
	}
	if rec.FinishedAt == "" {
		t.Fatal("finished_at required for complete")
	}
}

func TestPhaseB2_SpawnFailureClassified(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(func(hookactions.ProjectRef) error {
		return errors.New("exec boom")
	})
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() { hookactions.SetSpawner(nil); hookactions.ResetAdmitStateForTest() })

	ref := seedInitializedProject(t, "b2-spawn")
	act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
	res, err := act.Run(context.Background(), hooks.Event{
		ID: "spawn-fail", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s"},
	})
	if res.Outcome != hooks.OutcomeFailed {
		t.Fatalf("%#v err=%v", res, err)
	}
	if hookactions.ClassifyAdmitError(err) != hookactions.AdmitClassSpawnFail {
		t.Fatalf("class=%q err=%v", hookactions.ClassifyAdmitError(err), err)
	}
	// Lease must be cleared after spawn failure
	lease := filepath.Join(ref.Home, "projects", ref.ProjectID, "state", "maintain.lease")
	if _, err := os.Stat(lease); !os.IsNotExist(err) {
		t.Fatalf("lease must be cleared on spawn failure: %v", err)
	}
}

func TestPhaseB2_CursorOpenCodeParallelOneSpawn(t *testing.T) {
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
	t.Cleanup(func() {
		work.Wait()
		hookactions.SetSpawner(nil)
	})

	ref := seedInitializedProject(t, "b2-both")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	hookactions.ResetAdmitStateForTest()
	var wg sync.WaitGroup
	for _, ad := range []adapters.ID{adapters.Cursor, adapters.OpenCode} {
		wg.Add(1)
		go func(ad adapters.ID) {
			defer wg.Done()
			cap := sessionCapableFor(ad, hooks.EventSessionStart)
			eng := hooksengine.Engine{
				Registry:           hooksengine.DefaultRegistry(),
				Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
				CapabilityOverride: &cap,
			}
			_ = eng.Dispatch(context.Background(), hooksengine.Request{
				SchemaVersion: 1,
				Adapter:       string(ad),
				Event: hooksengine.EventInput{
					ID: "b2-" + string(ad), Type: string(hooks.EventSessionStart),
					OccurredAt: time.Now().UTC().Format(time.RFC3339),
					Payload: hooksengine.PayloadInput{
						ProjectID: ref.ProjectID, SessionID: "s",
						SessionDirectory: openCodeSessionDir(ad, ref.Root),
					},
				},
			})
		}(ad)
	}
	wg.Wait()
	work.Wait()
	if spawned.Load() != 1 {
		t.Fatalf("spawned=%d want 1", spawned.Load())
	}
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
}

func TestPhaseB2_AtomicMaintainLastNoPartialYAML(t *testing.T) {
	ref := seedInitializedProject(t, "b2-atomic")
	path := hookactions.MaintainLastPath(ref.Home, ref.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Partial garbage then successful write must replace atomically.
	if err := os.WriteFile(path, []byte("schema_version: 1\n\x00BROKEN"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = hookactions.InlineSpawner(mustRef(t, ref))
	rec, ok, err := hookactions.LoadMaintainLast(ref.Home, ref.ProjectID)
	if err != nil || !ok {
		t.Fatalf("after worker: ok=%v err=%v", ok, err)
	}
	if rec.Status != hookactions.MaintainStatusComplete {
		t.Fatalf("%#v", rec)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if containsAny(string(raw), []string{"BROKEN", "\x00"}) {
		t.Fatalf("partial content survived: %q", raw)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if n != "" && containsFold(s, n) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || len(s) > 0 && findSub(s, sub)))
}

func findSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
