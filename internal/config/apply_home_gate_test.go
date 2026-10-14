package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestApplyConfig_MissingHome_FailClosedNoCreate(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-home")
	t.Setenv(home.EnvAtlasHome, missing)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "no-home", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")

	beforeRoot := listNamesInit(t, root)
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("expected fail-closed when Atlas Home missing")
	}
	if !strings.Contains(err.Error(), home.RepairHomeHint) {
		t.Fatalf("expected actionable repair hint, got %v", err)
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Fatal("Init Apply must not create Atlas Home")
	}
	afterRoot := listNamesInit(t, root)
	if strings.Join(afterRoot, ",") != strings.Join(beforeRoot, ",") {
		t.Fatalf("workspace mutated: before=%v after=%v", beforeRoot, afterRoot)
	}
}

func TestApplyConfig_CorruptHome_FailClosed(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if err := os.WriteFile(filepath.Join(homeDir, "assets"), []byte("not-a-dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "corrupt-home", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")

	beforeHome := listNamesInit(t, homeDir)
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 10, 1, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("expected fail-closed for corrupt Home")
	}
	if !strings.Contains(err.Error(), home.RepairHomeHint) {
		t.Fatalf("expected actionable repair hint, got %v", err)
	}
	afterHome := listNamesInit(t, homeDir)
	if strings.Join(afterHome, ",") != strings.Join(beforeHome, ",") {
		t.Fatalf("Home mutated by Init: before=%v after=%v", beforeHome, afterHome)
	}
	data, err := os.ReadFile(filepath.Join(homeDir, "assets"))
	if err != nil || string(data) != "not-a-dir\n" {
		t.Fatalf("corrupt fixture mutated: %q %v", data, err)
	}
}
