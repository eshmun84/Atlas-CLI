package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
)

func listTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInitApply_ExactTreeRollbackFreshProject(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	before := listTree(t, root)

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "tree-rb", ProjectMode: "new",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
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
		Now: func() time.Time { return time.Date(2026, 10, 8, 23, 0, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	after := listTree(t, root)
	if strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("tree mismatch after rollback\nbefore=%v\nafter=%v", before, after)
	}
}

func TestInitApply_ExactTreeRollbackPreservesPreexistingDirs(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(root, ".cursor", "rules", "user.mdc")
	if err := os.WriteFile(userFile, []byte("keep-user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := listTree(t, root)

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "tree-keep", ProjectMode: "new", ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
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
		Now: func() time.Time { return time.Date(2026, 10, 8, 23, 5, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	after := listTree(t, root)
	if strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("tree mismatch\nbefore=%v\nafter=%v", before, after)
	}
	got, err := os.ReadFile(userFile)
	if err != nil || string(got) != "keep-user\n" {
		t.Fatalf("user file: %q %v", got, err)
	}
}

func TestInitApply_RestoresGlobalHomeMirrorOnFailure(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	// Home does not exist yet — failed Init must remove the Home it created.
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "home-mirror", ProjectMode: "new", ToolOpenCodeAvailable: true,
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
		Now: func() time.Time { return time.Date(2026, 10, 8, 23, 10, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "Atlas Home mirror baseline") {
		t.Fatalf("expected home mirror rollback wording, got %v", err)
	}
	entries, _ := os.ReadDir(homeDir)
	if len(entries) != 0 {
		t.Fatalf("Atlas Home mirror must be cleared on failed Init: %v", entries)
	}
}
