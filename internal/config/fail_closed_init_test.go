package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func TestApplyConfig_InspectProjectDataFailure_ZeroMutation(t *testing.T) {
	// Symlink project root makes InspectProjectDataPresence fail closed without
	// home production test setters.
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	pid, err := home.ProjectID(root, "gate-fail")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, home.ProjectRoot(homeDir, pid)); err != nil {
		t.Fatal(err)
	}

	beforeHome := listNamesInit(t, homeDir)
	beforeRoot := listNamesInit(t, root)
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "gate-fail", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 8, 23, 50, 0, 0, time.UTC) },
	})
	// Symlink project root fails closed on inspect and/or contained ownership read.
	if err == nil || !(strings.Contains(err.Error(), "inspect home project data") ||
		strings.Contains(err.Error(), "ownership") ||
		strings.Contains(err.Error(), "symlink")) {
		t.Fatalf("expected fail-closed on symlink project Home, got %v", err)
	}
	afterHome := listNamesInit(t, homeDir)
	afterRoot := listNamesInit(t, root)
	if strings.Join(afterHome, ",") != strings.Join(beforeHome, ",") {
		t.Fatalf("home mutated: before=%v after=%v", beforeHome, afterHome)
	}
	if strings.Join(afterRoot, ",") != strings.Join(beforeRoot, ",") {
		t.Fatalf("workspace mutated: before=%v after=%v", beforeRoot, afterRoot)
	}
}

func TestApplyConfig_MirrorSnapshotFailure_BeforeEnsureAndMirror(t *testing.T) {
	// Pre-existing Home with layout name "assets" as a file forces CaptureMirrorSnapshot
	// to fail closed before EnsureAndMirror.
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if err := os.WriteFile(filepath.Join(homeDir, "assets"), []byte("not-a-dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()

	beforeHome := listNamesInit(t, homeDir)
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "mirror-fail", ProjectMode: "new", ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	orig := config.MCPProjectors()
	t.Cleanup(func() {
		for _, p := range orig {
			mcp.RegisterDefaultProjector(p)
		}
	})
	mcp.RegisterDefaultProjector(opencode.New())
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 8, 23, 55, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("expected snapshot abort, got %v", err)
	}
	afterHome := listNamesInit(t, homeDir)
	if strings.Join(afterHome, ",") != strings.Join(beforeHome, ",") {
		t.Fatalf("EnsureAndMirror must not run: before=%v after=%v", beforeHome, afterHome)
	}
	// assets file must remain untouched.
	data, err := os.ReadFile(filepath.Join(homeDir, "assets"))
	if err != nil || string(data) != "not-a-dir\n" {
		t.Fatalf("assets fixture mutated: %q %v", data, err)
	}
}

func listNamesInit(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
