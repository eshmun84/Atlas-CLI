package home_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

type boomOpenCode struct {
	opencode.Projector
}

func (boomOpenCode) ApplyMCPProjection(string, []mcp.NativeEntry, []string, []string) error {
	return fmt.Errorf("induced opencode failure")
}

func TestMirrorSnapshot_RestoresPriorModes(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"projects", "assets"} {
		if err := os.MkdirAll(filepath.Join(homeDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(homeDir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(homeDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "mode-rb", ProjectMode: "new", ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	orig := config.MCPProjectors()
	t.Cleanup(func() {
		for _, p := range orig {
			mcp.RegisterDefaultProjector(p)
		}
	})
	mcp.RegisterDefaultProjector(cursor.New())
	mcp.RegisterDefaultProjector(boomOpenCode{})
	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcpDraft,
		Now: func() time.Time { return time.Date(2026, 10, 8, 23, 40, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("expected init failure")
	}
	for _, path := range []string{homeDir, filepath.Join(homeDir, "projects"), filepath.Join(homeDir, "assets")} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("%s mode=%04o want 0755", path, info.Mode().Perm())
		}
	}
}

func TestMirrorSnapshot_RemovesNewlyCreatedNestedAssetDirs(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	// assets/ exists; assets/agents/ does not.
	if _, err := os.Lstat(filepath.Join(homeDir, "assets", "agents")); !os.IsNotExist(err) {
		t.Fatalf("precondition: assets/agents must be absent: %v", err)
	}

	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "nested-rb", ProjectMode: "new", ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	orig := config.MCPProjectors()
	t.Cleanup(func() {
		for _, p := range orig {
			mcp.RegisterDefaultProjector(p)
		}
	})
	mcp.RegisterDefaultProjector(boomOpenCode{})
	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcpDraft,
		Now: func() time.Time { return time.Date(2026, 10, 8, 23, 45, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("expected init failure")
	}
	if _, err := os.Lstat(filepath.Join(homeDir, "assets")); err != nil {
		t.Fatalf("assets/ must remain: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(homeDir, "assets", "agents")); !os.IsNotExist(err) {
		t.Fatalf("newly created empty assets/agents/ must be removed: %v", err)
	}
}

func TestCaptureRestoreMirrorSnapshot_Direct(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(homeDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	snap, err := home.CaptureMirrorSnapshot(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.HomeExisted || snap.HomeMode != 0o755 {
		t.Fatalf("snap home: existed=%v mode=%04o", snap.HomeExisted, snap.HomeMode)
	}
	if !snap.LayoutDirsExisted["assets"] || snap.LayoutDirModes["assets"] != 0o755 {
		t.Fatalf("assets snap: %#v modes=%v", snap.LayoutDirsExisted["assets"], snap.LayoutDirModes)
	}
	if snap.NestedDirsExisted["assets/agents"] {
		t.Fatal("assets/agents should not have existed")
	}

	ensured, err := home.EnsureAndMirror(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snap.Footprint = ensured.Footprint
	info, _ := os.Lstat(homeDir)
	if info.Mode().Perm() != home.DirPermHome {
		t.Fatalf("EnsureAndMirror should harden home to 0700, got %04o", info.Mode().Perm())
	}
	if err := home.RestoreMirrorSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Lstat(homeDir)
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("home mode not restored: %04o", info.Mode().Perm())
	}
	assetsInfo, _ := os.Lstat(filepath.Join(homeDir, "assets"))
	if assetsInfo.Mode().Perm() != 0o755 {
		t.Fatalf("assets mode not restored: %04o", assetsInfo.Mode().Perm())
	}
	if _, err := os.Lstat(filepath.Join(homeDir, "assets", "agents")); !os.IsNotExist(err) {
		t.Fatal("nested assets/agents should be removed when empty")
	}
}
