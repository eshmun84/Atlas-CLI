package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	atlasmcp "github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

// Slice 32 real smoke: Init Cursor+Filesystem, developer MCP survive, GitHub enable,
// idempotence, disable, adapter switch, corrupt block, custom stdio/http, Status/Doctor RO, no CodeGraph MCP.
func TestSlice32MCPExternalCapabilitySmoke(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pass := func(msg string) { t.Log("PASS " + msg) }

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "slice32",
		ProjectMode:         "new",
		ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinFilesystem)
	fixed := time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: mcp, Now: func() time.Time { return fixed }}); err != nil {
		t.Fatal(err)
	}
	cursorMCP := filepath.Join(root, ".cursor/mcp.json")
	raw, err := os.ReadFile(cursorMCP)
	if err != nil || !strings.Contains(string(raw), `"filesystem"`) {
		t.Fatalf("A: filesystem projection missing: %v %s", err, raw)
	}
	pass("A. Init Cursor + Filesystem")

	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	servers := doc["mcpServers"].(map[string]any)
	servers["personal-db"] = map[string]any{"command": "node", "args": []any{"db.js"}}
	doc["mcpServers"] = servers
	out, _ := json.MarshalIndent(doc, "", "  ")
	_ = os.WriteFile(cursorMCP, append(out, '\n'), 0o644)

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "slice32", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "cursor")
	mcpB := config.EmptyMCPDraft()
	mcpB.EnableBuiltin(config.MCPBuiltinFilesystem)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcpB}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(cursorMCP)
	if !strings.Contains(string(raw), "personal-db") {
		t.Fatal("B: developer MCP lost")
	}
	pass("B. Developer-owned MCP survives Configure Apply")

	mcpC := config.EmptyMCPDraft()
	mcpC.EnableBuiltin(config.MCPBuiltinFilesystem)
	mcpC.EnableBuiltin(config.MCPBuiltinGitHub)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcpC}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(cursorMCP)
	if !strings.Contains(string(raw), `"github"`) || strings.Contains(string(raw), "ghp_") {
		t.Fatalf("C: github projection/credentials: %s", raw)
	}
	pass("C. Enable GitHub MCP")

	before := string(raw)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcpC}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(cursorMCP)
	if before != string(after) {
		t.Fatal("D: not idempotent")
	}
	pass("D. Second Apply idempotent")

	mcpE := config.EmptyMCPDraft()
	mcpE.EnableBuiltin(config.MCPBuiltinGitHub)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcpE}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(cursorMCP)
	if strings.Contains(string(raw), `"filesystem"`) || !strings.Contains(string(raw), "personal-db") {
		t.Fatalf("E: %s", raw)
	}
	pass("E. Disable Filesystem removes only Atlas entry")

	cfgOC := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "slice32", ProjectMode: "existing", ToolOpenCodeAvailable: true,
	})
	_ = cfgOC.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgOC, MCP: mcpE}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(cursorMCP)
	if strings.Contains(string(raw), `"github"`) || !strings.Contains(string(raw), "personal-db") {
		t.Fatalf("F cursor: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor/rules/atlas.mdc")); err != nil {
		t.Fatal("F: unrelated cursor config removed")
	}
	oc, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	if err != nil || !strings.Contains(string(oc), `"github"`) {
		t.Fatalf("F opencode: %v %s", err, oc)
	}
	pass("F. Switch Cursor -> OpenCode")

	_ = os.WriteFile(filepath.Join(root, "opencode.json"), []byte("{bad"), 0o644)
	beforeG, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
	disc, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(disc)
	foundMalformed := false
	for _, c := range report.Checks {
		if strings.Contains(c.Name, "mcp") && (c.Severity == doctor.SeverityFail || strings.Contains(c.Message, "malformed")) {
			foundMalformed = true
		}
	}
	if !foundMalformed {
		t.Fatalf("G: doctor missed malformed: %#v", report.Checks)
	}
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgOC, MCP: mcpE})
	if err == nil {
		t.Fatal("G: apply should block")
	}
	afterG, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
	if string(beforeG) != string(afterG) {
		t.Fatal("G: overwritten")
	}
	pass("G. Corrupt native config blocks")

	// Restore valid opencode for remaining steps.
	_ = os.WriteFile(filepath.Join(root, "opencode.json"), []byte("{\n  \"mcp\": {}\n}\n"), 0o644)
	mcpH := config.EmptyMCPDraft()
	if _, err := mcpH.AddCustom("company-stdio", config.MCPTransportStdio, "npx", "-y demo-mcp", "HOME"); err != nil {
		t.Fatal(err)
	}
	mcpH.ToggleCustom(0)
	if _, err := mcpH.AddCustom("company-http", config.MCPTransportStreamableHTTP, "https://mcp.example.com/v1", "", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	mcpH.ToggleCustom(1)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgOC, MCP: mcpH}); err != nil {
		t.Fatal(err)
	}
	oc, _ = os.ReadFile(filepath.Join(root, "opencode.json"))
	if !strings.Contains(string(oc), "company-stdio") || !strings.Contains(string(oc), "company-http") {
		t.Fatalf("H/I customs missing: %s", oc)
	}
	pass("H/I. Custom stdio + streamable_http")

	info, _ := os.Stat(filepath.Join(root, "opencode.json"))
	mtime := info.ModTime()
	bytesBefore, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
	disc, err = inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = doctor.Evaluate(disc)
	if _, err := inspect.Inspect(root); err != nil {
		t.Fatal(err)
	}
	info2, _ := os.Stat(filepath.Join(root, "opencode.json"))
	bytesAfter, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
	if !info2.ModTime().Equal(mtime) || string(bytesBefore) != string(bytesAfter) {
		t.Fatal("J: Status/Doctor mutated files")
	}
	pass("J. Status/Doctor read-only")

	if strings.Contains(string(oc), "codegraph") || strings.Contains(string(raw), "codegraph") {
		t.Fatal("K: CodeGraph MCP projection must not exist")
	}
	pass("K. No CodeGraph MCP projection")

	// L. Multi-adapter induced failure => full rollback
	{
		rootL := t.TempDir()
		draftL := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
			ProjectName: "slice32L", ProjectMode: "new", ToolCursorAvailable: true,
		})
		_ = draftL.ToggleMulti("adapters.selected", "cursor")
		mcpL := config.EmptyMCPDraft()
		mcpL.EnableBuiltin(config.MCPBuiltinFilesystem)
		if _, err := config.ApplyConfig(config.ApplyInput{
			Root: rootL, Draft: draftL, MCP: mcpL,
			Now: func() time.Time { return fixed.Add(time.Hour) },
		}); err != nil {
			t.Fatal(err)
		}
		beforeL, _ := os.ReadFile(filepath.Join(rootL, ".cursor/mcp.json"))
		beforeCfgL, _ := os.ReadFile(filepath.Join(rootL, config.FileConfig))
		orig := config.MCPProjectors()
		atlasmcp.RegisterDefaultProjector(cursor.New())
		atlasmcp.RegisterDefaultProjector(failOC{})
		restoreProjectors := func() {
			for _, p := range orig {
				atlasmcp.RegisterDefaultProjector(p)
			}
		}
		cfgL := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
			ProjectName: "slice32L", ProjectMode: "existing",
			ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
		})
		_ = cfgL.ToggleMulti("adapters.selected", "cursor")
		_ = cfgL.ToggleMulti("adapters.selected", "opencode")
		mcpL2 := config.EmptyMCPDraft()
		mcpL2.EnableBuiltin(config.MCPBuiltinFilesystem)
		mcpL2.EnableBuiltin(config.MCPBuiltinGitHub)
		_, err := config.PersistConfigure(config.ApplyInput{Root: rootL, Draft: cfgL, MCP: mcpL2})
		restoreProjectors()
		if err == nil {
			t.Fatal("L: expected failure")
		}
		afterL, _ := os.ReadFile(filepath.Join(rootL, ".cursor/mcp.json"))
		afterCfgL, _ := os.ReadFile(filepath.Join(rootL, config.FileConfig))
		if string(afterL) != string(beforeL) || string(afterCfgL) != string(beforeCfgL) {
			t.Fatal("L: rollback incomplete")
		}
		pass("L. Multi-adapter induced failure full rollback")
	}

	// M. .cursor symlink escape
	{
		rootM := t.TempDir()
		external := t.TempDir()
		if err := os.Symlink(external, filepath.Join(rootM, ".cursor")); err != nil {
			t.Fatal(err)
		}
		def := mustBuiltinDef(t, "filesystem")
		def.Enabled = true
		_, err := atlasmcp.Reconcile(atlasmcp.ReconcileInput{
			Root: rootM, HomePath: t.TempDir(), ProjectID: "sym",
			Desired:    atlasmcp.DesiredState{Definitions: []atlasmcp.Definition{def}},
			Adapters:   []atlasmcp.AdapterID{atlasmcp.AdapterCursor},
			Projectors: map[atlasmcp.AdapterID]atlasmcp.Projector{atlasmcp.AdapterCursor: cursor.New()},
		})
		if err == nil {
			t.Fatal("M: expected symlink block")
		}
		ents, _ := os.ReadDir(external)
		if len(ents) != 0 {
			t.Fatal("M: external write")
		}
		pass("M. Symlink escape blocked")
	}

	// N. Apply twice => no-op, no workspace backup churn
	{
		beforeN, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
		infoN, _ := os.Stat(filepath.Join(root, "opencode.json"))
		mcpN := config.EmptyMCPDraft()
		if _, err := mcpN.AddCustom("company-stdio", config.MCPTransportStdio, "npx", "-y demo-mcp", "HOME"); err != nil {
			t.Fatal(err)
		}
		mcpN.ToggleCustom(0)
		if _, err := mcpN.AddCustom("company-http", config.MCPTransportStreamableHTTP, "https://mcp.example.com/v1", "", "API_TOKEN"); err != nil {
			t.Fatal(err)
		}
		mcpN.ToggleCustom(1)
		if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgOC, MCP: mcpN}); err != nil {
			t.Fatal(err)
		}
		afterN, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
		infoN2, _ := os.Stat(filepath.Join(root, "opencode.json"))
		if string(beforeN) != string(afterN) || !infoN2.ModTime().Equal(infoN.ModTime()) {
			t.Fatal("N: second apply churned")
		}
		if found, rel, _ := atlasmcp.WorkspaceHasAtlasMCPBackup(root); found {
			t.Fatalf("N: workspace backup %s", rel)
		}
		pass("N. Apply twice no-op")
	}

	// O. OpenCode nested existing config preserved
	{
		rootO := t.TempDir()
		writeJSONSmoke(t, filepath.Join(rootO, "opencode.json"), map[string]any{
			"$schema": "https://opencode.ai/config.json",
			"mcp": map[string]any{
				"servers": map[string]any{
					"user-nested": map[string]any{"type": "local", "command": []any{"echo"}},
				},
			},
		})
		draftO := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
			ProjectName: "slice32O", ProjectMode: "new", ToolOpenCodeAvailable: true,
		})
		_ = draftO.ToggleMulti("adapters.selected", "opencode")
		// Persist nested file after empty init runtime by writing before configure.
		if _, err := config.ApplyConfig(config.ApplyInput{
			Root: rootO, Draft: draftO, MCP: config.EmptyMCPDraft(),
			Now: func() time.Time { return fixed.Add(2 * time.Hour) },
		}); err != nil {
			t.Fatal(err)
		}
		writeJSONSmoke(t, filepath.Join(rootO, "opencode.json"), map[string]any{
			"$schema": "https://opencode.ai/config.json",
			"mcp": map[string]any{
				"servers": map[string]any{
					"user-nested": map[string]any{"type": "local", "command": []any{"echo"}},
				},
			},
		})
		cfgO := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
			ProjectName: "slice32O", ProjectMode: "existing", ToolOpenCodeAvailable: true,
		})
		_ = cfgO.ToggleMulti("adapters.selected", "opencode")
		mcpO := config.EmptyMCPDraft()
		mcpO.EnableBuiltin(config.MCPBuiltinGitHub)
		if _, err := config.PersistConfigure(config.ApplyInput{Root: rootO, Draft: cfgO, MCP: mcpO}); err != nil {
			t.Fatal(err)
		}
		rawO, _ := os.ReadFile(filepath.Join(rootO, "opencode.json"))
		var docO map[string]any
		_ = json.Unmarshal(rawO, &docO)
		mcpObj := docO["mcp"].(map[string]any)
		servers, ok := mcpObj["servers"].(map[string]any)
		if !ok || len(mcpObj) != 1 {
			t.Fatalf("O: nested lost: %s", rawO)
		}
		if _, ok := servers["user-nested"]; !ok {
			t.Fatal("O: user entry lost")
		}
		if _, ok := servers["github"]; !ok {
			t.Fatal("O: github missing")
		}
		pass("O. OpenCode nested representation preserved")
	}

	// P. workspace scan: no Atlas MCP backups / machine-local metadata in repo
	if found, rel, err := atlasmcp.WorkspaceHasAtlasMCPBackup(root); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("P: workspace pollution %s", rel)
	}
	for _, banned := range []string{
		filepath.Join(root, ".cursor", "mcp.json.atlas-backup"),
		filepath.Join(root, "opencode.json.atlas-backup"),
		filepath.Join(root, "mcp", "ownership.yaml"),
	} {
		if _, err := os.Stat(banned); !os.IsNotExist(err) {
			t.Fatalf("P: banned path present: %s", banned)
		}
	}
	pass("P. No Atlas MCP backups/metadata in workspace")
}

type failOC struct {
	opencode.Projector
}

func (failOC) ApplyMCPProjection(string, []atlasmcp.NativeEntry, []string, []string) error {
	return fmt.Errorf("induced failure")
}

func mustBuiltinDef(t *testing.T, id string) atlasmcp.Definition {
	t.Helper()
	def, ok := atlasmcp.BuiltinByID(id)
	if !ok {
		t.Fatalf("missing %s", id)
	}
	return def
}

func writeJSONSmoke(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
