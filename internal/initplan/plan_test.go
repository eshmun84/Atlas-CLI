package initplan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestBuild_InfersNameAndExistingMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	named := filepath.Join(root, "Atlas-CLI")
	if err := os.Mkdir(named, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(named, "README.md"), []byte("# demo"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plan, err := initplan.Build(named, workspace.DiscoveryResult{RootPath: named})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if plan.ProjectName != "Atlas-CLI" {
		t.Fatalf("name = %q, want Atlas-CLI", plan.ProjectName)
	}
	if plan.ProjectMode != config.ModeExisting {
		t.Fatalf("mode = %q, want %q", plan.ProjectMode, config.ModeExisting)
	}
	if plan.ConfigStorageMode != config.StorageModeLocal {
		t.Fatalf("storage_mode = %q, want %q", plan.ConfigStorageMode, config.StorageModeLocal)
	}
	if plan.MemoryProvider != config.MemoryProviderSQLite {
		t.Fatalf("memory provider = %q, want %q", plan.MemoryProvider, config.MemoryProviderSQLite)
	}
	assertStep(t, plan, "AGENTS.md", initplan.StatusCreate, "project runtime governance entrypoint")
	assertStep(t, plan, config.FileConfig, initplan.StatusCreate, "project Atlas configuration")
	assertStep(t, plan, config.FileMemoryDB, initplan.StatusFuture, "SQLite memory store")
}

func TestBuild_GreenfieldForEmptyDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plan, err := initplan.Build(root, workspace.DiscoveryResult{RootPath: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plan.ProjectMode != config.ModeGreenfield {
		t.Fatalf("mode = %q, want %q", plan.ProjectMode, config.ModeGreenfield)
	}
}

func TestBuild_ExistingWhenAtlasConfigPresent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".atlas", "config.yaml"), []byte("atlas: {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plan, err := initplan.Build(root, workspace.DiscoveryResult{
		RootPath: root,
		Files:    workspace.FileInfo{HasAtlasConfig: true},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plan.ProjectMode != config.ModeExisting {
		t.Fatalf("mode = %q, want %q", plan.ProjectMode, config.ModeExisting)
	}
	assertStep(t, plan, config.FileConfig, initplan.StatusSkipExisting, "existing Atlas config detected")
}

func TestBuild_SkipExistingPlannedFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# agents"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".atlas", "context"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".atlas", "local.yaml"), []byte("local: {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plan, err := initplan.Build(root, workspace.DiscoveryResult{RootPath: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	assertStep(t, plan, "AGENTS.md", initplan.StatusSkipExisting, "already exists")
	assertStep(t, plan, config.FileLocal, initplan.StatusSkipExisting, "already exists")
	assertStep(t, plan, config.FileConfig, initplan.StatusCreate, "project Atlas configuration")
}

func TestBuild_DoesNotCreateFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := initplan.Build(root, workspace.DiscoveryResult{RootPath: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not be created, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not be created, stat err = %v", err)
	}
}

func assertStep(t *testing.T, plan initplan.Plan, path, status, reason string) {
	t.Helper()
	for _, step := range plan.Steps {
		if step.Path == path {
			if step.Status != status {
				t.Fatalf("%s status = %q, want %q", path, step.Status, status)
			}
			if step.Reason != reason {
				t.Fatalf("%s reason = %q, want %q", path, step.Reason, reason)
			}
			return
		}
	}
	t.Fatalf("missing planned file %s in %#v", path, plan.Steps)
}
