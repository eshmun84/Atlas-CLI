package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestApplyRuntimeRepair_RollbackOnSecondWriteFailure(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	// Drift two Atlas-owned files so Apply has multiple writes.
	if err := os.WriteFile(filepath.Join(root, config.FileCursorAtlasMDC), []byte("drifted-cursor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orch := filepath.Join(root, ".cursor", "agents", "atlas-orchestrator.md")
	if err := os.WriteFile(orch, []byte("drifted-orch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeMDC, err := os.ReadFile(filepath.Join(root, config.FileCursorAtlasMDC))
	if err != nil {
		t.Fatal(err)
	}
	beforeOrch, err := os.ReadFile(orch)
	if err != nil {
		t.Fatal(err)
	}

	var sawFirst bool
	runtime.SetRepairAfterWriteHookForTest(func(rel string) error {
		if !sawFirst {
			sawFirst = true
			return nil
		}
		return os.ErrInvalid
	})
	t.Cleanup(func() { runtime.SetRepairAfterWriteHookForTest(nil) })

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !plan.NeedsApply() {
		t.Fatalf("expected apply: %#v", plan)
	}
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 8, 22, 0, 0, 0, time.UTC)
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back project repair baseline") {
		t.Fatalf("expected rollback error, got %v", err)
	}
	afterMDC, _ := os.ReadFile(filepath.Join(root, config.FileCursorAtlasMDC))
	afterOrch, _ := os.ReadFile(orch)
	if string(afterMDC) != string(beforeMDC) {
		t.Fatalf("cursor mdc not restored: %q", afterMDC)
	}
	if string(afterOrch) != string(beforeOrch) {
		t.Fatalf("orchestrator not restored to baseline: %q want %q", afterOrch, beforeOrch)
	}
}
