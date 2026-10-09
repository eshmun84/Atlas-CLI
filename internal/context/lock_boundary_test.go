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

func TestApplyUpdate_StaleSignature_RecomputedUnderLock(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "lock-ctx", ProjectMode: "new",
		ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 9, 18, 30, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	}); err != nil {
		t.Fatal(err)
	}

	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		// State may live under .atlas/state.yaml via apply result paths.
		data, readErr := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
		if readErr != nil {
			t.Fatal(err)
		}
		_ = data
		state, err = config.LoadStateDocumentAt(root)
		if err != nil {
			t.Fatal(err)
		}
	}
	plan := atlascontext.BuildUpdatePlan(root, state.Initialized, state, "implement feature")
	if plan.Blocked {
		t.Fatalf("plan blocked: %v", plan.Blockers)
	}
	sig := plan.Signature()

	// Mutate Home context leaf so locked BuildUpdatePlan signature changes.
	pid := plan.ProjectID
	indexPath := atlascontext.IndexPath(plan.HomePath, pid)
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte("stale-index: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := atlascontext.ApplyUpdate(root, sig, state, "implement feature", func() time.Time {
		return fixed.Add(time.Minute)
	})
	if err != nil {
		t.Fatalf("stale update should not hard-error: %v", err)
	}
	if !res.Stale {
		t.Fatal("expected Stale after pre-lock context drift")
	}
	got, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "stale-index: true\n" {
		t.Fatalf("zero mutation on stale; got %q", got)
	}
}
