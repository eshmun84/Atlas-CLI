package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestApplyRuntimeRepair_ProjectHomeSnapshotError_AbortsBeforeMutation(t *testing.T) {
	// Observable fail-closed: project Home root as symlink makes CaptureProjectLayoutSnapshot
	// fail without home production test setters.
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baselineHome := snapshotTree(t, filepath.Join(homePath, "projects", pid))
	preApplyMDC := []byte("drifted\n")

	projRoot := home.ProjectRoot(homePath, pid)
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

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !plan.NeedsApply() {
		t.Fatalf("expected apply plan")
	}
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 9, 0, 20, 0, 0, time.UTC)
	})
	// Symlink project root fails closed on contained local-state read and/or snapshot.
	if err == nil || !(strings.Contains(err.Error(), "snapshot") ||
		strings.Contains(err.Error(), "local state") ||
		strings.Contains(err.Error(), "symlink")) {
		t.Fatalf("expected fail-closed abort on symlink project Home, got %v", err)
	}
	_ = os.Remove(projRoot)
	if err := os.Rename(realRoot, projRoot); err != nil {
		t.Fatal(err)
	}
	assertUnchangedTree(t, filepath.Join(homePath, "projects", pid), baselineHome)
	got, _ := os.ReadFile(filepath.Join(root, config.FileCursorAtlasMDC))
	if string(got) != string(preApplyMDC) {
		t.Fatalf("runtime file mutated after snapshot abort: %q", got)
	}
}
