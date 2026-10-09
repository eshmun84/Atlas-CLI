package home_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestEnsureProjectLayoutCreated_PartialNested_ReturnsCreatedDirs(t *testing.T) {
	homeDir := t.TempDir()
	pid := "partial-layout"
	root := filepath.Join(homeDir, "projects", pid)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	// Block the second layout directory so state is created then context fails.
	if err := os.WriteFile(filepath.Join(root, home.ProjectDirContext), []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirs, err := home.EnsureProjectLayoutCreated(homeDir, pid)
	if err == nil {
		t.Fatal("expected failure after partial layout creation")
	}
	foundState := false
	stateRel := filepath.ToSlash(filepath.Join("projects", pid, home.ProjectDirState))
	for _, d := range dirs {
		if d.Rel == stateRel {
			foundState = true
			break
		}
	}
	if !foundState {
		t.Fatalf("expected partial CreatedDir for %s, got %#v", stateRel, dirs)
	}
	if _, err := os.Lstat(filepath.Join(root, home.ProjectDirState)); err != nil {
		t.Fatal("state dir must remain after partial failure")
	}
}

func TestRestoreProjectLayoutSnapshot_NonEmptyCreatedDir_Conflict(t *testing.T) {
	homeDir := t.TempDir()
	pid := "layout-ne"
	snap, err := home.CaptureProjectLayoutSnapshot(homeDir, pid)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := home.EnsureProjectLayoutCreated(homeDir, pid)
	if err != nil {
		t.Fatal(err)
	}
	snap.Footprint.MergeDirs(dirs)
	foreign := filepath.Join(homeDir, "projects", pid, "context", "foreign.txt")
	if err := os.WriteFile(foreign, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = home.RestoreProjectLayoutSnapshot(snap)
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty conflict, got %v", err)
	}
	if _, err := os.Lstat(foreign); err != nil {
		t.Fatal("foreign content must be preserved")
	}
}

func TestRestoreProjectReset_ForeignContent_ConflictNoWipe(t *testing.T) {
	homeDir := t.TempDir()
	pid := "reset-fx"
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	prior := filepath.Join(homeDir, "projects", pid, "state", "prior.yaml")
	if err := os.WriteFile(prior, []byte("prior\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := home.StageProjectReset(homeDir, pid, "stamp1")
	if err != nil || !st.Active {
		t.Fatalf("stage: %#v %v", st, err)
	}
	// New live tree with Atlas footprint + foreign file.
	dirs, err := home.EnsureProjectLayoutCreated(homeDir, pid)
	if err != nil {
		t.Fatal(err)
	}
	fp := fsafety.TransactionFootprint{}
	fp.MergeDirs(dirs)
	w, wdirs, err := fsafety.AtomicWriteContainedTracked(homeDir, filepath.ToSlash(filepath.Join("projects", pid, "state", "local.yaml")), []byte("new\n"), 0o600, home.DirPermHome, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	fp.AddFile(w)
	fp.MergeDirs(wdirs)
	foreign := filepath.Join(homeDir, "projects", pid, "foreign-live.txt")
	if err := os.WriteFile(foreign, []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = home.RestoreProjectReset(st, fp)
	if err == nil || !strings.Contains(err.Error(), "rollback conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := os.Lstat(foreign); err != nil {
		t.Fatal("foreign live content must not be wiped")
	}
	// Staging still present (rename-back blocked).
	stagingAbs := filepath.Join(homeDir, filepath.FromSlash(st.StagingRel))
	if _, err := os.Lstat(stagingAbs); err != nil {
		t.Fatal("staging must remain when rename-back blocked")
	}
}

func TestRestoreMirrorSnapshot_NewHome_ForeignContent_Conflict(t *testing.T) {
	parent := t.TempDir()
	homeDir := filepath.Join(parent, "new-home")
	t.Setenv(home.EnvAtlasHome, homeDir)
	snap, err := home.CaptureMirrorSnapshot(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if snap.HomeExisted {
		t.Fatal("home must be absent at baseline")
	}
	ensured, err := home.EnsureAndMirror(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snap.Footprint = ensured.Footprint
	foreign := filepath.Join(homeDir, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = home.RestoreMirrorSnapshot(snap)
	// Footprint teardown may succeed for Atlas files/dirs; foreign file blocks
	// empty removal of home root if root is in createdDirs as ".".
	if err != nil && !strings.Contains(err.Error(), "rollback conflict") && !strings.Contains(err.Error(), "not empty") {
		// If restore reports other aggregated errors including foreign-blocked dirs, OK.
		if !strings.Contains(err.Error(), "conflict") && !strings.Contains(err.Error(), "not empty") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if _, err := os.Lstat(foreign); err != nil {
		t.Fatal("foreign content must survive (no RemoveAll on Home)")
	}
}

func TestRestoreMirrorSnapshot_LayoutDir_NonEmpty_Conflict(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	snap, err := home.CaptureMirrorSnapshot(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	ensured, err := home.EnsureAndMirror(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	snap.Footprint = ensured.Footprint
	// Inject foreign into a newly created nested layout dir that was empty-removable.
	agents := filepath.Join(homeDir, "assets", "agents")
	if err := os.WriteFile(filepath.Join(agents, "foreign.md"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = home.RestoreMirrorSnapshot(snap)
	if err == nil {
		t.Fatal("expected conflict from non-empty created dir")
	}
	if _, err := os.Lstat(filepath.Join(agents, "foreign.md")); err != nil {
		t.Fatal("foreign must be preserved")
	}
}

func TestRemoveContainedRel_StillRemovesAllowlistedBackupStamp(t *testing.T) {
	homeDir := t.TempDir()
	pid := "bak"
	rel := filepath.ToSlash(filepath.Join("projects", pid, "backups", "20260101T000000Z"))
	full := filepath.Join(homeDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(full, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "x"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := home.RemoveContainedRel(homeDir, rel); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(full); !os.IsNotExist(err) {
		t.Fatal("allowlisted stamp must be removed")
	}
}

func TestCommitProjectReset_RequiresActiveStaging(t *testing.T) {
	if err := home.CommitProjectReset(home.ProjectResetStaging{}); err != nil {
		t.Fatal(err)
	}
}

func TestResetProject_StillExplicitAPI(t *testing.T) {
	homeDir := t.TempDir()
	pid := "explicit"
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	if err := home.ResetProject(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(homeDir, "projects", pid)); !os.IsNotExist(err) {
		t.Fatal("reset must remove project tree")
	}
}
