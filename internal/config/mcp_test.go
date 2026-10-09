package config_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestDefaultMCPDraftBuiltinsAndCustomAdd(t *testing.T) {
	t.Parallel()

	draft := config.DefaultMCPDraft()
	if len(draft.Builtins) != 5 {
		t.Fatalf("builtins = %d, want 5", len(draft.Builtins))
	}
	if draft.Builtins[0].ID != config.MCPBuiltinFilesystem ||
		draft.Builtins[1].ID != config.MCPBuiltinGitHub ||
		draft.Builtins[2].ID != config.MCPBuiltinJira ||
		draft.Builtins[3].ID != config.MCPBuiltinContext7 ||
		draft.Builtins[4].ID != config.MCPBuiltinChromeDevTools {
		t.Fatalf("unexpected builtins: %#v", draft.Builtins)
	}
	for _, item := range draft.Builtins {
		if item.Enabled {
			t.Fatalf("builtin %s should start disabled", item.Name)
		}
	}
	if len(draft.CustomServers) != 0 || draft.ConfiguredCount() != 0 {
		t.Fatalf("custom/configured = %d/%d", len(draft.CustomServers), draft.ConfiguredCount())
	}

	if !draft.EnableBuiltin(config.MCPBuiltinJira) || !draft.Builtins[draft.BuiltinIndex(config.MCPBuiltinJira)].Enabled {
		t.Fatal("toggle jira")
	}
	if !draft.EnableBuiltin(config.MCPBuiltinChromeDevTools) {
		t.Fatal("chrome toggle")
	}
	if !draft.Builtins[draft.BuiltinIndex(config.MCPBuiltinJira)].Enabled {
		t.Fatal("chrome toggle must not unset jira")
	}
	if draft.ToggleBuiltin(99) {
		t.Fatal("out of range builtin toggle")
	}

	server, err := draft.AddCustom("My Browser MCP", config.MCPTransportStdio, "npx", "--yes pkg", "HOME")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if server.Name != "My Browser MCP" || server.Transport != config.MCPTransportStdio || server.Enabled {
		t.Fatalf("server = %#v", server)
	}
	if server.Status != config.MCPStatusNotSelected {
		t.Fatalf("status = %q", server.Status)
	}
	if !draft.ToggleCustom(0) || !draft.CustomServers[0].Enabled {
		t.Fatal("custom toggle")
	}
	if draft.CustomServers[0].Status != config.MCPStatusConfigured && draft.CustomServers[0].Status != config.MCPStatusAuthRequired {
		t.Fatalf("enabled status = %q", draft.CustomServers[0].Status)
	}
	if !draft.RemoveCustom(0) || len(draft.CustomServers) != 0 {
		t.Fatal("remove custom")
	}
	if _, err := draft.AddCustom("My Browser MCP", config.MCPTransportStdio, "npx", "--yes pkg", "HOME"); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if _, err := draft.AddCustom("  ", config.MCPTransportStreamableHTTP, "https://example.com/mcp", "", ""); err == nil {
		t.Fatal("empty name must fail")
	}
	if _, err := draft.AddCustom("my browser mcp", config.MCPTransportStreamableHTTP, "https://example.com/mcp", "", ""); err == nil {
		t.Fatal("duplicate name must fail")
	}
	if _, err := draft.AddCustom("Jira", config.MCPTransportStdio, "npx", "", ""); err == nil {
		t.Fatal("builtin name must be rejected")
	}
	if _, err := draft.AddCustom("Legacy SSE", config.MCPTransportSSE, "https://example.com/mcp", "", ""); err == nil {
		t.Fatal("sse must not be selectable for new customs")
	}
	if draft.ToggleCustom(99) {
		t.Fatal("out of range custom toggle")
	}
	if config.NormalizeTransport("") != config.MCPTransportStdio {
		t.Fatal("default transport")
	}
	if config.NormalizeTransport("http") != config.MCPTransportStreamableHTTP {
		t.Fatal("http normalizes to streamable_http")
	}
	if config.NormalizeTransport("sse") != config.MCPTransportSSE {
		t.Fatal("legacy sse load")
	}
	if config.StatusLabel(config.MCPStatusConfigured) != "configured" {
		t.Fatal("status label")
	}
	if len(config.MCPTransports()) != 2 {
		t.Fatal("selectable transports")
	}
}
