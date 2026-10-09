package mcp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
)

func TestRestoreSnapshots_AbsentNativeConfigInspectionFailure(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Symlink parent makes ContainedJoin fail during absence restore.
	if err := os.Remove(filepath.Join(root, ".cursor")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".cursor")); err != nil {
		t.Fatal(err)
	}
	snap := mcp.TransactionSnapshot{
		Files: []mcp.FileSnapshot{{
			Adapter: mcp.AdapterCursor,
			RelPath: cursor.ConfigRelPath,
			Exists:  false,
		}},
	}
	err := mcp.RestoreSnapshots(root, homePath, "abs-native", snap)
	if err == nil {
		t.Fatal("expected RestoreSnapshots error")
	}
	if !strings.Contains(err.Error(), "cursor") && !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("want containment/inspection failure, got %v", err)
	}
}

func TestRestoreSnapshots_OwnershipAbsentSymlinkFails(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	pid := "abs-own-sym"
	outside := t.TempDir()
	ext := filepath.Join(outside, "ownership.yaml")
	extBytes := []byte("EXTERNAL_MCP_OWN\n")
	if err := os.WriteFile(ext, extBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	mcpDir := filepath.Join(homePath, "projects", pid, "mcp")
	if err := os.MkdirAll(mcpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(mcpDir, mcp.FileOwnership)); err != nil {
		t.Fatal(err)
	}
	err := mcp.RestoreOwnershipSnapshotForTest(homePath, pid, mcp.TransactionSnapshot{OwnershipExists: false})
	if err == nil {
		t.Fatal("expected ownership rollback error")
	}
	got, readErr := os.ReadFile(ext)
	if readErr != nil || string(got) != string(extBytes) {
		t.Fatalf("external mutated: %q err=%v", got, readErr)
	}
	info, err := os.Lstat(filepath.Join(mcpDir, mcp.FileOwnership))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink must remain: info=%v err=%v", info, err)
	}
	_ = root
}

func TestRestoreSnapshots_OrdinaryAbsentSucceeds(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	snap := mcp.TransactionSnapshot{
		OwnershipExists: false,
		Files: []mcp.FileSnapshot{{
			Adapter: mcp.AdapterCursor,
			RelPath: cursor.ConfigRelPath,
			Exists:  false,
		}},
	}
	if err := mcp.RestoreSnapshots(root, homePath, "abs-ok", snap); err != nil {
		t.Fatalf("ordinary absent must succeed: %v", err)
	}
}
