package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
)

func TestLoadOwnership_SymlinkLeaf_RefusesExternalAndBlocksReconcile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	outside := t.TempDir()

	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	mcpDir := filepath.Join(homeDir, "projects", pid, "mcp")
	if err := os.MkdirAll(mcpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	external := []byte("schema_version: 1\nproject_id: forged\nadapters:\n  - adapter: cursor\n    entries:\n      - definition_id: filesystem\n        native_key: filesystem\n")
	extPath := filepath.Join(outside, "ownership.yaml")
	if err := os.WriteFile(extPath, external, 0o600); err != nil {
		t.Fatal(err)
	}
	ownPath := filepath.Join(mcpDir, "ownership.yaml")
	if err := os.Symlink(extPath, ownPath); err != nil {
		t.Fatal(err)
	}

	_, err = mcp.LoadOwnership(homeDir, pid)
	if err == nil {
		t.Fatal("LoadOwnership must refuse symlink leaf ownership")
	}

	nativePath := filepath.Join(root, cursor.ConfigRelPath)
	if err := os.MkdirAll(filepath.Dir(nativePath), 0o755); err != nil {
		t.Fatal(err)
	}
	nativeBefore := []byte(`{"mcpServers":{}}`)
	if err := os.WriteFile(nativePath, nativeBefore, 0o644); err != nil {
		t.Fatal(err)
	}

	doc := config.ProjectDocument{
		Project:  config.ProjectPersist{Name: "demo", Mode: "existing"},
		Adapters: config.AdaptersPersist{Selected: []string{"cursor"}},
	}
	draft := doc.ToMCPDraft()
	if !draft.EnableBuiltin(config.MCPBuiltinFilesystem) {
		t.Fatal("enable filesystem")
	}

	res, recErr := config.ReconcileMCPProjections(root, doc, draft)
	if recErr == nil && !res.Blocked {
		t.Fatalf("expected BLOCK on unsafe ownership, res=%#v err=%v", res, recErr)
	}

	nativeAfter, err := os.ReadFile(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(nativeAfter) != string(nativeBefore) {
		t.Fatalf("native MCP mutated:\nbefore=%s\nafter=%s", nativeBefore, nativeAfter)
	}

	gotExt, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotExt) != string(external) {
		t.Fatal("external ownership document was mutated")
	}
	info, err := os.Lstat(ownPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("ownership leaf must remain symlink: info=%v err=%v", info, err)
	}
}

func TestLoadOwnership_SymlinkParent_Refuses(t *testing.T) {
	homeDir := t.TempDir()
	outside := t.TempDir()
	pid := "own-parent"
	if err := os.MkdirAll(filepath.Join(homeDir, "projects", pid), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "mcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	extPath := filepath.Join(outside, "mcp", "ownership.yaml")
	if err := os.WriteFile(extPath, []byte("schema_version: 1\nproject_id: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "mcp"), filepath.Join(homeDir, "projects", pid, "mcp")); err != nil {
		t.Fatal(err)
	}
	_, err := mcp.LoadOwnership(homeDir, pid)
	if err == nil {
		t.Fatal("LoadOwnership must refuse symlink parent")
	}
}

func TestLoadOwnership_Missing_EmptyDoc(t *testing.T) {
	homeDir := t.TempDir()
	doc, err := mcp.LoadOwnership(homeDir, "missing-proj")
	if err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != mcp.OwnershipSchemaVersion || len(doc.Adapters) != 0 {
		t.Fatalf("want empty ownership, got %#v", doc)
	}
}

func TestLoadOwnership_RegularFile_OK(t *testing.T) {
	homeDir := t.TempDir()
	pid := "own-ok"
	if err := os.MkdirAll(filepath.Join(homeDir, "projects", pid, "mcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("schema_version: 1\nproject_id: own-ok\nadapters: []\n")
	if err := os.WriteFile(mcp.OwnershipPath(homeDir, pid), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := mcp.LoadOwnership(homeDir, pid)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ProjectID != pid || doc.SchemaVersion != 1 {
		t.Fatalf("got %#v", doc)
	}
}
