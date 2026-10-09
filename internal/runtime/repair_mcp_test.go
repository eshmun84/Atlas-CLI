package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestApplyRuntimeRepair_PreservesAtlasAndUserMCP(t *testing.T) {
	root := materializeProject(t, []string{"cursor", "opencode"}, true)

	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: doc.Project.Name, ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	config.ApplyProjectDocument(&cfg, doc)
	mcpDraft := doc.ToMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)
	if _, err := mcpDraft.AddCustomRemote("company-http", "https://mcp.example.com/v1", config.MCPAuthEnvironmentReference, "Authorization", "API_TOKEN", "Bearer "); err != nil {
		t.Fatal(err)
	}
	for i := range mcpDraft.CustomServers {
		mcpDraft.CustomServers[i].Enabled = true
	}
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcpDraft}); err != nil {
		t.Fatal(err)
	}

	injectUserMCP(t, root, cursor.ConfigRelPath, "mcpServers", "user-cursor")
	injectUserOpenCode(t, root, "user-opencode")
	if err := os.WriteFile(filepath.Join(root, ".cursor", "settings.json"), []byte(`{"editor.fontSize":14}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	_ = applyRepair(t, root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 8, 22, 0, 0, 0, time.UTC)
	})

	assertMCPKey(t, root, cursor.ConfigRelPath, "mcpServers", "filesystem")
	assertMCPKey(t, root, cursor.ConfigRelPath, "mcpServers", "company-http")
	assertMCPKey(t, root, cursor.ConfigRelPath, "mcpServers", "user-cursor")
	if _, err := os.Stat(filepath.Join(root, ".cursor", "settings.json")); err != nil {
		t.Fatal("cursor settings must survive")
	}
	if _, err := os.Stat(filepath.Join(root, opencode.ConfigRelPath)); err != nil {
		t.Fatal("opencode.json must survive")
	}
	assertOpenCodeKey(t, root, "user-opencode")
}

func injectUserMCP(t *testing.T, root, rel, serversKey, userKey string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	servers, _ := doc[serversKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[userKey] = map[string]any{"url": "https://user.example/mcp"}
	doc[serversKey] = servers
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func injectUserOpenCode(t *testing.T, root, userKey string) {
	t.Helper()
	path := filepath.Join(root, opencode.ConfigRelPath)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		doc := map[string]any{"mcp": map[string]any{userKey: map[string]any{"type": "remote", "url": "https://user.example/mcp", "enabled": true}}}
		out, _ := json.MarshalIndent(doc, "", "  ")
		if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	mcpObj, _ := doc["mcp"].(map[string]any)
	if mcpObj == nil {
		mcpObj = map[string]any{}
	}
	if servers, ok := mcpObj["servers"].(map[string]any); ok {
		servers[userKey] = map[string]any{"type": "remote", "url": "https://user.example/mcp", "enabled": true}
		mcpObj["servers"] = servers
	} else {
		mcpObj[userKey] = map[string]any{"type": "remote", "url": "https://user.example/mcp", "enabled": true}
	}
	doc["mcp"] = mcpObj
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertMCPKey(t *testing.T, root, rel, serversKey, key string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	servers, _ := doc[serversKey].(map[string]any)
	if _, ok := servers[key]; !ok {
		t.Fatalf("missing %s in %s: %s", key, rel, raw)
	}
}

func assertOpenCodeKey(t *testing.T, root, key string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	mcpObj, _ := doc["mcp"].(map[string]any)
	if _, ok := mcpObj[key]; ok {
		return
	}
	if servers, ok := mcpObj["servers"].(map[string]any); ok {
		if _, ok := servers[key]; ok {
			return
		}
	}
	t.Fatalf("missing opencode key %s: %s", key, raw)
}
