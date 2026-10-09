package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestRemoveConflict_Directory_RefusesRecursiveWipe(t *testing.T) {
	root := t.TempDir()
	rel := ".atlas/weird"
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(full, "child.txt")
	if err := os.WriteFile(child, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runtime.RemoveConflictForTest(root, rel)
	if err == nil || !strings.Contains(err.Error(), "refusing recursive directory wipe") {
		t.Fatalf("expected refuse, got %v", err)
	}
	if _, err := os.Lstat(child); err != nil {
		t.Fatal("directory children must be preserved")
	}
}

func TestRemoveConflict_RegularFile_RemovesWithDeletionFootprint(t *testing.T) {
	root := t.TempDir()
	rel := ".atlas/conflict.txt"
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseline, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil || !baseline.Exists {
		t.Fatalf("baseline: %#v %v", baseline, err)
	}
	var fp fsafety.TransactionFootprint
	if err := runtime.RemoveConflictTrackedForTest(root, baseline, &fp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(full); !os.IsNotExist(err) {
		t.Fatal("regular file must be removed")
	}
	if fp.DeletedByRel(rel) == nil {
		t.Fatal("deletion must be recorded in footprint")
	}
}

func TestRestoreFileSnapshot_MissingBaselineWithoutDeletion_Conflict(t *testing.T) {
	root := t.TempDir()
	rel := "gone.txt"
	if err := fsafety.AtomicWriteContained(root, rel, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	err = fsafety.RestoreFileSnapshot(root, snap, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "deletion footprint") {
		t.Fatalf("expected deletion-proof conflict, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatal("must not recreate without deletion proof")
	}
}
