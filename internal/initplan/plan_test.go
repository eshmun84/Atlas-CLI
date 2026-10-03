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

func TestBuildWithOptions_DefaultMatchesBuild(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	result := workspace.DiscoveryResult{RootPath: root}

	base, err := initplan.Build(root, result)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	withOpts, err := initplan.BuildWithOptions(root, result, initplan.Options{})
	if err != nil {
		t.Fatalf("BuildWithOptions: %v", err)
	}
	if base.ProjectMode != withOpts.ProjectMode {
		t.Fatalf("mode = %q, want %q", withOpts.ProjectMode, base.ProjectMode)
	}
	if base.ProjectName != withOpts.ProjectName {
		t.Fatalf("name = %q, want %q", withOpts.ProjectName, base.ProjectName)
	}
}

func TestBuildWithOptions_ProjectNameOverride(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plan, err := initplan.BuildWithOptions(root, workspace.DiscoveryResult{RootPath: root}, initplan.Options{
		ProjectName: "CustomName",
	})
	if err != nil {
		t.Fatalf("BuildWithOptions: %v", err)
	}
	if plan.ProjectName != "CustomName" {
		t.Fatalf("name = %q, want CustomName", plan.ProjectName)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not be created, stat err = %v", err)
	}
}

func TestBuildWithOptions_ModeOverride(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	result := workspace.DiscoveryResult{RootPath: root}

	base, err := initplan.Build(root, result)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if base.ProjectMode != config.ModeExisting {
		t.Fatalf("detected mode = %q, want existing", base.ProjectMode)
	}

	forcedNew, err := initplan.BuildWithOptions(root, result, initplan.Options{ModeOverride: "new"})
	if err != nil {
		t.Fatalf("new override: %v", err)
	}
	if forcedNew.ProjectMode != config.ModeGreenfield {
		t.Fatalf("new override mode = %q, want %q", forcedNew.ProjectMode, config.ModeGreenfield)
	}

	forcedExisting, err := initplan.BuildWithOptions(root, result, initplan.Options{ModeOverride: "existing"})
	if err != nil {
		t.Fatalf("existing override: %v", err)
	}
	if forcedExisting.ProjectMode != config.ModeExisting {
		t.Fatalf("existing override mode = %q, want %q", forcedExisting.ProjectMode, config.ModeExisting)
	}

	auto, err := initplan.BuildWithOptions(root, result, initplan.Options{ModeOverride: "auto"})
	if err != nil {
		t.Fatalf("auto override: %v", err)
	}
	if auto.ProjectMode != base.ProjectMode {
		t.Fatalf("auto override mode = %q, want %q", auto.ProjectMode, base.ProjectMode)
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
