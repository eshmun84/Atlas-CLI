package mcp_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func TestOpenCodeRemoteAuthProjectionMatrix(t *testing.T) {
	t.Parallel()
	proj := opencode.New()
	base := mcp.Definition{
		ID:             "remote-auth",
		DisplayName:    "Remote Auth",
		Source:         mcp.SourceCustom,
		Transport:      mcp.TransportStreamableHTTP,
		Endpoint:       "https://example.com/mcp",
		Materializable: true,
		Enabled:        true,
	}
	cases := []struct {
		name  string
		auth  mcp.AuthRequirement
		check func(t *testing.T, payload map[string]any)
	}{
		{
			name: "none",
			auth: mcp.AuthNone,
			check: func(t *testing.T, payload map[string]any) {
				v, ok := payload["oauth"]
				if !ok || v != false {
					t.Fatalf("AuthNone oauth=%v present=%v want false", v, ok)
				}
			},
		},
		{
			name: "env",
			auth: mcp.AuthEnvironmentReference,
			check: func(t *testing.T, payload map[string]any) {
				v, ok := payload["oauth"]
				if !ok || v != false {
					t.Fatalf("AuthEnvironmentReference oauth=%v want false", v)
				}
			},
		},
		{
			name: "oauth_external",
			auth: mcp.AuthOAuthExternal,
			check: func(t *testing.T, payload map[string]any) {
				v, ok := payload["oauth"].(map[string]any)
				if !ok {
					t.Fatalf("AuthOAuthExternal oauth=%T want object", payload["oauth"])
				}
				if len(v) != 0 {
					t.Fatalf("oauth object should be empty: %#v", v)
				}
			},
		},
		{
			name: "provider_managed",
			auth: mcp.AuthProviderManaged,
			check: func(t *testing.T, payload map[string]any) {
				if _, ok := payload["oauth"]; ok {
					t.Fatalf("AuthProviderManaged must omit oauth, got %#v", payload["oauth"])
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := base
			def.AuthRequirement = tc.auth
			if tc.auth == mcp.AuthEnvironmentReference {
				def.HeaderRefs = map[string]mcp.HeaderValueRef{
					"Authorization": {Env: "TOKEN", Prefix: "Bearer "},
				}
			}
			entry, err := proj.BuildMCPProjection(t.TempDir(), def)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, entry.Payload)
		})
	}
}
