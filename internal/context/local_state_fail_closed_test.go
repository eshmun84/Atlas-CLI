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

func TestApplyUpdate_CorruptLocalState_NoMutation(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateRaw := []byte("schema_version: 1\ninitialized: true\nproject_name: ctx-corrupt-local\n")
	if err := os.WriteFile(filepath.Join(root, ".atlas", "state.yaml"), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := home.ProjectID(root, "ctx-corrupt-local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	if err := home.WriteProjectIdentity(homeDir, pid, "ctx-corrupt-local", root, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("schema_version: [\nbad-local\n")
	localPath := home.ProjectLocalStatePath(homeDir, pid)
	if err := os.WriteFile(localPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	ctxDir := home.ProjectContextDir(homeDir, pid)
	baselineHome := snapshotHomeProject(t, homeDir, pid)
	baselinePortable, err := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	plan := atlascontext.BuildUpdatePlan(root, true, state, "implement feature")
	_, err = atlascontext.ApplyUpdate(root, plan.Signature(), state, "implement feature", func() time.Time {
		return time.Date(2026, 10, 8, 23, 40, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "local state") {
		t.Fatalf("expected local state error, got %v", err)
	}
	afterHome := snapshotHomeProject(t, homeDir, pid)
	if !mapsEqual(baselineHome, afterHome) {
		t.Fatalf("project Home drifted\nbefore=%v\nafter=%v", baselineHome, afterHome)
	}
	gotLocal, err := os.ReadFile(localPath)
	if err != nil || string(gotLocal) != string(corrupt) {
		t.Fatalf("local state mutated: %q err=%v", gotLocal, err)
	}
	gotPortable, err := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if err != nil || string(gotPortable) != string(baselinePortable) {
		t.Fatalf("portable state mutated: %q err=%v", gotPortable, err)
	}
	for _, name := range []string{"index.yaml", "capsule.md"} {
		if _, err := os.Lstat(filepath.Join(ctxDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s must not remain after abort: %v", name, err)
		}
	}
	entries, err := os.ReadDir(ctxDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "pack-") {
			t.Fatalf("pack artifact leaked: %s", e.Name())
		}
	}
}
