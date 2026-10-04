package config_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestDefaultMCPDraftBuiltinsAndCustomAdd(t *testing.T) {
	t.Parallel()

	draft := config.DefaultMCPDraft()
	if len(draft.Builtins) != 3 {
		t.Fatalf("builtins = %d, want 3", len(draft.Builtins))
	}
	if draft.Builtins[0].ID != config.MCPBuiltinJira ||
		draft.Builtins[1].ID != config.MCPBuiltinContext7 ||
		draft.Builtins[2].ID != config.MCPBuiltinChromeDevTools {
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

	if !draft.ToggleBuiltin(0) || !draft.Builtins[0].Enabled {
		t.Fatal("toggle jira")
	}
	if !draft.ToggleBuiltin(2) || !draft.Builtins[2].Enabled || !draft.Builtins[0].Enabled {
		t.Fatal("chrome toggle must not unset jira")
	}
	if draft.ToggleBuiltin(99) {
		t.Fatal("out of range builtin toggle")
	}

	server, err := draft.AddCustom("My Browser MCP", config.MCPTransportStdio, "npx", "--yes", "HOME")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if server.Name != "My Browser MCP" || server.Transport != config.MCPTransportStdio || server.Enabled {
		t.Fatalf("server = %#v", server)
	}
	if server.Status != config.MCPStatusInMemoryOnly {
		t.Fatalf("status = %q", server.Status)
	}
	if !draft.ToggleCustom(0) || !draft.CustomServers[0].Enabled {
		t.Fatal("custom toggle")
	}
	if !draft.RemoveCustom(0) || len(draft.CustomServers) != 0 {
		t.Fatal("remove custom")
	}
	if _, err := draft.AddCustom("My Browser MCP", config.MCPTransportStdio, "npx", "--yes", "HOME"); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if _, err := draft.AddCustom("  ", config.MCPTransportHTTP, "", "", ""); err == nil {
		t.Fatal("empty name must fail")
	}
	if _, err := draft.AddCustom("my browser mcp", config.MCPTransportHTTP, "", "", ""); err == nil {
		t.Fatal("duplicate name must fail")
	}
	if _, err := draft.AddCustom("Jira", config.MCPTransportStdio, "", "", ""); err == nil {
		t.Fatal("builtin name must be rejected")
	}
	if draft.ToggleCustom(99) {
		t.Fatal("out of range custom toggle")
	}
	if config.NormalizeTransport("") != config.MCPTransportStdio {
		t.Fatal("default transport")
	}
	if config.StatusLabel(config.MCPStatusInMemoryOnly) != "in memory only" {
		t.Fatal("status label")
	}
}
