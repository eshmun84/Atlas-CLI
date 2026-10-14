package home_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestRequireHealthy_MissingHome(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing-home")
	t.Setenv(home.EnvAtlasHome, dir)
	status, err := home.RequireHealthy()
	if err == nil {
		t.Fatal("expected error for missing Home")
	}
	if status.Exists {
		t.Fatal("status must report missing")
	}
	if !strings.Contains(err.Error(), home.RepairHomeHint) {
		t.Fatalf("error must be actionable: %v", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("RequireHealthy must not create Home")
	}
}

func TestRequireHealthy_AfterEnsureAndMirror(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "atlas-home")
	t.Setenv(home.EnvAtlasHome, dir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	status, err := home.RequireHealthy()
	if err != nil {
		t.Fatalf("healthy Home: %v", err)
	}
	if !home.Healthy(status) {
		t.Fatalf("Healthy = false: %#v", status)
	}
}

func TestRequireHealthy_IncompleteLayout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "atlas-home")
	t.Setenv(home.EnvAtlasHome, dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := home.RequireHealthy()
	if err == nil {
		t.Fatal("expected incomplete Home error")
	}
	if !strings.Contains(err.Error(), home.RepairHomeHint) {
		t.Fatalf("error must be actionable: %v", err)
	}
}

func TestEnsureAndMirror_IdempotentContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "atlas-home")
	t.Setenv(home.EnvAtlasHome, dir)
	fixed := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	if _, err := home.EnsureAndMirror(fixed); err != nil {
		t.Fatal(err)
	}
	first, err := snapshotHomeTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := home.EnsureAndMirror(fixed.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	second, err := snapshotHomeTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Paths and asset bytes must match; state/home.yaml UpdatedAt may change.
	for rel, sum := range first {
		if rel == "state/home.yaml" {
			continue
		}
		if second[rel] != sum {
			t.Fatalf("idempotent drift at %s", rel)
		}
	}
	for rel := range second {
		if _, ok := first[rel]; !ok && rel != "state/home.yaml" {
			t.Fatalf("unexpected new path on second mirror: %s", rel)
		}
	}
	if _, err := home.RequireHealthy(); err != nil {
		t.Fatal(err)
	}
}

func snapshotHomeTree(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			out[rel+"/"] = "dir"
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[rel] = string(data)
		return nil
	})
	return out, err
}
