package context_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestInspect_IndexSymlink_StatusUnreadable(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")

	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	ctxDir := filepath.Join(homeDir, "projects", pid, "context")
	if err := os.MkdirAll(ctxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	// Valid-looking external index that must never be accepted as fresh.
	extIndex := []byte("schema_version: 1\nproject_id: " + pid + "\nproject_root: " + root + "\nindexed_at: 2026-10-09T12:00:00Z\nfingerprint: deadbeef\n")
	extPath := filepath.Join(outside, "index.yaml")
	if err := os.WriteFile(extPath, extIndex, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, filepath.Join(ctxDir, "index.yaml")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ctxDir, "capsule.md"), "# capsule\n")

	snap := atlascontext.Inspect(atlascontext.InspectInput{
		Root: root, Initialized: true, ProjectName: "demo", StateProjectID: pid,
	})
	if snap.State != atlascontext.StatusUnreadable {
		t.Fatalf("want StatusUnreadable, got %#v", snap)
	}
	if snap.Fingerprint == "deadbeef" || snap.State == atlascontext.StatusPresent {
		t.Fatal("external index content must not be accepted")
	}
	got, _ := os.ReadFile(extPath)
	if string(got) != string(extIndex) {
		t.Fatal("external index mutated")
	}
}

func TestInspect_CapsuleSymlink_NeverHealthy(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")

	state := config.StateDocument{Initialized: true, ProjectName: "demo"}
	plan := atlascontext.BuildUpdatePlan(root, true, state, atlascontext.DefaultPackObjective)
	result, err := atlascontext.ApplyUpdate(root, plan.Signature(), state, atlascontext.DefaultPackObjective, func() time.Time {
		return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	extPath := filepath.Join(outside, "capsule.md")
	if err := os.WriteFile(extPath, []byte("# EXTERNAL CAPSULE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(result.CapsulePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, result.CapsulePath); err != nil {
		t.Fatal(err)
	}

	snap := atlascontext.InspectFromState(root, true, config.StateDocument{
		Initialized:             true,
		ProjectName:             "demo",
		ContextEconomyProjectID: result.Index.ProjectID,
	})
	if snap.State == atlascontext.StatusPresent {
		t.Fatalf("capsule symlink must never be healthy: %#v", snap)
	}
	if snap.State != atlascontext.StatusUnreadable && snap.State != atlascontext.StatusStale {
		t.Fatalf("want unreadable/stale, got %#v", snap)
	}
}

func TestInspect_LegacyContextSymlinkEscape_Refused(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")

	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, pid), 0o700); err != nil {
		t.Fatal(err)
	}
	extIndex := []byte("schema_version: 1\nproject_id: " + pid + "\nfingerprint: escape\nindexed_at: 2026-10-09T12:00:00Z\n")
	if err := os.WriteFile(filepath.Join(outside, pid, "index.yaml"), extIndex, 0o644); err != nil {
		t.Fatal(err)
	}
	legacyParent := filepath.Join(homeDir, "context", "projects")
	if err := os.MkdirAll(filepath.Dir(legacyParent), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, legacyParent); err != nil {
		t.Fatal(err)
	}

	snap := atlascontext.Inspect(atlascontext.InspectInput{
		Root: root, Initialized: true, ProjectName: "demo", StateProjectID: pid,
	})
	if snap.State == atlascontext.StatusPresent {
		t.Fatalf("legacy symlink escape must not be present: %#v", snap)
	}
	// Either missing (canonical absent, legacy refused) or unreadable — never fresh.
	if snap.Fingerprint == "escape" {
		t.Fatal("escaped legacy index must not be parsed")
	}
}

func TestLoadIndexAt_Symlink_Error(t *testing.T) {
	homeDir := t.TempDir()
	outside := t.TempDir()
	pid := "idx-sym"
	ctxDir := filepath.Join(homeDir, "projects", pid, "context")
	if err := os.MkdirAll(ctxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ext := filepath.Join(outside, "index.yaml")
	if err := os.WriteFile(ext, []byte("schema_version: 1\nfingerprint: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(ctxDir, "index.yaml")); err != nil {
		t.Fatal(err)
	}
	_, present, err := atlascontext.LoadIndexAt(homeDir, pid)
	if err == nil || present {
		t.Fatalf("LoadIndexAt must refuse symlink: present=%v err=%v", present, err)
	}
}

func TestResolveContextDir_SymlinkIndex_NotPresent(t *testing.T) {
	homeDir := t.TempDir()
	outside := t.TempDir()
	pid := "resolve-sym"
	ctxDir := filepath.Join(homeDir, "projects", pid, "context")
	if err := os.MkdirAll(ctxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ext := filepath.Join(outside, "index.yaml")
	if err := os.WriteFile(ext, []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(ctxDir, "index.yaml")); err != nil {
		t.Fatal(err)
	}
	got := atlascontext.ResolveContextDir(homeDir, pid)
	want := atlascontext.ProjectDir(homeDir, pid)
	if got != want {
		t.Fatalf("symlink index must not select as present dir: got=%s want=%s", got, want)
	}
}
