package codeintel_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
)

func TestRefresh_FreshnessDecisionUnderLock(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	stub := &refreshStub{cap: codeintel.Capability{
		Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0",
	}}
	svc := codeintel.NewService(stub)
	fixed := time.Date(2026, 10, 9, 18, 40, 0, 0, time.UTC)

	out, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{
		Root: root, ProjectName: "demo", Now: func() time.Time { return fixed },
	})
	if err != nil || out.Noop {
		t.Fatalf("initial %#v err=%v", out, err)
	}

	// Would be noop if decided before lock; change source so locked recompute must refresh.
	mustWrite(t, filepath.Join(root, "a.go"), "package a\nfunc Changed(){}\n")
	stub.calls = 0
	out2, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{
		Root: root, ProjectName: "demo", Now: func() time.Time { return fixed.Add(time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out2.Noop || stub.calls == 0 {
		t.Fatalf("expected locked recompute to see stale source; noop=%v calls=%d", out2.Noop, stub.calls)
	}
}

func TestRefresh_BusyWhileHomeLockHeld(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")

	held, err := mutatelock.Acquire(mutatelock.Options{HomePath: homeDir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()

	stub := &refreshStub{cap: codeintel.Capability{
		Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0",
	}}
	svc := codeintel.NewService(stub)
	_, err = svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo"})
	if !errors.Is(err, mutatelock.ErrBusy) && (err == nil || !strings.Contains(err.Error(), "another Atlas mutation")) {
		t.Fatalf("want busy, got %v", err)
	}
	if stub.calls != 0 {
		t.Fatal("provider must not run while lock busy")
	}
	if entries, _ := os.ReadDir(filepath.Join(homeDir, "projects")); len(entries) != 0 {
		t.Fatal("busy refresh must not create project storage")
	}
}
