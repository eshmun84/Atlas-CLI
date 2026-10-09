package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
	"gopkg.in/yaml.v3"
)

func TestAddCustomRemote_AuthModesAndRoundTrip(t *testing.T) {
	withTempAtlasHome(t)

	cases := []struct {
		name   string
		auth   config.MCPAuthRequirement
		header string
		env    string
		prefix string
	}{
		{name: "none", auth: config.MCPAuthNone},
		{name: "bearer", auth: config.MCPAuthEnvironmentReference, header: "Authorization", env: "GITHUB_TOKEN", prefix: "Bearer "},
		{name: "oauth", auth: config.MCPAuthOAuthExternal},
		{name: "provider", auth: config.MCPAuthProviderManaged},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			draft := config.EmptyMCPDraft()
			server, err := draft.AddCustomRemote("Remote "+tc.name, "https://mcp.example.com/"+tc.name, tc.auth, tc.header, tc.env, tc.prefix)
			if err != nil {
				t.Fatal(err)
			}
			if server.AuthRequirement != tc.auth {
				t.Fatalf("auth = %q", server.AuthRequirement)
			}
			if tc.auth == config.MCPAuthEnvironmentReference {
				ref := server.HeaderRefs["Authorization"]
				if ref.Env != "GITHUB_TOKEN" || ref.Prefix != "Bearer " {
					t.Fatalf("header ref = %#v", ref)
				}
			}
			doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
				ProjectName: "auth-" + tc.name, ProjectMode: "new", ToolCursorAvailable: true,
			}), draft)
			raw, err := yaml.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "ghp_") || strings.Contains(strings.ToLower(string(raw)), "sk-") {
				t.Fatal("secret-like material persisted")
			}
			var loaded config.ProjectDocument
			if err := yaml.Unmarshal(raw, &loaded); err != nil {
				t.Fatal(err)
			}
			round := loaded.ToMCPDraft()
			if len(round.CustomServers) != 1 {
				t.Fatalf("customs = %d", len(round.CustomServers))
			}
			got := round.CustomServers[0]
			if got.AuthRequirement != tc.auth {
				t.Fatalf("round-trip auth = %q", got.AuthRequirement)
			}
			if tc.auth == config.MCPAuthEnvironmentReference {
				ref := got.HeaderRefs["Authorization"]
				if ref.Env != "GITHUB_TOKEN" || ref.Prefix != "Bearer " {
					t.Fatalf("round-trip header = %#v", ref)
				}
			}
		})
	}
}

func TestHeaderValueRef_LegacyStringLoadAndBearerProjectors(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()

	var legacy map[string]mcp.HeaderValueRef
	if err := yaml.Unmarshal([]byte("Authorization: API_TOKEN\n"), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy["Authorization"].Env != "API_TOKEN" || legacy["Authorization"].Prefix != "" {
		t.Fatalf("legacy load = %#v", legacy)
	}

	def := mcp.Definition{
		ID: "custom-1", DisplayName: "Remote", Source: mcp.SourceCustom,
		Transport: mcp.TransportStreamableHTTP, Endpoint: "https://example.com/mcp",
		HeaderRefs:      map[string]mcp.HeaderValueRef{"Authorization": {Env: "API_TOKEN", Prefix: "Bearer "}},
		AuthRequirement: mcp.AuthEnvironmentReference,
		Materializable:  true, Enabled: true,
	}
	cEntry, err := cursor.New().BuildMCPProjection(root, def)
	if err != nil {
		t.Fatal(err)
	}
	if cEntry.Payload["headers"].(map[string]any)["Authorization"] != "Bearer ${env:API_TOKEN}" {
		t.Fatalf("cursor payload %#v", cEntry.Payload)
	}
	oEntry, err := opencode.New().BuildMCPProjection(root, def)
	if err != nil {
		t.Fatal(err)
	}
	if oEntry.Payload["headers"].(map[string]any)["Authorization"] != "Bearer {env:API_TOKEN}" {
		t.Fatalf("opencode payload %#v", oEntry.Payload)
	}
}

func TestInitApply_MCPFailureRollsBackBaseline(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	userREADME := "# existing project\n"
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(userREADME), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	userMCP := []byte(`{"mcpServers":{"user-owned":{"url":"https://user.example/mcp"}}}` + "\n")
	if err := os.WriteFile(filepath.Join(root, cursor.ConfigRelPath), userMCP, 0o644); err != nil {
		t.Fatal(err)
	}

	orig := config.MCPProjectors()
	t.Cleanup(func() {
		for _, p := range orig {
			mcp.RegisterDefaultProjector(p)
		}
	})
	mcp.RegisterDefaultProjector(boomCursor{})
	mcp.RegisterDefaultProjector(opencode.New())

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "init-rb", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)

	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcpDraft,
		Now: func() time.Time { return time.Date(2026, 10, 8, 21, 30, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, config.FileConfig)); !os.IsNotExist(err) {
		t.Fatal(".atlas/config.yaml must not remain after rollback")
	}
	if _, err := os.Stat(filepath.Join(root, config.FileState)); !os.IsNotExist(err) {
		t.Fatal("state.yaml must not mark success after rollback")
	}
	if _, err := os.Stat(filepath.Join(root, config.FileAgentsMD)); !os.IsNotExist(err) {
		t.Fatal("AGENTS.md must be restored/absent after rollback")
	}
	gotMCP, err := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	if err != nil || string(gotMCP) != string(userMCP) {
		t.Fatalf("user MCP must be preserved: %q %v", gotMCP, err)
	}
	gotREADME, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(gotREADME) != userREADME {
		t.Fatalf("user README lost: %q %v", gotREADME, err)
	}
}

type boomCursor struct {
	cursor.Projector
}

func (boomCursor) ApplyMCPProjection(string, []mcp.NativeEntry, []string, []string) error {
	return os.ErrPermission
}
