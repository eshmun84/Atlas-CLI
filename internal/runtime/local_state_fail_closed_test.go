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

func TestApplyRuntimeRepair_CorruptLocalState_NoMutation(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("schema_version: [\nbad-local\n")
	localPath := home.ProjectLocalStatePath(homePath, pid)
	if err := os.WriteFile(localPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted-mdc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baselineHome := snapshotTree(t, filepath.Join(homePath, "projects", pid))
	baselineRoot := snapshotTree(t, root)
	portableBefore, err := os.ReadFile(filepath.Join(root, config.FileState))
	if err != nil {
		t.Fatal(err)
	}

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !plan.NeedsApply() {
		t.Fatalf("expected apply plan: %#v", plan)
	}
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 8, 22, 45, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "local state") {
		t.Fatalf("expected local state error, got %v", err)
	}
	assertUnchangedTree(t, filepath.Join(homePath, "projects", pid), baselineHome)
	assertUnchangedTree(t, root, baselineRoot)
	gotLocal, err := os.ReadFile(localPath)
	if err != nil || string(gotLocal) != string(corrupt) {
		t.Fatalf("local state mutated: %q err=%v", gotLocal, err)
	}
	portableAfter, err := os.ReadFile(filepath.Join(root, config.FileState))
	if err != nil || string(portableAfter) != string(portableBefore) {
		t.Fatalf("portable state mutated: %q err=%v", portableAfter, err)
	}
}
