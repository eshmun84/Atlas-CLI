package runtime_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestApplyRuntimeRepair_ProjectHomeBaselinePreservedOnRollback(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	baseline := snapshotTree(t, filepath.Join(homePath, "projects", pid))

	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted-mdc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orch := filepath.Join(root, ".cursor", "agents", "atlas-orchestrator.md")
	if err := os.WriteFile(orch, []byte("drifted-orch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var n int
	runtime.SetRepairAfterWriteHookForTest(func(rel string) error {
		n++
		if n >= 1 {
			return os.ErrInvalid
		}
		return nil
	})
	t.Cleanup(func() { runtime.SetRepairAfterWriteHookForTest(nil) })

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !plan.NeedsApply() {
		t.Fatalf("expected apply plan: %#v", plan)
	}
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 8, 22, 10, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back project repair baseline") {
		t.Fatalf("expected rollback, got %v", err)
	}
	after := snapshotTree(t, filepath.Join(homePath, "projects", pid))
	assertUnchangedTree(t, filepath.Join(homePath, "projects", pid), baseline)
	_ = after
}

func TestApplyRuntimeRepair_FreshProjectHomeRemovedOnRollback(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate fresh project-Home: remove prior project tree after Init.
	if err := os.RemoveAll(filepath.Join(homePath, "projects", pid)); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime.SetRepairAfterWriteHookForTest(func(rel string) error { return os.ErrInvalid })
	t.Cleanup(func() { runtime.SetRepairAfterWriteHookForTest(nil) })

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !plan.NeedsApply() {
		t.Fatalf("expected apply plan: %#v", plan)
	}
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 8, 22, 15, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back project repair baseline") {
		t.Fatalf("expected rollback, got %v", err)
	}
	projRoot := filepath.Join(homePath, "projects", pid)
	if _, err := os.Lstat(projRoot); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(projRoot)
		t.Fatalf("fresh project Home must be removed after rollback: %v err=%v", entries, err)
	}
}

func TestApplyRuntimeRepair_AttemptBackupRemovedAfterSuccessfulRollback(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	oldStamp := "20260101T000000.000000000Z-000001"
	oldBackup := filepath.Join(homePath, "projects", pid, home.ProjectDirBackups, oldStamp)
	if err := os.MkdirAll(oldBackup, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldBackup, "keep.txt"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime.SetRepairAfterWriteHookForTest(func(rel string) error { return os.ErrInvalid })
	t.Cleanup(func() { runtime.SetRepairAfterWriteHookForTest(nil) })

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 8, 22, 20, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	backups := filepath.Join(homePath, "projects", pid, home.ProjectDirBackups)
	entries, err := os.ReadDir(backups)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != oldStamp {
		t.Fatalf("expected only pre-existing backup retained, got %v", names)
	}
}

func TestApplyRuntimeRepair_CleanupFailureAfterSuccessfulRestore(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime.SetRemoveAttemptBackupForTest(func(homePath, rel string) error {
		return errors.New("injected cleanup failure")
	})
	t.Cleanup(func() { runtime.SetRemoveAttemptBackupForTest(nil) })
	runtime.SetRepairAfterWriteHookForTest(func(rel string) error { return os.ErrInvalid })
	t.Cleanup(func() { runtime.SetRepairAfterWriteHookForTest(nil) })

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 9, 1, 10, 0, 0, time.UTC)
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "transactional backup cleanup failed") {
		t.Fatalf("expected cleanup failure wording, got %v", err)
	}
	if strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("must not claim rollback failed when baseline restored: %v", err)
	}
	if !strings.Contains(err.Error(), "restored project repair baseline") {
		t.Fatalf("expected restored wording: %v", err)
	}
	backups := filepath.Join(homePath, "projects", pid, home.ProjectDirBackups)
	entries, err := os.ReadDir(backups)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("attempt backup should remain when cleanup fails")
	}
}

func TestApplyRuntimeRepair_RollbackFailureRetainsBackup(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	pid, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Poison identity into a directory so RestoreFileSnapshot fails during rollback.
	runtime.SetRepairAfterWriteHookForTest(func(rel string) error {
		idPath := filepath.Join(homePath, "projects", pid, filepath.FromSlash(home.FileProjectIdentity))
		_ = os.Remove(idPath)
		if err := os.MkdirAll(idPath, 0o700); err != nil {
			return err
		}
		return os.ErrInvalid
	})
	t.Cleanup(func() { runtime.SetRepairAfterWriteHookForTest(nil) })

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 9, 1, 15, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("expected rollback failure, got %v", err)
	}
	backups := filepath.Join(homePath, "projects", pid, home.ProjectDirBackups)
	entries, err := os.ReadDir(backups)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("attempt backup must be retained when rollback fails")
	}
}
