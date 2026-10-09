package fsafety_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestRestoreFileSnapshot_FootprintMatch_RestoresBaselineFile(t *testing.T) {
	root := t.TempDir()
	rel := "f.txt"
	if err := fsafety.AtomicWriteContained(root, rel, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	wrote, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("new\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := fsafety.RestoreFileSnapshot(root, snap, &wrote, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, rel))
	if string(got) != "base\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRestoreFileSnapshot_FootprintMatch_RemovesCreatedFile(t *testing.T) {
	root := t.TempDir()
	rel := "created.txt"
	snap := fsafety.FileSnapshot{Rel: rel, Exists: false}
	wrote, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("x\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := fsafety.RestoreFileSnapshot(root, snap, &wrote, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatal("expected removed")
	}
}

func TestRestoreFileSnapshot_FootprintMismatch_ConflictNoOverwrite(t *testing.T) {
	root := t.TempDir()
	rel := "f.txt"
	if err := fsafety.AtomicWriteContained(root, rel, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, _ := fsafety.CaptureFileSnapshot(root, rel)
	wrote, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("atlas\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = fsafety.RestoreFileSnapshot(root, snap, &wrote, nil)
	if err == nil || !strings.Contains(err.Error(), "rollback conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, rel))
	if string(got) != "foreign\n" {
		t.Fatalf("must preserve foreign: %q", got)
	}
}

func TestRestoreFileSnapshot_FootprintMismatch_ConflictNoRemove(t *testing.T) {
	root := t.TempDir()
	rel := "created.txt"
	snap := fsafety.FileSnapshot{Rel: rel, Exists: false}
	wrote, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("atlas\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = fsafety.RestoreFileSnapshot(root, snap, &wrote, nil)
	if err == nil || !strings.Contains(err.Error(), "rollback conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, rel))
	if string(got) != "foreign\n" {
		t.Fatalf("must preserve: %q", got)
	}
}

func TestRestoreFileSnapshot_ByteIdenticalReplacedCreated_Conflict(t *testing.T) {
	root := t.TempDir()
	rel := "created.txt"
	snap := fsafety.FileSnapshot{Rel: rel, Exists: false}
	wrote, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("same\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(root, rel)
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = fsafety.RestoreFileSnapshot(root, snap, &wrote, nil)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity conflict, got %v", err)
	}
	got, _ := os.ReadFile(full)
	if string(got) != "same\n" {
		t.Fatalf("must preserve: %q", got)
	}
}

func TestRestoreFileSnapshot_ByteIdenticalReplacedOverwritten_Conflict(t *testing.T) {
	root := t.TempDir()
	rel := "f.txt"
	if err := fsafety.AtomicWriteContained(root, rel, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, _ := fsafety.CaptureFileSnapshot(root, rel)
	wrote, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("atlas\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(root, rel)
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("atlas\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = fsafety.RestoreFileSnapshot(root, snap, &wrote, nil)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity conflict, got %v", err)
	}
	got, _ := os.ReadFile(full)
	if !bytes.Equal(got, []byte("atlas\n")) {
		t.Fatalf("must preserve: %q", got)
	}
}

func TestRemoveRegularFileTracked_AndRecreateWithDeletionProof(t *testing.T) {
	root := t.TempDir()
	rel := "kept.txt"
	if err := fsafety.AtomicWriteContained(root, rel, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := fsafety.RemoveRegularFileTracked(root, snap)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Rel != rel {
		t.Fatalf("DeletedFile rel=%q", deleted.Rel)
	}
	if err := fsafety.RestoreFileSnapshot(root, snap, nil, &deleted); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, rel))
	if string(got) != "base\n" {
		t.Fatalf("recreated %q", got)
	}
}

func TestRemoveRegularFileTracked_DisappearBetweenVerifyAndRemove_ConflictNoProof(t *testing.T) {
	root := t.TempDir()
	rel := "race.txt"
	if err := fsafety.AtomicWriteContained(root, rel, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	// Concurrent actor removes the object after verification; Atlas Remove sees missing.
	fsafety.SetRemoveForTest(func(path string) error {
		_ = os.Remove(path)
		return os.ErrNotExist
	})
	t.Cleanup(func() { fsafety.SetRemoveForTest(nil) })

	deleted, err := fsafety.RemoveRegularFileTracked(root, snap)
	if err == nil || !strings.Contains(err.Error(), "rollback conflict") {
		t.Fatalf("expected conflict, got deleted=%+v err=%v", deleted, err)
	}
	if deleted.Rel != "" || deleted.Info != nil || len(deleted.Expected) != 0 {
		t.Fatalf("must not return DeletedFile proof: %+v", deleted)
	}
}

func TestSafeMkdirAllCreated_PartialComponent_ReturnsSuccessfulDirs(t *testing.T) {
	root := t.TempDir()
	calls := 0
	fsafety.SetMkdirForTest(func(path string, mode os.FileMode) error {
		calls++
		if calls == 1 {
			return os.Mkdir(path, mode)
		}
		return os.ErrPermission
	})
	t.Cleanup(func() { fsafety.SetMkdirForTest(nil) })

	dirs, err := fsafety.SafeMkdirAllCreated(root, "a/b", 0o755)
	if err == nil {
		t.Fatal("expected later-component failure")
	}
	foundA := false
	for _, d := range dirs {
		if d.Rel == "a" {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Fatalf("expected CreatedDir for successful component a, got %#v", dirs)
	}
	if _, err := os.Lstat(filepath.Join(root, "a")); err != nil {
		t.Fatal("successful component must exist on disk")
	}
}

func TestRestoreFileSnapshot_AbsentDir_ConflictNoRemoveAll(t *testing.T) {
	root := t.TempDir()
	rel := "was-file"
	snap := fsafety.FileSnapshot{Rel: rel, Exists: false}
	if err := os.Mkdir(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Fake footprint pointing at a different object — verify never RemoveAll.
	err := fsafety.RestoreFileSnapshot(root, snap, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "rollback conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, rel, "child")); err != nil {
		t.Fatal("child must survive (no RemoveAll)")
	}
}

func TestRestoreFileSnapshot_Symlink_Conflict(t *testing.T) {
	root := t.TempDir()
	rel := "link"
	outside := t.TempDir()
	target := filepath.Join(outside, "t")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	snap := fsafety.FileSnapshot{Rel: rel, Exists: false}
	err := fsafety.RestoreFileSnapshot(root, snap, &fsafety.WrittenFile{Rel: rel, Expected: []byte("x"), Mode: 0o644}, nil)
	if err == nil {
		t.Fatal("expected conflict/error")
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatal("symlink target must remain")
	}
}

func TestCreatedDirs_OnlyRecordedDirsRemovedWhenEmpty(t *testing.T) {
	root := t.TempDir()
	dirs, err := fsafety.SafeMkdirAllCreated(root, "a/b", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	fp := fsafety.TransactionFootprint{}
	fp.MergeDirs(dirs)
	// Untracked sibling must remain.
	if err := os.Mkdir(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsafety.RestoreCreatedDirs(root, fp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "a", "b")); !os.IsNotExist(err) {
		t.Fatal("b must be removed")
	}
	if _, err := os.Lstat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Fatal("a must be removed")
	}
	if _, err := os.Lstat(filepath.Join(root, "other")); err != nil {
		t.Fatal("untracked other must remain")
	}
}

func TestCreatedDirs_BaselineAbsentWithoutFootprint_NotRemoved(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "ghost"), 0o755); err != nil {
		t.Fatal(err)
	}
	fp := fsafety.TransactionFootprint{} // empty — no authority
	if err := fsafety.RestoreCreatedDirs(root, fp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "ghost")); err != nil {
		t.Fatal("must not remove without footprint")
	}
}

func TestCreatedDirs_NonEmpty_ConflictPreserves(t *testing.T) {
	root := t.TempDir()
	dirs, err := fsafety.SafeMkdirAllCreated(root, "keep", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep", "x"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := fsafety.TransactionFootprint{}
	fp.MergeDirs(dirs)
	err = fsafety.RestoreCreatedDirs(root, fp)
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty conflict, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "keep", "x")); err != nil {
		t.Fatal("must preserve content")
	}
}

func TestCreatedDirs_RecreatedEmpty_IdentityConflict(t *testing.T) {
	root := t.TempDir()
	dirs, err := fsafety.SafeMkdirAllCreated(root, "d", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(root, "d")
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(full, 0o755); err != nil {
		t.Fatal(err)
	}
	fp := fsafety.TransactionFootprint{}
	fp.MergeDirs(dirs)
	err = fsafety.RestoreCreatedDirs(root, fp)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity conflict, got %v", err)
	}
	if _, err := os.Lstat(full); err != nil {
		t.Fatal("recreated dir must remain")
	}
}

func TestCreatedDirs_NestedDeepestFirst_ChildConflictBlocksParent(t *testing.T) {
	root := t.TempDir()
	dirs, err := fsafety.SafeMkdirAllCreated(root, "p/c", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "p", "c", "foreign"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := fsafety.TransactionFootprint{}
	fp.MergeDirs(dirs)
	err = fsafety.RestoreCreatedDirs(root, fp)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if !strings.Contains(err.Error(), "p/c") {
		t.Fatalf("want child conflict: %v", err)
	}
	if !strings.Contains(err.Error(), "skipped destructive remove") && !strings.Contains(err.Error(), "p:") {
		// parent skip message
		if !strings.Contains(err.Error(), "child conflict") {
			t.Fatalf("want parent blocked: %v", err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "p")); err != nil {
		t.Fatal("parent must remain when child conflicts")
	}
	if _, err := os.Lstat(filepath.Join(root, "p", "c", "foreign")); err != nil {
		t.Fatal("foreign content preserved")
	}
}

func TestRemoveEmptyCreatedDir_Symlink_Conflict(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "d")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(root, "d"))
	if err != nil {
		t.Fatal(err)
	}
	err = fsafety.RemoveEmptyCreatedDir(root, fsafety.CreatedDir{Rel: "d", Info: info})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink conflict, got %v", err)
	}
	if _, err := os.Lstat(outside); err != nil {
		t.Fatal("target must remain")
	}
}

func TestRestoreDirMode_IdentityMismatch_ConflictNoChmod(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "d")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseline, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	err = fsafety.RestoreDirMode(dir, baseline, 0o755)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity conflict, got %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("must not chmod replaced dir: %04o", info.Mode().Perm())
	}
}

func TestRestoreDirMode_SameIdentity_RestoresMode(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "d")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseline, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := fsafety.RestoreDirMode(dir, baseline, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%04o", info.Mode().Perm())
	}
}

func TestSortedDirsDeepestFirst_Stable(t *testing.T) {
	fp := fsafety.TransactionFootprint{
		Dirs: []fsafety.CreatedDir{
			{Rel: "a"},
			{Rel: "a/b/c"},
			{Rel: "a/b"},
			{Rel: "z"},
		},
	}
	got := fp.SortedDirsDeepestFirst()
	want := []string{"a/b/c", "a/b", "a", "z"}
	for i, w := range want {
		if got[i].Rel != w {
			t.Fatalf("order[%d]=%s want %s", i, got[i].Rel, w)
		}
	}
}
