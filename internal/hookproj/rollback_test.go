package hookproj_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
)

type fsFingerprint struct {
	cfg       string
	wrapper   string
	ownership string
	userHook  string
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "<missing>"
		}
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func captureFP(t *testing.T, root, homePath, projectID, userHookPath string) fsFingerprint {
	t.Helper()
	return fsFingerprint{
		cfg:       fileDigest(t, filepath.Join(root, ".cursor", "hooks.json")),
		wrapper:   fileDigest(t, filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")),
		ownership: fileDigest(t, hookproj.OwnershipPath(homePath, projectID)),
		userHook:  fileDigest(t, userHookPath),
	}
}

func seedWithUserHook(t *testing.T) (root, homePath, projectID, userHookPath string, ownership hookproj.OwnershipDocument, before fsFingerprint) {
	t.Helper()
	root = t.TempDir()
	homePath = t.TempDir()
	projectID = "proj-rb"
	userHookPath = filepath.Join(root, "user-owned.sh")
	if err := os.WriteFile(userHookPath, []byte("#!/bin/sh\necho user\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	userCfg := `{
  "version": 1,
  "hooks": {
    "afterFileEdit": [{"command": "./user-owned.sh", "matcher": "Write"}]
  }
}
`
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), []byte(userCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || res.Blocked || !res.Applied {
		t.Fatalf("seed err=%v blocked=%v applied=%v %v", err, res.Blocked, res.Applied, res.Errors)
	}
	// Confirm user entry preserved.
	raw, _ := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["hooks"].(map[string]any)["afterFileEdit"] == nil {
		t.Fatal("user afterFileEdit missing after seed")
	}
	return root, homePath, projectID, userHookPath, res.Ownership, captureFP(t, root, homePath, projectID, userHookPath)
}

func assertUnchanged(t *testing.T, before, after fsFingerprint) {
	t.Helper()
	if before != after {
		t.Fatalf("filesystem fingerprint changed\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestReconcile_FailureBeforeFirstWrite_NoMutation(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-rb-before"
	userHook := filepath.Join(root, "user-owned.sh")
	_ = os.WriteFile(userHook, []byte("#!/bin/sh\necho user\n"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), []byte(`{
  "version": 1,
  "hooks": {"afterFileEdit": [{"command": "./user-owned.sh"}]}
}
`), 0o644)
	before := captureFP(t, root, homePath, projectID, userHook)

	hookproj.SetApplyPhaseHookForTest(func(phase string) error {
		if phase == "before_apply" {
			return fmt.Errorf("injected before first write")
		}
		return nil
	})
	t.Cleanup(func() { hookproj.SetApplyPhaseHookForTest(nil) })

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if !strings.Contains(err.Error(), "injected before first write") {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rolled back: %v", err)
	}
	if res.Applied || res.Mutation.Applied || res.Mutation.Touched {
		t.Fatalf("must not mark success/touched: %#v", res.Mutation)
	}
	assertUnchanged(t, before, captureFP(t, root, homePath, projectID, userHook))
}

func TestReconcile_FailureAfterConfig_RollsBack(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-rb-cfg"
	userHook := filepath.Join(root, "user-owned.sh")
	_ = os.WriteFile(userHook, []byte("#!/bin/sh\necho user\n"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	userCfg := []byte(`{
  "version": 1,
  "hooks": {
    "afterFileEdit": [{"command": "./user-owned.sh", "matcher": "Write"}]
  }
}
`)
	cfgPath := filepath.Join(root, ".cursor", "hooks.json")
	if err := os.WriteFile(cfgPath, userCfg, 0o644); err != nil {
		t.Fatal(err)
	}
	before := captureFP(t, root, homePath, projectID, userHook)

	hookproj.SetApplyPhaseHookForTest(func(phase string) error {
		if phase == "after_config" {
			return fmt.Errorf("injected after hooks.json")
		}
		return nil
	})
	t.Cleanup(func() { hookproj.SetApplyPhaseHookForTest(nil) })

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err == nil || !strings.Contains(err.Error(), "injected after hooks.json") {
		t.Fatalf("err=%v", err)
	}
	if res.Applied {
		t.Fatal("must not apply on failure")
	}
	after := captureFP(t, root, homePath, projectID, userHook)
	assertUnchanged(t, before, after)

	got, _ := os.ReadFile(cfgPath)
	if string(got) != string(userCfg) {
		t.Fatalf("hooks.json not restored\nwant=%s\ngot=%s", userCfg, got)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")); !os.IsNotExist(err) {
		t.Fatal("wrapper must be removed on rollback")
	}
	if _, err := os.Stat(hookproj.OwnershipPath(homePath, projectID)); !os.IsNotExist(err) {
		t.Fatal("ownership must not remain after rollback")
	}
}

func TestReconcile_FailureAfterWrapper_RollsBack(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-rb-wrap"
	userHook := filepath.Join(root, "user-owned.sh")
	_ = os.WriteFile(userHook, []byte("#!/bin/sh\necho user\n"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	userCfg := []byte(`{"version":1,"hooks":{"afterFileEdit":[{"command":"./user-owned.sh"}]}}`)
	cfgPath := filepath.Join(root, ".cursor", "hooks.json")
	_ = os.WriteFile(cfgPath, userCfg, 0o644)
	before := captureFP(t, root, homePath, projectID, userHook)

	hookproj.SetApplyPhaseHookForTest(func(phase string) error {
		if phase == "after_wrapper" {
			return fmt.Errorf("injected after wrapper")
		}
		return nil
	})
	t.Cleanup(func() { hookproj.SetApplyPhaseHookForTest(nil) })

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err == nil || !strings.Contains(err.Error(), "injected after wrapper") {
		t.Fatalf("err=%v", err)
	}
	if res.Applied {
		t.Fatal("must not apply")
	}
	assertUnchanged(t, before, captureFP(t, root, homePath, projectID, userHook))
	got, _ := os.ReadFile(cfgPath)
	if string(got) != string(userCfg) {
		t.Fatalf("user hooks.json mutated: %s", got)
	}
}

func TestReconcile_FailureBeforeOwnership_RollsBackProjectWrites(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-rb-own"
	userHook := filepath.Join(root, "user-owned.sh")
	_ = os.WriteFile(userHook, []byte("#!/bin/sh\necho user\n"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	userCfg := []byte(`{"version":1,"hooks":{"afterFileEdit":[{"command":"./user-owned.sh"}]}}`)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), userCfg, 0o644)
	before := captureFP(t, root, homePath, projectID, userHook)

	hookproj.SetApplyPhaseHookForTest(func(phase string) error {
		if phase == "before_ownership" {
			return fmt.Errorf("injected before ownership")
		}
		return nil
	})
	t.Cleanup(func() { hookproj.SetApplyPhaseHookForTest(nil) })

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err == nil || !strings.Contains(err.Error(), "injected before ownership") {
		t.Fatalf("err=%v", err)
	}
	if res.Applied || res.Mutation.Applied {
		t.Fatal("must not mark Applied on failure")
	}
	assertUnchanged(t, before, captureFP(t, root, homePath, projectID, userHook))
	if _, err := os.Stat(hookproj.OwnershipPath(homePath, projectID)); !os.IsNotExist(err) {
		t.Fatal("ownership metadata must not persist after rollback")
	}
}

func TestReconcile_FailureAfterOwnershipWrite_RollsBackOwnership(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-rb-own-after"
	userHook := filepath.Join(root, "user-owned.sh")
	_ = os.WriteFile(userHook, []byte("#!/bin/sh\necho user\n"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	userCfg := []byte(`{"version":1,"hooks":{"afterFileEdit":[{"command":"./user-owned.sh"}]}}`)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), userCfg, 0o644)
	before := captureFP(t, root, homePath, projectID, userHook)

	hookproj.SetApplyPhaseHookForTest(func(phase string) error {
		if phase == "after_ownership" {
			return fmt.Errorf("injected after ownership write")
		}
		return nil
	})
	t.Cleanup(func() { hookproj.SetApplyPhaseHookForTest(nil) })

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err == nil || !strings.Contains(err.Error(), "injected after ownership write") {
		t.Fatalf("err=%v", err)
	}
	if res.Applied || res.Mutation.Applied {
		t.Fatal("must not mark Applied on failure")
	}
	assertUnchanged(t, before, captureFP(t, root, homePath, projectID, userHook))
	if _, err := os.Stat(hookproj.OwnershipPath(homePath, projectID)); !os.IsNotExist(err) {
		t.Fatal("ownership metadata must be rolled back")
	}
}

func TestReconcile_FailureDuringRemoval_RollsBack(t *testing.T) {
	root, homePath, projectID, userHookPath, ownership, before := seedWithUserHook(t)

	hookproj.SetApplyPhaseHookForTest(func(phase string) error {
		if phase == "after_config_remove" {
			return fmt.Errorf("injected during removal")
		}
		return nil
	})
	t.Cleanup(func() { hookproj.SetApplyPhaseHookForTest(nil) })

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   nil, // deselect → remove
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  ownership,
	})
	if err == nil || !strings.Contains(err.Error(), "injected during removal") {
		t.Fatalf("err=%v", err)
	}
	if res.Applied {
		t.Fatal("must not apply")
	}
	assertUnchanged(t, before, captureFP(t, root, homePath, projectID, userHookPath))

	// Ownership metadata must remain consistent with pre-removal baseline.
	ownGot, err := os.ReadFile(hookproj.OwnershipPath(homePath, projectID))
	if err != nil {
		t.Fatal(err)
	}
	if fileDigest(t, hookproj.OwnershipPath(homePath, projectID)) != before.ownership {
		t.Fatalf("ownership drifted: %s", ownGot)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	if doc["hooks"].(map[string]any)["afterFileEdit"] == nil {
		t.Fatal("user-owned hook lost during failed removal")
	}
}

func TestReconcile_RepeatedRollback_Idempotent(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-rb-repeat"
	userHook := filepath.Join(root, "user-owned.sh")
	_ = os.WriteFile(userHook, []byte("#!/bin/sh\necho user\n"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	userCfg := []byte(`{"version":1,"hooks":{"afterFileEdit":[{"command":"./user-owned.sh"}]}}`)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), userCfg, 0o644)
	baseline := captureFP(t, root, homePath, projectID, userHook)

	applied, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || !applied.Applied {
		t.Fatalf("apply: err=%v %#v", err, applied)
	}
	if !applied.Mutation.Touched || !applied.Mutation.Applied {
		t.Fatal("successful apply must set Touched and Applied")
	}
	if err := applied.Mutation.Rollback(); err != nil {
		t.Fatal(err)
	}
	if applied.Mutation.Touched || applied.Mutation.Applied {
		t.Fatal("rollback must clear Touched and Applied")
	}
	if err := applied.Mutation.Rollback(); err != nil {
		t.Fatalf("second rollback must be idempotent: %v", err)
	}
	assertUnchanged(t, baseline, captureFP(t, root, homePath, projectID, userHook))
}

func TestReconcile_PartialFailure_PreservesUserHooks(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-rb-user"
	userHook := filepath.Join(root, "user-owned.sh")
	_ = os.WriteFile(userHook, []byte("#!/bin/sh\necho user\n"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	userCfg := []byte(`{
  "version": 1,
  "meta": {"keep": true},
  "hooks": {
    "afterFileEdit": [
      {"command": "./user-owned.sh", "matcher": "Write", "timeout": 9}
    ],
    "stop": [{"command": "./stop.sh"}]
  }
}
`)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), userCfg, 0o644)

	hookproj.SetApplyPhaseHookForTest(func(phase string) error {
		if phase == "after_config" {
			return fmt.Errorf("injected mid-apply")
		}
		return nil
	})
	t.Cleanup(func() { hookproj.SetApplyPhaseHookForTest(nil) })

	_, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	got, _ := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if string(got) != string(userCfg) {
		t.Fatalf("user-owned hooks/config not restored exactly\nwant=%s\ngot=%s", userCfg, got)
	}
}
