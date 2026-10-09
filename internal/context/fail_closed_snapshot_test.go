package context_test

import (
	"errors"
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

func TestApplyUpdate_ProjectHomeSnapshotError_AbortsBeforeMutation(t *testing.T) {
	// Observable fail-closed: project Home root as symlink makes CaptureProjectLayoutSnapshot
	// fail without relying on home production test setters.
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateRaw := []byte("schema_version: 1\ninitialized: true\nproject_name: ctx-snap\n")
	if err := os.WriteFile(filepath.Join(root, ".atlas", "state.yaml"), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := home.ProjectID(root, "ctx-snap")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	if err := home.WriteProjectIdentity(homeDir, pid, "ctx-snap", root, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	baseline := snapshotHomeProject(t, homeDir, pid)

	projRoot := home.ProjectRoot(homeDir, pid)
	realRoot := projRoot + ".real"
	if err := os.Rename(projRoot, realRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, projRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(projRoot)
		_ = os.Rename(realRoot, projRoot)
	})

	plan := atlascontext.BuildUpdatePlan(root, true, state, "implement feature")
	_, err = atlascontext.ApplyUpdate(root, plan.Signature(), state, "implement feature", func() time.Time {
		return time.Date(2026, 10, 9, 0, 10, 0, 0, time.UTC)
	})
	// Symlink project root fails closed on contained local-state read and/or snapshot.
	if err == nil || !(strings.Contains(err.Error(), "snapshot") ||
		strings.Contains(err.Error(), "local state") ||
		strings.Contains(err.Error(), "symlink")) {
		t.Fatalf("expected fail-closed abort on symlink project Home, got %v", err)
	}
	// Restore symlink so baseline comparison reads the real tree.
	_ = os.Remove(projRoot)
	if err := os.Rename(realRoot, projRoot); err != nil {
		t.Fatal(err)
	}
	after := snapshotHomeProject(t, homeDir, pid)
	if !mapsEqual(baseline, after) {
		t.Fatalf("project Home mutated after snapshot abort\nbefore=%v\nafter=%v", baseline, after)
	}
	gotState, _ := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if string(gotState) != string(stateRaw) {
		t.Fatalf("portable state mutated: %q", gotState)
	}
}

func TestApplyUpdate_PortableStateSnapshotError_ZeroWrites(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateRaw := []byte("schema_version: 1\ninitialized: true\nproject_name: ctx-ps\n")
	if err := os.WriteFile(filepath.Join(root, ".atlas", "state.yaml"), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := home.ProjectID(root, "ctx-ps")
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	atlascontext.SetCaptureFileSnapshotForTest(func(r, rel string) (fsafety.FileSnapshot, error) {
		if rel == config.FileState {
			return fsafety.FileSnapshot{}, errors.New("injected portable state snapshot error")
		}
		return fsafety.CaptureFileSnapshot(r, rel)
	})
	t.Cleanup(func() { atlascontext.SetCaptureFileSnapshotForTest(nil) })

	plan := atlascontext.BuildUpdatePlan(root, true, state, "implement feature")
	_, err = atlascontext.ApplyUpdate(root, plan.Signature(), state, "implement feature", func() time.Time {
		return time.Date(2026, 10, 9, 0, 15, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("expected snapshot abort, got %v", err)
	}
	ctxDir := home.ProjectContextDir(homeDir, pid)
	if entries, _ := os.ReadDir(ctxDir); len(entries) != 0 {
		t.Fatalf("context writes occurred: %v", entries)
	}
	gotState, _ := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if string(gotState) != string(stateRaw) {
		t.Fatalf("portable state mutated: %q", gotState)
	}
}
