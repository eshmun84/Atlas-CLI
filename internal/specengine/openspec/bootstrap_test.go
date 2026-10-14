package openspec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestEnsureBootstrap_AbsentCreatesMinimum(t *testing.T) {
	root := t.TempDir()
	result, err := openspec.EnsureBootstrap(root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created {
		t.Fatal("expected Created")
	}
	for _, rel := range []string{"openspec", "openspec/changes"} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s: %v %#v", rel, err, info)
		}
	}
	assess, err := openspec.AssessLayout(root)
	if err != nil || !assess.Valid {
		t.Fatalf("assess = %#v err=%v", assess, err)
	}
}

func TestEnsureBootstrap_ValidNoOp(t *testing.T) {
	root := t.TempDir()
	if _, err := openspec.EnsureBootstrap(root); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "openspec", "changes", "keep.txt")
	if err := os.WriteFile(marker, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := openspec.EnsureBootstrap(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatal("valid layout must be no-op")
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "keep\n" {
		t.Fatalf("tree mutated: %q %v", data, err)
	}
}

func TestEnsureBootstrap_PartialFailClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "openspec", "marker.txt")
	if err := os.WriteFile(marker, []byte("partial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := openspec.EnsureBootstrap(root)
	if err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("expected incompatible error, got %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "partial\n" {
		t.Fatalf("partial tree mutated: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec", "changes")); !os.IsNotExist(err) {
		t.Fatal("must not heuristically complete partial openspec/")
	}
}
