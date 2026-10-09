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
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestApplyUpdate_RollbackAfterPartialWrite(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateRaw := []byte("schema_version: 1\ninitialized: true\nproject_name: ctx-rb\n")
	if err := os.WriteFile(filepath.Join(root, ".atlas", "state.yaml"), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}

	var n int
	atlascontext.SetAfterWriteHookForTest(func(rel string) error {
		n++
		if n >= 2 {
			return os.ErrInvalid
		}
		return nil
	})
	t.Cleanup(func() { atlascontext.SetAfterWriteHookForTest(nil) })

	plan := atlascontext.BuildUpdatePlan(root, true, state, "implement feature")
	_, err = atlascontext.ApplyUpdate(root, plan.Signature(), state, "implement feature", func() time.Time {
		return time.Date(2026, 10, 8, 23, 20, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back context baseline") {
		t.Fatalf("expected rollback, got %v", err)
	}
	pid, _ := home.ProjectID(root, "ctx-rb")
	ctxDir := home.ProjectContextDir(homeDir, pid)
	if entries, _ := os.ReadDir(ctxDir); len(entries) != 0 {
		// Index/capsule/pack must not remain from failed apply (or must match prior absence).
		for _, e := range entries {
			t.Fatalf("orphan context entry after rollback: %s", e.Name())
		}
	}
	gotState, err := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if err != nil || string(gotState) != string(stateRaw) {
		t.Fatalf("portable state changed: %q %v", gotState, err)
	}
}

func TestApplyUpdate_BlocksSymlinkAtlasParent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".atlas")); err != nil {
		t.Fatal(err)
	}
	if err := fsafety.AtomicWriteContained(root, config.FileState, []byte("x\n"), 0o644); err == nil {
		t.Fatal("expected symlink .atlas block")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("external write: %v", entries)
	}
}
