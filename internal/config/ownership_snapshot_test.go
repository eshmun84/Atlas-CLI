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
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
)

func TestInitApply_Ownership0640RestoredOnRollback(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "own-init-mode", ProjectMode: "new",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	fixed := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	res, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	ownPath := filepath.Join(res.HomePath, "projects", res.ProjectID, "mcp", "ownership.yaml")
	baseline := []byte("schema_version: 1\nproject_id: " + res.ProjectID + "\nadapters: []\n")
	if err := os.WriteFile(ownPath, baseline, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ownPath, 0o640); err != nil {
		t.Fatal(err)
	}

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
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcpDraft, AcceptHomeReset: true,
		Now: func() time.Time { return fixed.Add(time.Minute) },
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	after, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(baseline) {
		t.Fatalf("ownership bytes not restored:\n%s", after)
	}
	info, err := os.Lstat(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("ownership mode=%04o, want 0640", info.Mode().Perm())
	}
}

func TestInitApply_OwnershipSymlinkAbortsZeroMutation(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	pid, err := home.ProjectID(root, "own-init-sym")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	extPath := filepath.Join(outside, "ownership.yaml")
	extBytes := []byte("EXTERNAL_OWNERSHIP\n")
	if err := os.WriteFile(extPath, extBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	mcpDir := filepath.Join(homeDir, "projects", pid, "mcp")
	if err := os.MkdirAll(mcpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, filepath.Join(mcpDir, "ownership.yaml")); err != nil {
		t.Fatal(err)
	}
	beforeRoot := listNamesInit(t, root)
	beforeExt, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "own-init-sym", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: true,
		Now: func() time.Time { return time.Date(2026, 10, 9, 10, 5, 0, 0, time.UTC) },
	})
	if err == nil || !(strings.Contains(err.Error(), "ownership") || strings.Contains(err.Error(), "symlink")) {
		t.Fatalf("expected ownership symlink snapshot abort, got %v", err)
	}
	afterRoot := listNamesInit(t, root)
	if strings.Join(afterRoot, ",") != strings.Join(beforeRoot, ",") {
		t.Fatalf("workspace mutated: before=%v after=%v", beforeRoot, afterRoot)
	}
	afterExt, err := os.ReadFile(extPath)
	if err != nil || string(afterExt) != string(beforeExt) {
		t.Fatalf("external ownership mutated: %q err=%v", afterExt, err)
	}
	info, err := os.Lstat(filepath.Join(mcpDir, "ownership.yaml"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("ownership leaf must remain symlink: info=%v err=%v", info, err)
	}
}

func TestPersistConfigure_Ownership0640RestoredOnRollback(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "own-cfg-mode", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 9, 10, 10, 0, 0, time.UTC)
	res, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	ownPath := filepath.Join(res.HomePath, "projects", res.ProjectID, "mcp", "ownership.yaml")
	before, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ownPath, 0o640); err != nil {
		t.Fatal(err)
	}

	orig := config.MCPProjectors()
	t.Cleanup(func() {
		for _, p := range orig {
			mcp.RegisterDefaultProjector(p)
		}
	})
	mcp.RegisterDefaultProjector(cursor.New())
	mcp.RegisterDefaultProjector(boomOpenCode{})

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "own-cfg-mode", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "cursor")
	_ = cfg.ToggleMulti("adapters.selected", "opencode")
	mcp2 := config.EmptyMCPDraft()
	mcp2.EnableBuiltin(config.MCPBuiltinFilesystem)
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcp2})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected configure rollback, got %v", err)
	}
	after, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("ownership bytes not restored:\nbefore=%s\nafter=%s", before, after)
	}
	info, err := os.Lstat(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("ownership mode=%04o, want 0640", info.Mode().Perm())
	}
}

func TestPersistConfigure_OwnershipSymlinkAbortsZeroMutation(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "own-cfg-sym", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 9, 10, 15, 0, 0, time.UTC)
	res, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	ownPath := filepath.Join(res.HomePath, "projects", res.ProjectID, "mcp", "ownership.yaml")
	outside := t.TempDir()
	extPath := filepath.Join(outside, "ownership.yaml")
	extBytes := []byte("EXTERNAL_CFG_OWN\n")
	if err := os.WriteFile(extPath, extBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ownPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, ownPath); err != nil {
		t.Fatal(err)
	}
	beforeCfg, err := os.ReadFile(filepath.Join(root, config.FileConfig))
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "own-cfg-sym", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "cursor")
	mcp2 := config.EmptyMCPDraft()
	mcp2.EnableBuiltin(config.MCPBuiltinFilesystem)
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcp2})
	if err == nil || !(strings.Contains(err.Error(), "ownership") || strings.Contains(err.Error(), "symlink")) {
		t.Fatalf("expected ownership symlink snapshot abort, got %v", err)
	}
	afterCfg, err := os.ReadFile(filepath.Join(root, config.FileConfig))
	if err != nil || string(afterCfg) != string(beforeCfg) {
		t.Fatalf("config mutated on snapshot abort")
	}
	afterExt, err := os.ReadFile(extPath)
	if err != nil || string(afterExt) != string(extBytes) {
		t.Fatalf("external ownership mutated: %q err=%v", afterExt, err)
	}
	info, err := os.Lstat(ownPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("ownership leaf must remain symlink: info=%v err=%v", info, err)
	}
}
