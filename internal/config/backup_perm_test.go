package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestBackupPermissions_InitAndRepair(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("old-mdc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "agents", "keep.md"), []byte("nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "bak-perm", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 8, 21, 0, 0, 0, time.UTC)
	result, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupDir == "" {
		t.Fatal("expected init backup dir")
	}
	assertPrivateBackupTree(t, homeDir, result.BackupDir)

	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)
	repairRes, err := runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 8, 21, 5, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	if repairRes.BackupDir == "" {
		t.Fatal("expected repair backup dir")
	}
	assertPrivateBackupTree(t, homeDir, repairRes.BackupDir)
}

func TestBackupConflicts_NestedDirectoryPermissions(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	nested := filepath.Join(root, ".cursor", "agents")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "atlas-worker.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	pid := "nested-bak"
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	backupRel, _, _, err := config.BackupConflicts(root, homeDir, pid, []config.ConflictBackup{
		{Rel: ".cursor/agents", Action: "replaced", Reason: "test"},
	}, time.Date(2026, 10, 8, 21, 10, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	assertPrivateBackupTree(t, homeDir, backupRel)
}

func assertPrivateBackupTree(t *testing.T, homeDir, backupRel string) {
	t.Helper()
	root := filepath.Join(homeDir, filepath.FromSlash(backupRel))
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		// Umask-aware: no group/other access on project-scoped Home backups.
		if perm&0o077 != 0 {
			t.Fatalf("%s has group/other bits: %04o", path, perm)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "manifest.json")
	info, err := os.Lstat(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("manifest group/other bits: %04o", info.Mode().Perm())
	}
}
