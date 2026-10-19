package hookactions_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"gopkg.in/yaml.v3"
)

func TestContextEconomy_MissingCreate(t *testing.T) {
	ref := seedInitializedProject(t, "ce-missing")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	out, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home)
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != hooks.OutcomeOK || out.Noop {
		t.Fatalf("want create ok, got %#v", out)
	}
	index := atlascontext.IndexPath(ref.Home, ref.ProjectID)
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("index missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ref.Root, "context")); !os.IsNotExist(err) {
		t.Fatal("must not write context/ into product repo")
	}
}

func TestContextEconomy_StaleUpdate(t *testing.T) {
	ref := seedInitializedProject(t, "ce-stale")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	if _, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ref.Root, "main.go"), "package main\n")

	out, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home)
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != hooks.OutcomeOK || out.Noop {
		t.Fatalf("want stale update, got %#v", out)
	}
	if out.State != atlascontext.StatusPresent {
		t.Fatalf("post-state want present, got %#v", out)
	}
}

func TestContextEconomy_FreshNoop(t *testing.T) {
	ref := seedInitializedProject(t, "ce-fresh")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	if _, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(atlascontext.IndexPath(ref.Home, ref.ProjectID))
	if err != nil {
		t.Fatal(err)
	}

	out, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Noop || out.Outcome != hooks.OutcomeOK {
		t.Fatalf("want fresh noop, got %#v", out)
	}
	after, err := os.ReadFile(atlascontext.IndexPath(ref.Home, ref.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("fresh maintain must not rewrite index")
	}
}

func TestContextEconomy_UnreadableError(t *testing.T) {
	ref := seedInitializedProject(t, "ce-unreadable")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	ctxDir := filepath.Join(ref.Home, "projects", ref.ProjectID, "context")
	if err := os.MkdirAll(ctxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	ext := filepath.Join(outside, "index.yaml")
	write(t, ext, "schema_version: 1\nfingerprint: deadbeef\n")
	if err := os.Symlink(ext, filepath.Join(ctxDir, "index.yaml")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ctxDir, "capsule.md"), "# capsule\n")

	out, err := hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home)
	if err == nil || out.Outcome != hooks.OutcomeFailed {
		t.Fatalf("want controlled failure, got out=%#v err=%v", out, err)
	}
	got, _ := os.ReadFile(ext)
	if string(got) != "schema_version: 1\nfingerprint: deadbeef\n" {
		t.Fatal("external index must not be rewritten")
	}
}

func TestContextEconomy_Timeout(t *testing.T) {
	ref := seedInitializedProject(t, "ce-timeout")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	out, err := hookactions.MaintainContextEconomy(ctx, ref.Root, ref.Home)
	if err == nil || out.Outcome != hooks.OutcomeCancelled {
		t.Fatalf("want cancelled, got out=%#v err=%v", out, err)
	}
}

func TestContextEconomy_Concurrent(t *testing.T) {
	ref := seedInitializedProject(t, "ce-concurrent")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			for attempt := 0; attempt < 8; attempt++ {
				_, err = hookactions.MaintainContextEconomy(context.Background(), ref.Root, ref.Home)
				if err == nil {
					errs <- nil
					return
				}
				// Contended mutate lock is recoverable; retry briefly.
				time.Sleep(20 * time.Millisecond)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent maintain: %v", err)
		}
	}
	if _, err := os.Stat(atlascontext.IndexPath(ref.Home, ref.ProjectID)); err != nil {
		t.Fatal(err)
	}
}

func TestContextEconomy_RunSkippedWhenTriggersDisabled(t *testing.T) {
	hookactions.SetTriggersEnabled(false)
	t.Cleanup(func() { hookactions.SetTriggersEnabled(true) })
	hookactions.ResetAdmitStateForTest()
	ref := seedInitializedProject(t, "ce-skip")
	act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
	res, err := act.Run(context.Background(), hooks.Event{
		ID: "e1", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s1"},
	})
	if err != nil || res.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("got %#v err=%v", res, err)
	}
	if _, err := os.Stat(atlascontext.IndexPath(ref.Home, ref.ProjectID)); !os.IsNotExist(err) {
		t.Fatal("disabled triggers must not write")
	}
}

func TestContextEconomy_RunAdmitsWhenTriggersEnabled(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	done := make(chan struct{})
	hookactions.SetSpawner(func(ref hookactions.ProjectRef) error {
		go func() {
			defer close(done)
			_ = hookactions.InlineSpawner(ref)
		}()
		return nil
	})
	t.Cleanup(func() {
		hookactions.SetSpawner(nil)
		hookactions.ResetAdmitStateForTest()
	})
	ref := seedInitializedProject(t, "ce-enabled")
	write(t, filepath.Join(ref.Root, "go.mod"), "module demo\n")
	act := &hookactions.ContextEconomyAction{Resolve: fixedResolve(ref)}
	res, err := act.Run(context.Background(), hooks.Event{
		ID: "e-admit-1", Type: hooks.EventSessionStart, OccurredAt: time.Now().UTC(),
		Payload: hooks.Payload{ProjectID: ref.ProjectID, SessionID: "s1"},
	})
	if err != nil || res.Outcome != hooks.OutcomeOK {
		t.Fatalf("got %#v err=%v", res, err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("worker timeout")
	}
	if _, err := os.Stat(atlascontext.IndexPath(ref.Home, ref.ProjectID)); err != nil {
		t.Fatal("async spawner must create CE index")
	}
}

type projectFixture struct {
	Root, Home, ProjectID, Name string
	State                       config.StateDocument
}

func seedInitializedProject(t *testing.T, name string) projectFixture {
	t.Helper()
	root := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)

	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: name,
		ProjectMode: "existing",
	}), config.EmptyMCPDraft())
	doc.Adapters.Selected = []string{"cursor"}
	data, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	state := config.StateDocument{
		SchemaVersion: config.CurrentProjectSchemaVersion,
		Initialized:   true,
		ProjectName:   name,
		AppliedAt:     "2026-10-17T00:00:00Z",
	}
	stateData, err := yaml.Marshal(&state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileState)), stateData, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := home.ProjectID(root, name)
	if err != nil {
		t.Fatal(err)
	}
	return projectFixture{Root: root, Home: homeDir, ProjectID: id, Name: name, State: state}
}

func fixedResolve(ref projectFixture) hookactions.ResolveFunc {
	return func() (hookactions.ProjectRef, error) {
		return hookactions.ProjectRef{
			Root: ref.Root, Home: ref.Home, ProjectID: ref.ProjectID, Name: ref.Name, State: ref.State,
		}, nil
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
