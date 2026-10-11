package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestReconcileHookProjections_CursorOnly(t *testing.T) {
	homePath := t.TempDir()
	t.Setenv("ATLAS_HOME", homePath)
	root := t.TempDir()

	doc := config.ProjectDocument{
		Project:  config.ProjectPersist{Name: "hooks-proj"},
		Adapters: config.AdaptersPersist{Selected: []string{"cursor"}},
	}
	res, err := config.ReconcileHookProjections(root, doc, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked {
		t.Fatalf("blocked: %v", res.Errors)
	}
	if !res.Applied {
		t.Fatal("expected apply")
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")); err != nil {
		t.Fatal(err)
	}
	rawHooks, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hj map[string]any
	if err := json.Unmarshal(rawHooks, &hj); err != nil {
		t.Fatal(err)
	}
	hooksObj := hj["hooks"].(map[string]any)
	for _, ev := range []string{"sessionStart", "beforeSubmitPrompt", "preToolUse", "postToolUse", "preCompact"} {
		if _, ok := hooksObj[ev]; !ok {
			t.Fatalf("missing managed event %s in %s", ev, rawHooks)
		}
	}
	if _, ok := hooksObj["stop"]; ok {
		t.Fatal("stop must not be materialized")
	}

	// OpenCode-only selection must not project Cursor hooks.
	docOC := config.ProjectDocument{
		Project:  config.ProjectPersist{Name: "hooks-proj"},
		Adapters: config.AdaptersPersist{Selected: []string{"opencode"}},
	}
	res2, err := config.ReconcileHookProjections(root, docOC, false)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Blocked {
		t.Fatalf("opencode deselect blocked: %v", res2.Errors)
	}

	pid, err := home.ProjectID(root, doc.Project.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(homePath, "projects", pid, "hooks", "ownership.yaml")); err != nil {
		t.Fatal(err)
	}
}
