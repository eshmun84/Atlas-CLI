package hookproj_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/adapters/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
)

func TestReconcile_OpenCodePluginMaterializeIdempotent(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-oc-1"
	proj := opencode.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || res.Blocked || !res.Applied {
		t.Fatalf("err=%v blocked=%v applied=%v %v", err, res.Blocked, res.Applied, res.Errors)
	}
	plugin := filepath.Join(root, ".opencode", "plugins", "atlas-hooks", "index.ts")
	raw, err := os.ReadFile(plugin)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, part := range []string{
		"Plugin.define",
		"atlas.hooks",
		"tool.execute.before",
		"TextEncoder",
		"shouldDenyToolExecution",
		"parseAtlasResponse",
		"SUBPROCESS_TIMEOUT_MS",
		"utf8ByteLength",
	} {
		if !strings.Contains(body, part) {
			t.Fatalf("plugin missing %q", part)
		}
	}
	if strings.Contains(body, `from "@opencode/plugin"`) || strings.Contains(body, `from '@opencode/plugin'`) {
		t.Fatal("project-local plugin must not import @opencode/plugin (OpenCode v2.0.20 does not resolve it)")
	}
	if strings.Contains(body, "body.length > MAX_BYTES") {
		t.Fatal("plugin still uses UTF-16 string.length for byte limit")
	}
	if strings.Contains(body, `res.decision === "deny" || res.status === "denied"`) {
		t.Fatal("plugin must not deny on status alone")
	}

	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: proj},
		Ownership:  res.Ownership,
	})
	if err != nil || res2.Blocked {
		t.Fatalf("idempotent: %v %v", err, res2.Errors)
	}
	if res2.Applied {
		t.Fatal("idempotent must not re-apply")
	}
	raw2, _ := os.ReadFile(plugin)
	if string(raw2) != string(raw) {
		t.Fatal("idempotent mutated plugin")
	}
}

func TestReconcile_OpenCodeUserPluginCollision(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-oc-2"
	plugin := filepath.Join(root, ".opencode", "plugins", "atlas-hooks", "index.ts")
	if err := os.MkdirAll(filepath.Dir(plugin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plugin, []byte("// user owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Neighbor user plugin must survive.
	other := filepath.Join(root, ".opencode", "plugins", "user-plugin.ts")
	if err := os.WriteFile(other, []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: opencode.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked {
		t.Fatal("expected collision block")
	}
	got, _ := os.ReadFile(plugin)
	if string(got) != "// user owned\n" {
		t.Fatalf("user plugin overwritten: %q", got)
	}
	gotOther, _ := os.ReadFile(other)
	if string(gotOther) != "export default {}\n" {
		t.Fatal("neighbor plugin mutated")
	}
}

func TestReconcile_OpenCodeDigestDriftBlocks(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-oc-3"
	proj := opencode.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || !res.Applied {
		t.Fatalf("seed: %v %#v", err, res)
	}
	plugin := filepath.Join(root, ".opencode", "plugins", "atlas-hooks", "index.ts")
	if err := os.WriteFile(plugin, []byte("// tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: proj},
		Ownership:  res.Ownership,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Blocked {
		t.Fatal("expected digest drift block")
	}
	got, _ := os.ReadFile(plugin)
	if string(got) != "// tampered\n" {
		t.Fatal("tampered plugin overwritten")
	}
}

func TestReconcile_OpenCodeSymlinkBlocked(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-oc-4"
	target := filepath.Join(root, "outside.ts")
	_ = os.WriteFile(target, []byte("x\n"), 0o644)
	plugin := filepath.Join(root, ".opencode", "plugins", "atlas-hooks", "index.ts")
	if err := os.MkdirAll(filepath.Dir(plugin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, plugin); err != nil {
		t.Fatal(err)
	}
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: opencode.NewHooksProjector()},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked {
		t.Fatal("symlink must block")
	}
}

func TestReconcile_OpenCodeRemovalAndRollback(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "proj-oc-5"
	other := filepath.Join(root, ".opencode", "plugins", "keep.ts")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := opencode.NewHooksProjector()
	res, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || !res.Applied {
		t.Fatalf("seed: %v %#v", err, res)
	}
	if err := res.Mutation.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "plugins", "atlas-hooks", "index.ts")); !os.IsNotExist(err) {
		t.Fatal("plugin should be removed on rollback")
	}
	got, _ := os.ReadFile(other)
	if string(got) != "keep\n" {
		t.Fatal("user plugin lost on rollback")
	}

	// Re-apply then deselect removal.
	res2, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   []string{"opencode"},
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: proj},
		Ownership:  hookproj.OwnershipDocument{SchemaVersion: 1, ProjectID: projectID},
	})
	if err != nil || !res2.Applied {
		t.Fatalf("reseed: %v %#v", err, res2)
	}
	res3, err := hookproj.Reconcile(hookproj.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: projectID,
		Adapters:   nil,
		Projectors: map[adapters.ID]hookproj.Projector{adapters.OpenCode: proj},
		Ownership:  res2.Ownership,
	})
	if err != nil || res3.Blocked {
		t.Fatalf("remove: %v %v", err, res3.Errors)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "plugins", "atlas-hooks", "index.ts")); !os.IsNotExist(err) {
		t.Fatal("plugin must be removed on deselect")
	}
	got, _ = os.ReadFile(other)
	if string(got) != "keep\n" {
		t.Fatal("neighbor plugin lost on remove")
	}
}
