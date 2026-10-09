package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func TestEvaluateHealth_UnownedEqualPayload_NotMaterialized(t *testing.T) {
	for _, tc := range []struct {
		name      string
		adapter   mcp.AdapterID
		projector mcp.Projector
		rel       string
		wrap      func(key string, payload map[string]any) map[string]any
	}{
		{
			name: "cursor", adapter: mcp.AdapterCursor, projector: cursor.New(), rel: cursor.ConfigRelPath,
			wrap: func(key string, payload map[string]any) map[string]any {
				return map[string]any{"mcpServers": map[string]any{key: payload}}
			},
		},
		{
			name: "opencode", adapter: mcp.AdapterOpenCode, projector: opencode.New(), rel: opencode.ConfigRelPath,
			wrap: func(key string, payload map[string]any) map[string]any {
				return map[string]any{"mcp": map[string]any{key: payload}}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			homePath := t.TempDir()
			pid := "health-own"
			def, ok := mcp.BuiltinByID(mcp.BuiltinGitHub)
			if !ok {
				t.Fatal("github missing")
			}
			def.Enabled = true
			entry, err := tc.projector.BuildMCPProjection(root, def)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, tc.rel)), 0o755); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.MarshalIndent(tc.wrap(entry.Key, entry.Payload), "", "  ")
			if err := os.WriteFile(filepath.Join(root, tc.rel), append(raw, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err = mcp.Reconcile(mcp.ReconcileInput{
				Root: root, HomePath: homePath, ProjectID: pid,
				Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
				Adapters:   []mcp.AdapterID{tc.adapter},
				Projectors: map[mcp.AdapterID]mcp.Projector{tc.adapter: tc.projector},
			})
			if err == nil || !strings.Contains(err.Error(), "not Atlas-owned") {
				t.Fatalf("expected reconcile ownership block, got %v", err)
			}

			h := mcp.EvaluateHealth(root, homePath, pid, mcp.DesiredState{Definitions: []mcp.Definition{def}},
				[]mcp.AdapterID{tc.adapter}, map[mcp.AdapterID]mcp.Projector{tc.adapter: tc.projector})
			if h.Ready {
				t.Fatal("health must not be Ready under ownership conflict")
			}
			if len(h.Adapters) != 1 {
				t.Fatalf("adapters: %#v", h.Adapters)
			}
			ah := h.Adapters[0]
			if ah.Status != mcp.ProjectionBlocked {
				t.Fatalf("status=%s want blocked", ah.Status)
			}
			if ah.Materialized != 0 {
				t.Fatalf("materialized=%d want 0", ah.Materialized)
			}
			if len(ah.OwnershipConflict) == 0 {
				t.Fatal("expected OwnershipConflict keys")
			}
		})
	}
}

func TestDoctor_OwnershipConflictFailsWithoutMaterializedPass(t *testing.T) {
	homePath := t.TempDir()
	t.Setenv("ATLAS_HOME", homePath)
	root := t.TempDir()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "doc-own", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcpDraft,
		Now: func() time.Time { return time.Date(2026, 10, 8, 21, 30, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatal(err)
	}

	// Replace Atlas-owned projection with equal payload but wipe ownership.
	def, _ := mcp.BuiltinByID(mcp.BuiltinFilesystem)
	def.Enabled = true
	proj := cursor.New()
	entry, err := proj.BuildMCPProjection(root, def)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{entry.Key: entry.Payload}}, "", "  ")
	if err := os.WriteFile(filepath.Join(root, cursor.ConfigRelPath), append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, err := filepath.Glob(filepath.Join(homePath, "projects", "*"))
	if err != nil || len(pid) != 1 {
		t.Fatalf("project home: %v %v", pid, err)
	}
	ownPath := filepath.Join(pid[0], "mcp", "ownership.yaml")
	if err := os.Remove(ownPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(insp)
	var sawFailOwnership bool
	var sawPassMaterialized bool
	for _, c := range report.Checks {
		if c.Severity == doctor.SeverityFail && strings.Contains(c.Name, "ownership") {
			sawFailOwnership = true
		}
		if c.Severity == doctor.SeverityPass && strings.Contains(c.Message, "materialized") && strings.HasPrefix(c.Name, "mcp ") {
			sawPassMaterialized = true
		}
	}
	if !sawFailOwnership {
		t.Fatalf("expected FAIL ownership conflict; checks=%#v", report.Checks)
	}
	if sawPassMaterialized {
		t.Fatal("ownership conflict must not PASS as materialized")
	}
}
