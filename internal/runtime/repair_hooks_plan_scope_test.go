package runtime_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

// DOGFOOD-018: Repair Apply must not reconcile hooks unless the reviewed plan
// includes a hook target — even when a hooks-capable adapter is selected.

func TestApplyRuntimeRepair_UnplannedHooksUntouched(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)

	wrapper := filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	hooksJSON := filepath.Join(root, ".cursor", "hooks.json")
	if _, err := os.Stat(wrapper); err != nil {
		t.Fatalf("expected hooks wrapper after init: %v", err)
	}
	beforeWrapper := fileFingerprint(t, wrapper)
	beforeJSON := optionalFingerprint(t, hooksJSON)

	// Unrelated defect only: AGENTS.md missing → RepairKindAgents, not hooks.
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHook {
			t.Fatalf("plan must not include hook targets for AGENTS-only defect: %#v", plan.Targets)
		}
	}
	if !containsPath(plan.Creates, config.FileAgentsMD) {
		t.Fatalf("expected AGENTS repair, plan=%#v", plan)
	}

	res := applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 15, 14, 0, 0))
	if res.Blocked || res.Noop {
		t.Fatalf("unexpected result: %#v", res)
	}
	for _, action := range res.Actions {
		if strings.Contains(strings.ToLower(action), "hook") {
			t.Fatalf("hooks must not appear in Apply actions: %#v", res.Actions)
		}
	}

	if got := fileFingerprint(t, wrapper); got != beforeWrapper {
		t.Fatalf("hooks wrapper mutated without hook plan target:\nbefore=%s\nafter=%s", beforeWrapper, got)
	}
	if beforeJSON != "" {
		if got := optionalFingerprint(t, hooksJSON); got != beforeJSON {
			t.Fatalf("hooks.json mutated without hook plan target:\nbefore=%s\nafter=%s", beforeJSON, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal("AGENTS.md should have been restored")
	}
}

func TestApplyRuntimeRepair_PlannedHooksReconcile(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	wrapperRel := ".cursor/hooks/atlas/dispatch"
	wrapper := filepath.Join(root, filepath.FromSlash(wrapperRel))
	if err := os.Remove(wrapper); err != nil {
		t.Fatal(err)
	}

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	foundHook := false
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHook && target.Path == wrapperRel {
			foundHook = true
		}
	}
	if !foundHook {
		t.Fatalf("expected hook repair target for missing wrapper, plan=%#v", plan)
	}

	res := applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 15, 14, 1, 0))
	if res.Blocked {
		t.Fatalf("blocked: %#v", res)
	}
	if _, err := os.Stat(wrapper); err != nil {
		t.Fatalf("hooks wrapper not restored: %v", err)
	}
	restored := false
	for _, p := range res.Created {
		if p == wrapperRel {
			restored = true
		}
	}
	if !restored {
		t.Fatalf("expected created hook path in result: %#v", res)
	}
}

func fileFingerprint(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func optionalFingerprint(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
