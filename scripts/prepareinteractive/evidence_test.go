package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteEvidenceFile_Persists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose-project-a.json")
	if err := writeEvidenceFile(path, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != `{"ok":true}` {
		t.Fatalf("got %q err=%v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o want 0600", info.Mode().Perm())
	}
}

func TestWriteEvidenceFile_DoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ENVIRONMENT.json")
	if err := os.WriteFile(path, []byte(`{"keep":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeEvidenceFile(path, []byte(`{"overwrite":true}`)); err == nil {
		t.Fatal("expected exclusive-create failure")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != `{"keep":true}` {
		t.Fatalf("existing file mutated: %q", got)
	}
}

func TestWriteEvidenceFile_PropagatesDirectoryError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "compose-project-b.json")
	if err := writeEvidenceFile(path, []byte(`{}`)); err == nil {
		t.Fatal("expected write failure")
	}
}

func TestWriteEvidenceFile_SymlinkLeafDoesNotMutateTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.json")
	original := []byte(`{"outside":true,"secret":"unchanged"}`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "ENVIRONMENT.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := writeEvidenceFile(link, []byte(`{"env":"ATTACK"}`))
	if err == nil {
		t.Fatal("expected refusal to write through symlink")
	}

	got, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != string(original) {
		t.Fatalf("external target mutated: got %q want %q", got, original)
	}
	// Link must still be a symlink (not replaced).
	info, lerr := os.Lstat(link)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink leaf was replaced; exclusive create should have failed first")
	}
}
