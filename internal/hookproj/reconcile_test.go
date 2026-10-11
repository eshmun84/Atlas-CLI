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

func TestReconcile_ProjectMergePreserveUser(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hooks-1"

	userHooks := `{
  "version": 1,
  "hooks": {
    "afterFileEdit": [{"command": ".cursor/hooks/user-format.sh"}]
  }
}
`
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), []byte(userHooks), 0o644); err != nil {
		t.Fatal(err)
	}

	proj := cursor.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked || !res.Applied {
		t.Fatalf("res=%#v", res)
	}

	raw, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	hooks := doc["hooks"].(map[string]any)
	if _, ok := hooks["afterFileEdit"]; !ok {
		t.Fatal("user afterFileEdit must be preserved")
	}
	if _, ok := hooks["preToolUse"]; !ok {
		t.Fatal("atlas preToolUse missing")
	}
	wrapper := filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	info, err := os.Stat(wrapper)
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("wrapper missing or not executable: %v mode=%v", err, info)
	}

	// Idempotent second reconcile.
	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  res.Ownership,
	})
	if err != nil || res2.Blocked {
		t.Fatalf("idempotent: err=%v blocked=%v", err, res2.Blocked)
	}
}

func TestReconcile_OwnershipCollision(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hooks-2"
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "hooks", "atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch"), []byte("foreign\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked {
		t.Fatal("expected ownership conflict on unowned wrapper")
	}
	got, _ := os.ReadFile(filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch"))
	if string(got) != "foreign\n" {
		t.Fatalf("user wrapper mutated: %q", got)
	}
}

func TestReconcile_ManagedCommandWithoutOwnershipBlocked(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hooks-3"
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
  "version": 1,
  "hooks": {
    "preToolUse": [{"command": ".cursor/hooks/atlas/dispatch"}]
  }
}
`
	if err := os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked {
		t.Fatal("expected conflict when managed command present without ownership")
	}
}

func TestReconcile_MutationRollback(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hooks-rb"
	userHooks := `{"version":1,"hooks":{"stop":[{"command":"./user.sh"}]}}`
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), []byte(userHooks), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: cursor.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || !res.Applied {
		t.Fatalf("err=%v res=%#v", err, res)
	}
	if err := res.Mutation.Rollback(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != userHooks {
		t.Fatalf("rollback hooks.json=%s", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")); !os.IsNotExist(err) {
		t.Fatalf("wrapper should be removed on rollback, err=%v", err)
	}
}

func TestReconcile_DigestDriftRefusesRemoval(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-hooks-4"
	proj := cursor.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   []string{"cursor"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || res.Blocked {
		t.Fatalf("seed: err=%v blocked=%v", err, res.Blocked)
	}
	wrapper := filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	if err := os.WriteFile(wrapper, []byte("tampered\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   nil, // deselect
		Projectors: map[adapters.ID]hookproj.Projector{adapters.Cursor: proj},
		Ownership:  res.Ownership,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Blocked {
		t.Fatal("expected digest drift block on removal")
	}
	got, _ := os.ReadFile(wrapper)
	if string(got) != "tampered\n" {
		t.Fatalf("tampered wrapper removed: %q", got)
	}
}
