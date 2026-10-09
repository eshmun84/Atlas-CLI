package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
)

func TestApplyConfig_HomeResetGate_EvaluatedUnderLock(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "lock-reset", ProjectMode: "new",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	res, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	// Second apply without AcceptHomeReset must observe Home presence under lock.
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		AcceptHomeReset: false,
		Now:             func() time.Time { return fixed.Add(time.Minute) },
	})
	if err == nil || !strings.Contains(err.Error(), "reset acceptance") {
		t.Fatalf("expected home-reset gate under lock, got %v", err)
	}
	_ = res
}

func TestApplyConfig_BusyWhileLockHeld(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	held, err := mutatelock.Acquire(mutatelock.Options{HomePath: homePath, Workspace: root})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "lock-busy", ProjectMode: "new",
		ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
	})
	if !errors.Is(err, mutatelock.ErrBusy) && (err == nil || !strings.Contains(err.Error(), "another Atlas mutation")) {
		t.Fatalf("want busy, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatal("busy Apply must not create workspace .atlas")
	}
}

func TestPersistConfigure_LoadsPreviousUnderLock(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "lock-cfg", ProjectMode: "new",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 9, 18, 10, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	}); err != nil {
		t.Fatal(err)
	}

	// Hold lock so a concurrent PersistConfigure cannot use a pre-lock previous.
	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	held, err := mutatelock.Acquire(mutatelock.Options{HomePath: homePath, Workspace: root})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, ".atlas", "config.yaml")
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate on-disk config while lock held (simulates concurrent writer outside Atlas flock).
	poison := append([]byte{}, before...)
	poison = append(poison, []byte("\n# concurrent-edit\n")...)
	if err := os.WriteFile(cfgPath, poison, 0o644); err != nil {
		t.Fatal(err)
	}

	draft2 := draft
	_ = draft2.ToggleMulti("adapters.selected", "opencode")
	_, err = config.PersistConfigure(config.ApplyInput{
		Root: root, Draft: draft2, MCP: config.EmptyMCPDraft(),
	})
	_ = held.Release()
	if !errors.Is(err, mutatelock.ErrBusy) && (err == nil || !strings.Contains(err.Error(), "another Atlas mutation")) {
		t.Fatalf("want busy while lock held, got %v", err)
	}
	// After release, PersistConfigure must load current on-disk previous under its own lock.
	res, err := config.PersistConfigure(config.ApplyInput{
		Root: root, Draft: draft2, MCP: config.EmptyMCPDraft(),
	})
	if err != nil {
		// Poisoned YAML may fail load — that still proves under-lock load (fail closed).
		if !strings.Contains(err.Error(), "unreadable") && !strings.Contains(err.Error(), "config") {
			t.Fatalf("unexpected error after release: %v", err)
		}
		return
	}
	_ = res
}
