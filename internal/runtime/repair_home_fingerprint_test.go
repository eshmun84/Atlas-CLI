package runtime_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestApplyRuntimeRepair_ProjectOnly_DoesNotMutateHomeFingerprint(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	before := homeMirrorFingerprint(t, homePath)

	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHome {
			t.Fatalf("project-only defect must not include Home target: %#v", plan.Targets)
		}
	}
	res := applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 15, 16, 0, 0))
	if res.Blocked || res.Noop {
		t.Fatalf("unexpected result: %#v", res)
	}
	after := homeMirrorFingerprint(t, homePath)
	if before != after {
		t.Fatalf("project-only Repair mutated Home fingerprint\nbefore=%s\nafter=%s", before, after)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal("AGENTS.md should have been restored")
	}
}

func TestApplyRuntimeRepair_IncompatibleHome_FailsBeforeProjectMutation(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	sig := plan.Signature()

	// Stamp an unsupported newer Home schema so Apply fails closed before any
	// project mutation (schema gate runs before remirror / project writes).
	statePath := filepath.Join(homePath, "state", "home.yaml")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(string(raw), "schema_version: 1", "schema_version: 99", 1)
	if patched == string(raw) {
		t.Fatal("failed to patch schema_version")
	}
	if err := os.WriteFile(statePath, []byte(patched), 0o600); err != nil {
		t.Fatal(err)
	}

	_, applyErr := runtime.ApplyRuntimeRepair(root, sig, func() time.Time {
		return time.Date(2026, 10, 15, 16, 1, 0, 0, time.UTC)
	})
	if applyErr == nil {
		t.Fatal("expected Apply to fail when Home schema is incompatible")
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("project mutation must not occur when Home is incompatible")
	}
	if !strings.Contains(strings.ToLower(applyErr.Error()), "schema") {
		t.Fatalf("error should mention schema incompatibility: %v", applyErr)
	}
}

func TestApplyRuntimeRepair_ExplicitHomeTarget_CanRemirror(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !planHasHome(plan) {
		t.Fatalf("expected Home target, plan=%#v", plan)
	}
	res, err := runtime.ApplyRuntimeRepair(root, plan.Signature(), fixedClock(2026, 10, 15, 16, 2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked || res.Noop {
		t.Fatalf("result=%#v", res)
	}
	if _, err := home.RequireHealthy(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range res.Actions {
		if strings.Contains(a, runtime.RepairHomePath) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Home action: %#v", res.Actions)
	}
}

func TestApplyRuntimeRepair_UnrelatedTarget_NoHiddenEnsureAndMirror(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath := os.Getenv(home.EnvAtlasHome)
	stBefore, err := home.RequireHealthy()
	if err != nil {
		t.Fatal(err)
	}
	beforeUpdated := stBefore.UpdatedAt
	beforeFP := homeMirrorFingerprint(t, homePath)

	if err := os.Remove(filepath.Join(root, config.FileCursorAtlasMDC)); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if planHasHome(plan) {
		t.Fatal("cursor projection repair must not include Home target")
	}
	_ = applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 15, 16, 3, 0))

	stAfter, err := home.RequireHealthy()
	if err != nil {
		t.Fatal(err)
	}
	if stAfter.UpdatedAt != beforeUpdated {
		t.Fatalf("Home UpdatedAt changed without Home target: %q -> %q", beforeUpdated, stAfter.UpdatedAt)
	}
	if homeMirrorFingerprint(t, homePath) != beforeFP {
		t.Fatal("Home fingerprint changed without Home target")
	}
}

func planHasHome(plan runtime.RuntimeRepairPlan) bool {
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHome && target.Path == runtime.RepairHomePath {
			return true
		}
	}
	return false
}

func homeMirrorFingerprint(t *testing.T, homePath string) string {
	t.Helper()
	h := sha256.New()
	statePath := filepath.Join(homePath, "state", "home.yaml")
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	h.Write(state)
	assetsRoot := filepath.Join(homePath, "assets")
	err = filepath.Walk(assetsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(homePath, path)
		h.Write([]byte(filepath.ToSlash(rel)))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		h.Write(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
