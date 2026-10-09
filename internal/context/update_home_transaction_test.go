package context_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestApplyUpdate_ProjectHomeBaselinePreservedOnRollback(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateRaw := []byte("schema_version: 1\ninitialized: true\nproject_name: ctx-base\n")
	if err := os.WriteFile(filepath.Join(root, ".atlas", "state.yaml"), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := home.ProjectID(root, "ctx-base")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	if err := home.WriteProjectIdentity(homeDir, pid, "ctx-base", root, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(homeDir, "projects", pid, "local", "user-marker.txt")
	if err := os.WriteFile(marker, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := snapshotHomeProject(t, homeDir, pid)

	var n int
	atlascontext.SetAfterWriteHookForTest(func(rel string) error {
		n++
		if n >= 1 {
			return os.ErrInvalid
		}
		return nil
	})
	t.Cleanup(func() { atlascontext.SetAfterWriteHookForTest(nil) })

	plan := atlascontext.BuildUpdatePlan(root, true, state, "implement feature")
	_, err = atlascontext.ApplyUpdate(root, plan.Signature(), state, "implement feature", func() time.Time {
		return time.Date(2026, 10, 8, 23, 30, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back context baseline") {
		t.Fatalf("expected rollback, got %v", err)
	}
	after := snapshotHomeProject(t, homeDir, pid)
	if !mapsEqual(baseline, after) {
		t.Fatalf("project Home drifted from baseline\nbefore=%v\nafter=%v", baseline, after)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("user marker: %q %v", got, err)
	}
}

func TestApplyUpdate_FreshProjectHomeRemovedOnRollback(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateRaw := []byte("schema_version: 1\ninitialized: true\nproject_name: ctx-fresh\n")
	if err := os.WriteFile(filepath.Join(root, ".atlas", "state.yaml"), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := home.ProjectID(root, "ctx-fresh")
	if err != nil {
		t.Fatal(err)
	}

	atlascontext.SetAfterWriteHookForTest(func(rel string) error { return os.ErrInvalid })
	t.Cleanup(func() { atlascontext.SetAfterWriteHookForTest(nil) })

	plan := atlascontext.BuildUpdatePlan(root, true, state, "implement feature")
	_, err = atlascontext.ApplyUpdate(root, plan.Signature(), state, "implement feature", func() time.Time {
		return time.Date(2026, 10, 8, 23, 35, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back context baseline") {
		t.Fatalf("expected rollback, got %v", err)
	}
	projRoot := filepath.Join(homeDir, "projects", pid)
	if _, err := os.Lstat(projRoot); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(projRoot)
		t.Fatalf("fresh project Home must be removed: %v err=%v", entries, err)
	}
}

func snapshotHomeProject(t *testing.T, homeDir, pid string) map[string]string {
	t.Helper()
	root := filepath.Join(homeDir, "projects", pid)
	out := map[string]string{}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return out
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			out[filepath.ToSlash(rel)+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
