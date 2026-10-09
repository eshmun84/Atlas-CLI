package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestConfigureApply_MCPOnlyAndAgentSwitch(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "mcp-cfg",
		ProjectMode:         "new",
		ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinFilesystem)
	fixed := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: mcp, Now: func() time.Time { return fixed }}); err != nil {
		t.Fatalf("init: %v", err)
	}
	agentsBefore, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	cursorRuleBefore, err := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	if err != nil {
		t.Fatal(err)
	}

	// Developer-owned MCP survives Configure.
	mcpPath := filepath.Join(root, ".cursor/mcp.json")
	raw, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	servers := doc["mcpServers"].(map[string]any)
	servers["personal-db"] = map[string]any{"command": "node", "args": []any{"db.js"}}
	doc["mcpServers"] = servers
	out, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(mcpPath, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:         "mcp-cfg",
		ProjectMode:         "existing",
		ToolCursorAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "cursor")
	mcp2 := config.EmptyMCPDraft()
	mcp2.EnableBuiltin(config.MCPBuiltinFilesystem)
	mcp2.EnableBuiltin(config.MCPBuiltinGitHub)
	res, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcp2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Notice, "MCP selections were reconciled") {
		t.Fatalf("notice=%q", res.Notice)
	}
	agentsAfter, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	cursorRuleAfter, _ := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	if string(agentsAfter) != string(agentsBefore) || string(cursorRuleAfter) != string(cursorRuleBefore) {
		t.Fatal("non-MCP runtime rematerialized")
	}
	raw, _ = os.ReadFile(mcpPath)
	if !strings.Contains(string(raw), "personal-db") || !strings.Contains(string(raw), `"filesystem"`) || !strings.Contains(string(raw), `"github"`) {
		t.Fatalf("cursor mcp merge failed: %s", raw)
	}
	if strings.Contains(strings.ToLower(string(raw)), "ghp_") {
		t.Fatal("credential leaked")
	}

	// Idempotent Configure Apply.
	before := string(raw)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcp2}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(mcpPath)
	if before != string(after) {
		t.Fatal("second configure mutated mcp.json")
	}

	// Disable filesystem only.
	mcp3 := config.EmptyMCPDraft()
	mcp3.EnableBuiltin(config.MCPBuiltinGitHub)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcp3}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(mcpPath)
	if strings.Contains(string(raw), `"filesystem"`) {
		t.Fatal("filesystem should be removed")
	}
	if !strings.Contains(string(raw), "personal-db") || !strings.Contains(string(raw), `"github"`) {
		t.Fatalf("unexpected after disable: %s", raw)
	}

	// Switch Cursor -> OpenCode.
	cfgOC := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:           "mcp-cfg",
		ProjectMode:           "existing",
		ToolOpenCodeAvailable: true,
	})
	_ = cfgOC.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgOC, MCP: mcp3}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(mcpPath)
	if strings.Contains(string(raw), `"github"`) {
		t.Fatal("atlas github should be removed from cursor")
	}
	if !strings.Contains(string(raw), "personal-db") {
		t.Fatal("developer cursor mcp removed")
	}
	ocRaw, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ocRaw), `"github"`) {
		t.Fatalf("opencode missing github: %s", ocRaw)
	}

	// Corrupt native config blocks.
	if err := os.WriteFile(filepath.Join(root, "opencode.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeCorrupt, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgOC, MCP: mcp3})
	if err == nil {
		t.Fatal("expected block")
	}
	afterCorrupt, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
	if string(beforeCorrupt) != string(afterCorrupt) {
		t.Fatal("corrupt file overwritten")
	}
}

func TestLegacySSECustomLoads(t *testing.T) {
	t.Parallel()
	doc := config.ProjectDocument{
		Project: config.ProjectPersist{Name: "legacy", Mode: "existing"},
		Governance: config.GovernancePersist{
			Workflow: "sdd", SpecEngine: "openspec",
			TestingRequired: true, ReviewRequired: true, EvidenceRequired: true,
		},
		Adapters:      config.AdaptersPersist{Selected: []string{"cursor"}},
		SourceControl: config.SourceControlPersist{Mode: "none", DefaultRemote: "origin", BranchStrategy: "manual", GovernanceFiles: "local_only"},
		Memory:        config.MemoryPersist{Strategy: "sqlite_plus_context_capsule"},
		MCP: config.MCPPersist{
			Custom: []config.MCPCustomPersist{{
				Name: "Old SSE", Transport: "sse", CommandOrURL: "https://legacy.example/sse", Enabled: true,
			}},
		},
	}
	draft := doc.ToMCPDraft()
	if draft.CustomServers[0].Transport != config.MCPTransportSSE {
		t.Fatalf("transport = %q", draft.CustomServers[0].Transport)
	}
}
