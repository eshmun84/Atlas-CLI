package config_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestEmptyMCPDraftAndAdd(t *testing.T) {
	t.Parallel()

	draft := config.DefaultMCPDraft()
	if len(draft.Servers) != 0 {
		t.Fatalf("default servers = %d, want 0", len(draft.Servers))
	}
	kinds := config.MCPKindTemplates()
	if len(kinds) != 3 {
		t.Fatalf("templates = %d, want 3", len(kinds))
	}
	if kinds[0].Kind != config.MCPKindJira || kinds[1].Kind != config.MCPKindContext7 || kinds[2].Kind != config.MCPKindCustom {
		t.Fatalf("unexpected templates: %#v", kinds)
	}

	server, err := draft.AddServer("My Jira", config.MCPKindJira, "https://example.atlassian.net")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if server.Name != "My Jira" || server.Kind != config.MCPKindJira || server.Enabled {
		t.Fatalf("server = %#v", server)
	}
	if server.ConfigurationStatus != config.MCPNotConfigured {
		t.Fatalf("status = %q", server.ConfigurationStatus)
	}
	if !draft.ToggleEnabled(0) || !draft.Servers[0].Enabled {
		t.Fatal("expected toggle")
	}
	if _, err := draft.AddServer("  ", config.MCPKindCustom, ""); err == nil {
		t.Fatal("empty name must fail")
	}
	if _, err := draft.AddServer("My Jira", config.MCPKindCustom, ""); err == nil {
		t.Fatal("duplicate name must fail")
	}
	if draft.ToggleEnabled(99) {
		t.Fatal("out of range toggle must fail")
	}
	if config.MCPNotConfigured.StatusLabel() != "not configured" {
		t.Fatal("status label")
	}
	if config.MCPKindContext7.KindLabel() != "Context7" {
		t.Fatal("kind label")
	}
}
