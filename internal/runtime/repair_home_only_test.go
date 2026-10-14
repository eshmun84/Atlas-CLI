package runtime_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestBuildRuntimeRepairPlan_HomeOnlyWithoutProject(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "missing-home")
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	health := runtime.EvaluateHealth(root, atlas, files)
	plan := runtime.BuildRuntimeRepairPlan(root, health)
	if plan.Blocked {
		t.Fatalf("home-only repair must not be blocked: %#v", plan.Blockers)
	}
	if !plan.NeedsApply() {
		t.Fatal("expected home create target")
	}
	found := false
	for _, target := range plan.Targets {
		if target.Path == runtime.RepairHomePath && target.Kind == runtime.RepairKindHome {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v", plan.Targets)
	}
	if !runtime.ShowRuntimeRepair(root, atlas, health) {
		t.Fatal("Runtime Repair must be visible for unhealthy Home")
	}
}

func TestApplyRuntimeRepair_HomeOnlyMaterializes(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	health := runtime.EvaluateHealth(root, atlas, files)
	plan := runtime.BuildRuntimeRepairPlan(root, health)
	result, err := runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 14, 18, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Noop || result.Blocked {
		t.Fatalf("result = %#v", result)
	}
	if _, err := home.RequireHealthy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "state", "home.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatal("home-only repair must not create project .atlas")
	}
}
