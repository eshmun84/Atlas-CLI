package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
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

func TestPersistConfigure_MCPFailureRestoresConfig(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()

	// Init with Cursor + filesystem.
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "rb-cfg", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)
	fixed := time.Date(2026, 10, 8, 20, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcpDraft, Now: func() time.Time { return fixed },
	}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, config.FileConfig)
	beforeCfg, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cursorPath := filepath.Join(root, cursor.ConfigRelPath)
	beforeCursor, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}

	// Swap OpenCode projector for a failing one and attempt Configure with both adapters.
	orig := config.MCPProjectors()
	t.Cleanup(func() {
		for _, p := range orig {
			mcp.RegisterDefaultProjector(p)
		}
	})
	mcp.RegisterDefaultProjector(cursor.New())
	mcp.RegisterDefaultProjector(boomOpenCode{})

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "rb-cfg", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "cursor")
	_ = cfg.ToggleMulti("adapters.selected", "opencode")
	mcp2 := config.EmptyMCPDraft()
	mcp2.EnableBuiltin(config.MCPBuiltinFilesystem)
	mcp2.EnableBuiltin(config.MCPBuiltinGitHub)

	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcp2})
	if err == nil || !strings.Contains(err.Error(), "rolled back to baseline") {
		t.Fatalf("expected configure rollback error, got %v", err)
	}

	afterCfg, _ := os.ReadFile(configPath)
	if string(afterCfg) != string(beforeCfg) {
		t.Fatalf("config.yaml not restored")
	}
	afterCursor, _ := os.ReadFile(cursorPath)
	if string(afterCursor) != string(beforeCursor) {
		t.Fatalf("cursor mcp not restored:\n%s", afterCursor)
	}
	// GitHub must not remain projected; filesystem from prior init may remain as before.
	if strings.Contains(string(afterCursor), `"github"`) {
		t.Fatal("github leaked after failed configure")
	}
	if _, err := os.Stat(filepath.Join(root, opencode.ConfigRelPath)); err == nil {
		raw, _ := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
		if strings.Contains(string(raw), `"filesystem"`) || strings.Contains(string(raw), `"github"`) {
			t.Fatalf("opencode atlas entries after failed configure: %s", raw)
		}
	}
	found, rel, err := mcp.WorkspaceHasAtlasMCPBackup(root)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("workspace backup pollution: %s", rel)
	}
}

func TestNativeKeyCollisionRejectedOnConfigure(t *testing.T) {
	t.Parallel()
	// Display name differs from builtin "GitHub" but sanitizes to the same native key.
	draft := config.EmptyMCPDraft()
	_, err := draft.AddCustom("GitHub!!!", config.MCPTransportStreamableHTTP, "https://example.com/mcp", "", "")
	if err == nil || !strings.Contains(err.Error(), "native key collision") {
		t.Fatalf("expected collision on add, got %v", err)
	}
}

func TestOpenCodeNestedConfigurePreserved(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "nested-cfg", ProjectMode: "new", ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 8, 21, 0, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatal(err)
	}
	// Seed nested OpenCode MCP representation with a user entry.
	writeJSONFile(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp": map[string]any{
			"servers": map[string]any{
				"personal": map[string]any{"type": "local", "command": []any{"echo"}},
			},
		},
	})

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "nested-cfg", ProjectMode: "existing", ToolOpenCodeAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "opencode")
	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinGitHub)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcpDraft}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	mcpObj := doc["mcp"].(map[string]any)
	servers, ok := mcpObj["servers"].(map[string]any)
	if !ok || len(mcpObj) != 1 {
		t.Fatalf("nested shape lost: %s", raw)
	}
	if _, ok := servers["personal"]; !ok {
		t.Fatal("user entry lost")
	}
	if _, ok := servers["github"]; !ok {
		t.Fatal("github missing")
	}
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
