package mcp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestRestoreSnapshots_ExistingNative_FootprintMismatch_ConflictPreserves(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	rel := cursor.ConfigRelPath
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	baseline := []byte("baseline-native\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), baseline, 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("atlas-written\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	// Concurrent replace: same bytes as Atlas write would have, but new inode.
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("foreign-native\n")
	if err := os.WriteFile(full, foreign, 0o644); err != nil {
		t.Fatal(err)
	}

	snap := mcp.TransactionSnapshot{
		Files: []mcp.FileSnapshot{{
			Adapter: mcp.AdapterCursor,
			RelPath: rel,
			Exists:  true,
			Raw:     baseline,
			Mode:    0o644,
		}},
	}
	snap.Footprint.AddFile(wrote)
	err = mcp.RestoreSnapshots(root, homePath, "id-native", snap)
	if err == nil || !strings.Contains(err.Error(), "rollback conflict") {
		t.Fatalf("expected identity conflict, got %v", err)
	}
	got, _ := os.ReadFile(full)
	if string(got) != string(foreign) {
		t.Fatalf("must preserve foreign: %q", got)
	}
}

func TestRestoreSnapshots_ExistingOwnership_FootprintMatch_RestoresBaseline(t *testing.T) {
	homePath := t.TempDir()
	pid := "id-own"
	rel := mcp.OwnershipRelPath(pid)
	baseline := []byte("schema_version: 1\nproject_id: id-own\nadapters: []\n")
	if err := os.MkdirAll(filepath.Join(homePath, filepath.Dir(filepath.FromSlash(rel))), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homePath, filepath.FromSlash(rel)), baseline, 0o600); err != nil {
		t.Fatal(err)
	}
	wrote, _, err := fsafety.AtomicWriteContainedTracked(homePath, rel, []byte("atlas-own\n"), 0o600, 0o700, ".atlas-own-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	snap := mcp.TransactionSnapshot{
		OwnershipExists: true,
		OwnershipRaw:    baseline,
		OwnershipMode:   0o600,
	}
	snap.Footprint.AddFile(wrote)
	if err := mcp.RestoreOwnershipSnapshotForTest(homePath, pid, snap); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(homePath, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(baseline) {
		t.Fatalf("restored %q", got)
	}
}
