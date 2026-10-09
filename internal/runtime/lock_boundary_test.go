package runtime_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestApplyRuntimeRepair_StaleSignature_RecomputedUnderLock(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "lock-repair", ProjectMode: "new",
		ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 9, 18, 20, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	}); err != nil {
		t.Fatal(err)
	}

	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	health := runtime.EvaluateHealth(root, atlas, files)
	reviewed := runtime.BuildRuntimeRepairPlan(root, health)
	sig := reviewed.Signature()

	// Change runtime state after Review so locked recompute diverges.
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("drifted-by-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := runtime.ApplyRuntimeRepair(root, sig, func() time.Time { return fixed.Add(time.Minute) })
	if err != nil {
		t.Fatalf("stale apply should not error: %v", err)
	}
	if !res.Stale {
		t.Fatal("expected Stale after pre-lock drift")
	}
	got, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "drifted-by-test\n" {
		t.Fatalf("stale path must perform zero mutation; got %q", got)
	}
}
