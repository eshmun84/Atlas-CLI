package hookproj_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
)

func seedCursorHooks(t *testing.T) (root, homePath, projectID string, ownership hookproj.OwnershipDocument, cfgPath, wrapperPath string) {
	t.Helper()
	root = t.TempDir()
	homePath = t.TempDir()
	projectID = "proj-own-hard"
	proj := cursor.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || res.Blocked || !res.Applied {
		t.Fatalf("seed err=%v blocked=%v applied=%v errors=%v", err, res.Blocked, res.Applied, res.Errors)
	}
	cfgPath = filepath.Join(root, ".cursor", "hooks.json")
	wrapperPath = filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	return root, homePath, projectID, res.Ownership, cfgPath, wrapperPath
}

func snapshotFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), raw...)
}

func mutateManagedEntry(t *testing.T, cfgPath string, mut func(map[string]any)) []byte {
	t.Helper()
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	hooks := doc["hooks"].(map[string]any)
	pre := hooks["preToolUse"].([]any)
	var atlasIdx = -1
	for i, e := range pre {
		m := e.(map[string]any)
		if m["command"] == ".cursor/hooks/atlas/dispatch" {
			atlasIdx = i
			break
		}
	}
	if atlasIdx < 0 {
		t.Fatal("managed entry missing")
	}
	entry := pre[atlasIdx].(map[string]any)
	mut(entry)
	pre[atlasIdx] = entry
	hooks["preToolUse"] = pre
	doc["hooks"] = hooks
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(cfgPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}

func expectBlockedNoMutation(t *testing.T, root, homePath, projectID string, ownership hookproj.OwnershipDocument, cfgPath, wrapperPath string, beforeCfg, beforeWrapper []byte) {
	t.Helper()
	ownPath := hookproj.OwnershipPath(homePath, projectID)
	beforeOwn, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  ownership,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked {
		t.Fatalf("expected block, errors=%v actions=%v", res.Errors, res.Actions)
	}
	if res.Applied {
		t.Fatal("blocked reconcile must not apply")
	}
	afterCfg := snapshotFile(t, cfgPath)
	afterWrapper := snapshotFile(t, wrapperPath)
	afterOwn, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCfg) != string(beforeCfg) {
		t.Fatalf("hooks.json mutated on conflict\nbefore=%s\nafter=%s", beforeCfg, afterCfg)
	}
	if string(afterWrapper) != string(beforeWrapper) {
		t.Fatal("wrapper mutated on conflict")
	}
	if string(afterOwn) != string(beforeOwn) {
		t.Fatal("ownership.yaml mutated on conflict")
	}
}

func TestReconcile_ManagedEntryAttributeDriftBlocks(t *testing.T) {
	cases := []struct {
		name string
		mut  func(map[string]any)
	}{
		{name: "matcher", mut: func(e map[string]any) { e["matcher"] = "Shell" }},
		{name: "timeout", mut: func(e map[string]any) { e["timeout"] = 30 }},
		{name: "failClosed", mut: func(e map[string]any) { e["failClosed"] = true }},
		{name: "unknown attribute", mut: func(e map[string]any) { e["futureAttr"] = "x" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, homePath, projectID, ownership, cfgPath, wrapperPath := seedCursorHooks(t)
			beforeWrapper := snapshotFile(t, wrapperPath)
			beforeCfg := mutateManagedEntry(t, cfgPath, tc.mut)
			expectBlockedNoMutation(t, root, homePath, projectID, ownership, cfgPath, wrapperPath, beforeCfg, beforeWrapper)
		})
	}
}

