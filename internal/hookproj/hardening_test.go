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

func TestReconcile_PreservesUnknownAndMultipleUserCommands(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hard-1"
	raw := `{
  "version": 1,
  "meta": {"team": "platform"},
  "hooks": {
    "beforeSubmitPrompt": [
      {"command": "./a.sh", "failClosed": true},
      {"command": "./b.sh", "matcher": "UserPromptSubmit"}
    ]
  }
}`
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := cursor.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || res.Blocked {
		t.Fatalf("err=%v blocked=%v errors=%v", err, res.Blocked, res.Errors)
	}
	out, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["meta"] == nil {
		t.Fatal("meta preserved")
	}
	entries := doc["hooks"].(map[string]any)["beforeSubmitPrompt"].([]any)
	if len(entries) != 3 {
		t.Fatalf("want 2 user + 1 atlas, got %d", len(entries))
	}
	// Repeated reconcile must remain unblocked and preserve user commands.
	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  res.Ownership,
	})
	if err != nil || res2.Blocked {
		t.Fatalf("idempotent err=%v blocked=%v", err, res2.Blocked)
	}
	out2, _ := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	var doc2 map[string]any
	_ = json.Unmarshal(out2, &doc2)
	entries2 := doc2["hooks"].(map[string]any)["beforeSubmitPrompt"].([]any)
	if len(entries2) != 3 {
		t.Fatalf("idempotent changed entries: %d", len(entries2))
	}
}

func TestReconcile_OwnedWrapperDigestDriftBlocksUpdate(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hard-2"
	proj := cursor.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || !res.Applied {
		t.Fatalf("seed err=%v res=%#v", err, res)
	}
	wrapper := filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	if err := os.WriteFile(wrapper, []byte("user-edited\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  res.Ownership,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Blocked {
		t.Fatal("expected digest drift block on update")
	}
	got, _ := os.ReadFile(wrapper)
	if string(got) != "user-edited\n" {
		t.Fatalf("modified wrapper overwritten: %q", got)
	}
}

func TestReconcile_UserCanExtendHooksWithoutDrift(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hard-3"
	proj := cursor.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || !res.Applied {
		t.Fatalf("seed: %v %#v", err, res)
	}
	// User adds a new event — managed fingerprint unchanged.
	cfgPath := filepath.Join(root, ".cursor", "hooks.json")
	raw, _ := os.ReadFile(cfgPath)
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	hooks := doc["hooks"].(map[string]any)
	hooks["afterFileEdit"] = []any{map[string]any{"command": "./fmt.sh"}}
	doc["hooks"] = hooks
	updated, _ := json.MarshalIndent(doc, "", "  ")
	updated = append(updated, '\n')
	if err := os.WriteFile(cfgPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  res.Ownership,
	})
	if err != nil || res2.Blocked {
		t.Fatalf("user extension should not drift managed ownership: err=%v blocked=%v %v", err, res2.Blocked, res2.Errors)
	}
	raw2, _ := os.ReadFile(cfgPath)
	var doc2 map[string]any
	_ = json.Unmarshal(raw2, &doc2)
	if doc2["hooks"].(map[string]any)["afterFileEdit"] == nil {
		t.Fatal("user afterFileEdit lost")
	}
}

func TestReconcile_SymlinkWrapperBlocked(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hard-4"
	target := filepath.Join(root, "outside-dispatch")
	if err := os.WriteFile(target, []byte("x\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "hooks", "atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	if err := os.Symlink(target, link); err != nil {
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
	if !res.Blocked {
		t.Fatal("symlink wrapper must block")
	}
}

func TestReconcile_MalformedJSONNoMutation(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hard-5"
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{"version":1,`)
	path := filepath.Join(root, ".cursor", "hooks.json")
	if err := os.WriteFile(path, bad, 0o644); err != nil {
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
	if !res.Blocked {
		t.Fatal("malformed must block")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(bad) {
		t.Fatal("malformed file mutated")
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")); !os.IsNotExist(err) {
		t.Fatal("wrapper must not be created on malformed config")
	}
}
