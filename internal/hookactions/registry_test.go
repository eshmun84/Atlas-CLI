package hookactions_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

func TestRegistry_RegistrationsStable(t *testing.T) {
	regs := hookactions.Registrations()
	if len(regs) != 2 {
		t.Fatalf("len=%d", len(regs))
	}
	reg, err := hooks.NewRegistry(regs)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, r := range reg.All() {
		id := r.Action.ID()
		if ids[id] {
			t.Fatalf("duplicate %s", id)
		}
		ids[id] = true
		if r.Action.Mode() != hooks.ModeNonBlocking {
			t.Fatalf("%s mode=%s", id, r.Action.Mode())
		}
		if r.FailurePolicy != hooks.FailContinue {
			t.Fatalf("%s policy=%s", id, r.FailurePolicy)
		}
		if r.Timeout <= 0 {
			t.Fatalf("%s timeout unset", id)
		}
	}
	if !ids[hookactions.ActionIDContextEconomy] || !ids[hookactions.ActionIDCodeIntel] {
		t.Fatalf("ids=%v", ids)
	}
}

func TestRegistry_NoMemoryActions(t *testing.T) {
	for _, r := range hookactions.Registrations() {
		if strings.HasPrefix(r.Action.ID(), "memory.") {
			t.Fatalf("memory action registered: %s", r.Action.ID())
		}
	}
}

func TestRegistry_DefaultRegistryIncludesMaintenance(t *testing.T) {
	reg := hooksengine.DefaultRegistry()
	if reg.Len() != 2 {
		t.Fatalf("DefaultRegistry len=%d want 2", reg.Len())
	}
	for _, r := range reg.All() {
		if strings.HasPrefix(r.Action.ID(), "memory.") {
			t.Fatalf("memory action in DefaultRegistry: %s", r.Action.ID())
		}
	}
}

func TestRegistry_DispatchSkippedWhenTriggersDisabled(t *testing.T) {
	hookactions.SetTriggersEnabled(false)
	t.Cleanup(func() { hookactions.SetTriggersEnabled(true) })
	hookactions.ResetAdmitStateForTest()
	ref := seedInitializedProject(t, "hooks-skip")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	cap := hooks.Capability{
		Adapter:         adapters.Cursor,
		HooksCapability: true,
		Events:          map[hooks.EventType]hooks.EventSupport{},
	}
	for _, et := range hooks.KnownEventTypes() {
		cap.Events[et] = hooks.SupportUnsupported
	}
	cap.Events[hooks.EventSessionStart] = hooks.SupportSupported

	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID:         "evt-1",
			Type:       string(hooks.EventSessionStart),
			OccurredAt: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID,
				SessionID: "sess-1",
			},
		},
	})
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("status=%q %#v", resp.Status, resp)
	}
	if resp.Count != 2 {
		t.Fatalf("count=%d want 2 registered actions", resp.Count)
	}
	for _, r := range resp.Results {
		if r.Outcome != string(hooks.OutcomeSkipped) {
			t.Fatalf("Phase A dispatch must skip mutations: %#v", r)
		}
	}
	if _, err := os.Stat(atlascontext.IndexPath(ref.Home, ref.ProjectID)); !os.IsNotExist(err) {
		t.Fatal("automatic triggers disabled: CE must not write")
	}
	if _, err := os.Stat(filepath.Join(ref.Home, "projects", ref.ProjectID, "codegraph", "graph.db")); !os.IsNotExist(err) {
		t.Fatal("automatic triggers disabled: CodeIntel must not write")
	}
}

func TestRegistry_DispatchStructuredWhenTriggersEnabled(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	ref := seedInitializedProject(t, "hooks-enabled")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	reg := hooksengine.DefaultRegistry()

	cap := hooks.Capability{
		Adapter:         adapters.Cursor,
		HooksCapability: true,
		Events:          map[hooks.EventType]hooks.EventSupport{},
	}
	for _, et := range hooks.KnownEventTypes() {
		cap.Events[et] = hooks.SupportUnsupported
	}
	cap.Events[hooks.EventSessionStart] = hooks.SupportSupported

	eng := hooksengine.Engine{
		Registry:           reg,
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID:         "evt-2",
			Type:       string(hooks.EventSessionStart),
			OccurredAt: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID,
				SessionID: "sess-2",
			},
		},
	})
	if resp.Status != hooksengine.StatusSuccess || resp.Aborted {
		t.Fatalf("non-blocking maintain must not abort: %#v", resp)
	}
	if resp.Count != 2 {
		t.Fatalf("count=%d", resp.Count)
	}
	byID := map[string]string{}
	for _, r := range resp.Results {
		byID[r.ActionID] = r.Outcome
	}
	ok, skipped := 0, 0
	for _, o := range byID {
		switch o {
		case string(hooks.OutcomeOK):
			ok++
		case string(hooks.OutcomeSkipped):
			skipped++
		}
	}
	if ok != 1 || skipped != 1 {
		t.Fatalf("want 1 admit ok + 1 deduped skip, got %#v", byID)
	}
	waitForFile(t, atlascontext.IndexPath(ref.Home, ref.ProjectID), 30*time.Second)
}

func TestRegistry_UnregisteredActionAbsent(t *testing.T) {
	reg := hooksengine.DefaultRegistry()
	for _, r := range reg.All() {
		if r.Action.ID() == "memory.inject" {
			t.Fatal("unregistered memory.inject must not appear")
		}
	}
	if len(reg.ForEvent(hooks.EventTaskComplete)) != 0 {
		t.Fatal("Phase A maintenance must not subscribe to task.complete yet")
	}
}

func TestRegistry_IdempotentMaintain(t *testing.T) {
	ref := seedInitializedProject(t, "hooks-idem")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	a, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home)
	if err != nil {
		t.Fatal(err)
	}
	if a.Outcome != hooks.OutcomeOK || !b.Noop {
		t.Fatalf("idempotent: first=%#v second=%#v", a, b)
	}
}

func TestRegistry_RecoverableErrorsDoNotAbortDispatch(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	hookactions.SetSpawner(hookactions.AsyncInlineSpawner)
	hookactions.ResetAdmitStateForTest()
	t.Cleanup(func() {
		hookactions.WaitAsyncInlineForTest()
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})

	// Unreadable CE: worker fail-opens; admission must not abort the session.
	ref := seedInitializedProject(t, "hooks-recover")
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

	cap := hooks.Capability{
		Adapter:         adapters.Cursor,
		HooksCapability: true,
		Events:          map[hooks.EventType]hooks.EventSupport{},
	}
	for _, et := range hooks.KnownEventTypes() {
		cap.Events[et] = hooks.SupportUnsupported
	}
	cap.Events[hooks.EventSessionStart] = hooks.SupportSupported

	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		Env:                hooksengine.Env{Root: ref.Root, Home: ref.Home},
		CapabilityOverride: &cap,
	}
	resp := eng.Dispatch(context.Background(), hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID:         "evt-3",
			Type:       string(hooks.EventSessionStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: ref.ProjectID,
				SessionID: "sess-3",
			},
		},
	})
	if resp.Aborted {
		t.Fatalf("non-blocking failures must not abort: %#v", resp)
	}
	if resp.Count != 2 {
		t.Fatalf("both actions should run: %#v", resp)
	}
	// Ensure async worker releases flock before TempDir cleanup.
	hookactions.WaitAsyncInlineForTest()
}