func TestReconcile_RemoveManagedEntryBlocks(t *testing.T) {
	root, homePath, projectID, ownership, cfgPath, wrapperPath := seedCursorHooks(t)
	beforeWrapper := snapshotFile(t, wrapperPath)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	hooks := doc["hooks"].(map[string]any)
	pre := hooks["preToolUse"].([]any)
	var kept []any
	for _, e := range pre {
		m := e.(map[string]any)
		if m["command"] == ".cursor/hooks/atlas/dispatch" {
			continue
		}
		kept = append(kept, e)
	}
	hooks["preToolUse"] = kept
	doc["hooks"] = hooks
	out, _ := json.MarshalIndent(doc, "", "  ")
	out = append(out, '\n')
	if err := os.WriteFile(cfgPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	expectBlockedNoMutation(t, root, homePath, projectID, ownership, cfgPath, wrapperPath, out, beforeWrapper)
}

func TestReconcile_DuplicateManagedEntryBlocks(t *testing.T) {
	root, homePath, projectID, ownership, cfgPath, wrapperPath := seedCursorHooks(t)
	beforeWrapper := snapshotFile(t, wrapperPath)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	hooks := doc["hooks"].(map[string]any)
	pre := hooks["preToolUse"].([]any)
	pre = append(pre, map[string]any{"command": ".cursor/hooks/atlas/dispatch"})
	hooks["preToolUse"] = pre
	doc["hooks"] = hooks
	out, _ := json.MarshalIndent(doc, "", "  ")
	out = append(out, '\n')
	if err := os.WriteFile(cfgPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	expectBlockedNoMutation(t, root, homePath, projectID, ownership, cfgPath, wrapperPath, out, beforeWrapper)
}

func TestReconcile_UserOwnedAndTopLevelChangesAllowed(t *testing.T) {
	root, homePath, projectID, ownership, cfgPath, _ := seedCursorHooks(t)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["teamNote"] = "keep-me"
	hooks := doc["hooks"].(map[string]any)
	hooks["afterFileEdit"] = []any{map[string]any{"command": "./user-fmt.sh", "matcher": "Write"}}
	// Modify an existing user-owned-style adjacent entry by adding a second user command on sessionStart if present,
	// or inject user command alongside managed on a managed event without touching Atlas entry attrs.
	pre := hooks["preToolUse"].([]any)
	pre = append(pre, map[string]any{"command": "./user-pre.sh", "timeout": 5})
	hooks["preToolUse"] = pre
	doc["hooks"] = hooks
	out, _ := json.MarshalIndent(doc, "", "  ")
	out = append(out, '\n')
	if err := os.WriteFile(cfgPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  ownership,
	})
	if err != nil || res.Blocked {
		t.Fatalf("user/top-level changes must not drift ownership: err=%v blocked=%v %v", err, res.Blocked, res.Errors)
	}
	got, _ := os.ReadFile(cfgPath)
	var after map[string]any
	if err := json.Unmarshal(got, &after); err != nil {
		t.Fatal(err)
	}
	if after["teamNote"] != "keep-me" {
		t.Fatal("top-level property lost")
	}
	if after["hooks"].(map[string]any)["afterFileEdit"] == nil {
		t.Fatal("user afterFileEdit lost")
	}
	preAfter := after["hooks"].(map[string]any)["preToolUse"].([]any)
	foundUser := false
	for _, e := range preAfter {
		m := e.(map[string]any)
		if m["command"] == "./user-pre.sh" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Fatal("user preToolUse entry lost")
	}
}

func TestReconcile_IdempotentNoChange(t *testing.T) {
	root, homePath, projectID, ownership, cfgPath, wrapperPath := seedCursorHooks(t)
	beforeCfg := snapshotFile(t, cfgPath)
	beforeWrapper := snapshotFile(t, wrapperPath)
	ownPath := hookproj.OwnershipPath(homePath, projectID)
	beforeOwn := snapshotFile(t, ownPath)

	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  ownership,
	})
	if err != nil || res.Blocked {
		t.Fatalf("idempotent err=%v blocked=%v %v", err, res.Blocked, res.Errors)
	}
	if string(snapshotFile(t, cfgPath)) != string(beforeCfg) {
		t.Fatal("idempotent reconcile mutated hooks.json")
	}
	if string(snapshotFile(t, wrapperPath)) != string(beforeWrapper) {
		t.Fatal("idempotent reconcile mutated wrapper")
	}
	// Ownership UpdatedAt may refresh only when Applied; no-op must not rewrite ownership file.
	if res.Applied {
		t.Fatal("pure no-op must not set Applied")
	}
	if string(snapshotFile(t, ownPath)) != string(beforeOwn) {
		t.Fatal("idempotent reconcile mutated ownership.yaml")
	}
}

func TestReconcile_StrictValidationNoMutation(t *testing.T) {
	cases := []string{
		`null`,
		`[]`,
		`"x"`,
		`{"hooks":{}}`,
		`{"version":1}`,
		`{"version":0,"hooks":{}}`,
		`{"version":1.5,"hooks":{}}`,
		`{"version":2,"hooks":{}}`,
		`{"version":1,"hooks":null}`,
		`{"version":1,"hooks":[]}`,
		`{"version":1,"hooks":"x"}`,
		`{"version":1,"hooks":{"preToolUse":[null]}}`,
		`{"version":1,"hooks":{"preToolUse":["bad"]}}`,
		`{"version":1,"hooks":{}}{"version":1}`,
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			root := t.TempDir()
			homePath := t.TempDir()
			projectID := "proj-val"
			if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, ".cursor", "hooks.json")
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			res, err := hookproj.Reconcile(hookproj.ReconcileInput{
				Root: root, HomePath: homePath, ProjectID: projectID,
				Adapters:   []string{"cursor"},
				Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
				Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !res.Blocked || res.Applied {
				t.Fatalf("want blocked no-apply, got blocked=%v applied=%v errors=%v", res.Blocked, res.Applied, res.Errors)
			}
			got, _ := os.ReadFile(path)
			if string(got) != raw {
				t.Fatalf("validation error mutated file: %q -> %q", raw, got)
			}
			if _, err := os.Stat(filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")); !os.IsNotExist(err) {
				t.Fatal("wrapper must not be created on validation failure")
			}
		})
	}
}
