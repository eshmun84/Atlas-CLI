package fsafety_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestReadFileContained_SymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "nested", "config.yaml"), []byte("evil: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".atlas")); err != nil {
		t.Fatal(err)
	}
	_, err := fsafety.ReadFileContained(root, ".atlas/nested/config.yaml")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink refusal, got %v", err)
	}
}

func TestReadFileContained_SymlinkLeaf(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ext := filepath.Join(outside, "x.yaml")
	if err := os.WriteFile(ext, []byte("evil: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(root, ".atlas", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := fsafety.ReadFileContained(root, ".atlas/config.yaml")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink leaf refusal, got %v", err)
	}
}

func TestReadFileContained_RegularOK(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("ok: true\n")
	if err := os.WriteFile(filepath.Join(root, ".atlas", "config.yaml"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fsafety.ReadFileContained(root, ".atlas/config.yaml")
	if err != nil || string(got) != string(want) {
		t.Fatalf("got %q err=%v", got, err)
	}
}
