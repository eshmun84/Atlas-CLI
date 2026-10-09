package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

// Finding 4: never silently adopt user-owned keys even when payload equals desired.
func TestReconcile_BlocksUnownedEqualPayload_CursorAndOpenCode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		adapter   mcp.AdapterID
		projector mcp.Projector
		rel       string
		wrap      func(key string, payload map[string]any) map[string]any
	}{
		{
			name:      "cursor",
			adapter:   mcp.AdapterCursor,
			projector: cursor.New(),
			rel:       cursor.ConfigRelPath,
			wrap: func(key string, payload map[string]any) map[string]any {
				return map[string]any{"mcpServers": map[string]any{key: payload}}
			},
		},
		{
			name:      "opencode",
			adapter:   mcp.AdapterOpenCode,
			projector: opencode.New(),
			rel:       opencode.ConfigRelPath,
			wrap: func(key string, payload map[string]any) map[string]any {
				return map[string]any{"mcp": map[string]any{key: payload}}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			homePath := t.TempDir()
			pid := "own-conflict"
			def, ok := mcp.BuiltinByID(mcp.BuiltinGitHub)
			if !ok {
				t.Fatal("github builtin missing")
			}
			def.Enabled = true
			entry, err := tc.projector.BuildMCPProjection(root, def)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, tc.rel)), 0o755); err != nil {
				t.Fatal(err)
			}
			writeJSON(t, filepath.Join(root, tc.rel), tc.wrap(entry.Key, entry.Payload))
			before, err := os.ReadFile(filepath.Join(root, tc.rel))
			if err != nil {
				t.Fatal(err)
			}
			ownBefore, err := mcp.LoadOwnership(homePath, pid)
			if err != nil {
				t.Fatal(err)
			}
			if len(ownBefore.Adapters) != 0 {
				t.Fatalf("ownership should start empty: %#v", ownBefore)
			}

			res, err := mcp.Reconcile(mcp.ReconcileInput{
				Root: root, HomePath: homePath, ProjectID: pid,
				Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
				Adapters:   []mcp.AdapterID{tc.adapter},
				Projectors: map[mcp.AdapterID]mcp.Projector{tc.adapter: tc.projector},
				Ownership:  ownBefore,
			})
			if err != nil && !res.Blocked && !strings.Contains(err.Error(), "not Atlas-owned") {
				t.Fatal(err)
			}
			if !res.Blocked && (err == nil || !strings.Contains(err.Error(), "not Atlas-owned")) {
				t.Fatalf("expected ownership conflict block, got res=%#v err=%v", res, err)
			}
			after, err := os.ReadFile(filepath.Join(root, tc.rel))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("native file mutated:\n%s", after)
			}
			ownAfter, err := mcp.LoadOwnership(homePath, pid)
			if err != nil {
				t.Fatal(err)
			}
			if len(ownAfter.AdapterEntries(tc.adapter)) != 0 {
				t.Fatalf("ownership must remain unchanged: %#v", ownAfter)
			}

			// Subsequent disable must not delete the user-owned key.
			def.Enabled = false
			res2, err := mcp.Reconcile(mcp.ReconcileInput{
				Root: root, HomePath: homePath, ProjectID: pid,
				Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
				Adapters:   []mcp.AdapterID{tc.adapter},
				Projectors: map[mcp.AdapterID]mcp.Projector{tc.adapter: tc.projector},
				Ownership:  ownAfter,
			})
			if err != nil {
				t.Fatal(err)
			}
			if res2.Blocked {
				t.Fatalf("disable with empty ownership should not block: %#v", res2)
			}
			final, err := os.ReadFile(filepath.Join(root, tc.rel))
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal(final, &doc); err != nil {
				t.Fatal(err)
			}
			servers := extractServers(t, tc.name, doc)
			if _, ok := servers[entry.Key]; !ok {
				t.Fatalf("user-owned %s key must survive disable: %s", entry.Key, final)
			}
		})
	}
}

func extractServers(t *testing.T, kind string, doc map[string]any) map[string]any {
	t.Helper()
	switch kind {
	case "cursor":
		servers, ok := doc["mcpServers"].(map[string]any)
		if !ok {
			t.Fatalf("cursor servers missing: %#v", doc)
		}
		return servers
	case "opencode":
		mcpObj, ok := doc["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("opencode mcp missing: %#v", doc)
		}
		// Flat or nested servers shape.
		if servers, ok := mcpObj["servers"].(map[string]any); ok {
			return servers
		}
		return mcpObj
	default:
		t.Fatalf("unknown kind %s", kind)
		return nil
	}
}
