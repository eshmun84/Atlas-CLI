package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestPersistConfigure_CorruptExistingConfigBlocked(t *testing.T) {
	t.Setenv(home.EnvAtlasHome, t.TempDir())
	root := t.TempDir()
	cfgPath := filepath.Join(root, filepath.FromSlash(config.FileConfig))
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("this is not valid yaml: [[[\n")
	if err := os.WriteFile(cfgPath, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	// Seed a fake MCP projection to ensure it stays untouched.
	mcpPath := filepath.Join(root, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatal(err)
	}
	mcpBytes := []byte(`{"mcpServers":{}}` + "\n")
	if err := os.WriteFile(mcpPath, mcpBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	docsPath := filepath.Join(root, "docs", "README.md")
	if err := os.MkdirAll(filepath.Dir(docsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	docsBytes := []byte("docs\n")
	if err := os.WriteFile(docsPath, docsBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "corrupt-demo", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinFilesystem)

	_, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp})
	if err == nil {
		t.Fatal("expected error for corrupt existing config")
	}
	gotCfg, err := os.ReadFile(cfgPath)
	if err != nil || string(gotCfg) != string(corrupt) {
		t.Fatalf("config bytes changed: %q err=%v", gotCfg, err)
	}
	gotMCP, err := os.ReadFile(mcpPath)
	if err != nil || string(gotMCP) != string(mcpBytes) {
		t.Fatalf("MCP mutated: %q err=%v", gotMCP, err)
	}
	gotDocs, err := os.ReadFile(docsPath)
	if err != nil || string(gotDocs) != string(docsBytes) {
		t.Fatalf("docs mutated: %q err=%v", gotDocs, err)
	}
}
