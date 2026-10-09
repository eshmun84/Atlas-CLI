package mcp

import (
	"strings"
	"unicode"
)

// Builtin IDs. Stable for persistence and ownership.
const (
	BuiltinFilesystem     = "filesystem"
	BuiltinGitHub         = "github"
	BuiltinJira           = "jira"
	BuiltinContext7       = "context7"
	BuiltinChromeDevTools = "chrome_devtools"
)

// WorkspaceFolderToken is substituted by projectors that support Cursor-style interpolation.
const WorkspaceFolderToken = "${workspaceFolder}"

// BuiltinCatalog returns declarative built-in MCP definitions (all disabled).
//
// Verified sources (Slice 32):
//   - Filesystem: https://github.com/modelcontextprotocol/servers (src/filesystem/README.md)
//     command npx -y @modelcontextprotocol/server-filesystem <allowed-dirs>
//   - GitHub remote: https://github.com/github/github-mcp-server/docs/remote-server.md
//     and GitHub Docs "Setting up the GitHub MCP Server"
//     URL https://api.githubcopilot.com/mcp/ ; OAuth/PAT via client (Atlas stores no token)
//   - Context7: https://opencode.ai/docs/mcp-servers/ example URL https://mcp.context7.com/mcp
//   - Chrome DevTools: https://github.com/ChromeDevTools/chrome-devtools-mcp
//     command npx -y chrome-devtools-mcp@latest
//   - Jira: no verified public Atlassian MCP endpoint hardcoded; selection persists, not materializable.
func BuiltinCatalog() []Definition {
	return []Definition{
		{
			ID:                  BuiltinFilesystem,
			DisplayName:         "Filesystem",
			Source:              SourceBuiltin,
			Transport:           TransportStdio,
			Command:             "npx",
			Args:                []string{"-y", "@modelcontextprotocol/server-filesystem", WorkspaceFolderToken},
			AuthRequirement:     AuthNone,
			Materializable:      true,
			PrerequisiteCommand: "npx",
		},
		{
			ID:              BuiltinGitHub,
			DisplayName:     "GitHub",
			Source:          SourceBuiltin,
			Transport:       TransportStreamableHTTP,
			Endpoint:        "https://api.githubcopilot.com/mcp/",
			AuthRequirement: AuthOAuthExternal,
			Materializable:  true,
			CompatibilityNote: "Remote Streamable HTTP; authenticate via the agent OAuth/PAT flow. " +
				"Atlas never stores GitHub tokens.",
		},
		{
			ID:              BuiltinJira,
			DisplayName:     "Jira",
			Source:          SourceBuiltin,
			Transport:       TransportStreamableHTTP,
			AuthRequirement: AuthOAuthExternal,
			Materializable:  false,
			CompatibilityNote: "Selection persisted for compatibility. No verified public Atlassian MCP " +
				"endpoint is hardcoded in Slice 32; not projected until a verified definition exists.",
		},
		{
			ID:          BuiltinContext7,
			DisplayName: "Context7",
			Source:      SourceBuiltin,
			Transport:   TransportStreamableHTTP,
			Endpoint:    "https://mcp.context7.com/mcp",
			HeaderRefs: map[string]HeaderValueRef{
				"Authorization": {Env: "CONTEXT7_API_KEY", Prefix: "Bearer "},
			},
			AuthRequirement: AuthEnvironmentReference,
			Materializable:  true,
			CompatibilityNote: "Optional CONTEXT7_API_KEY for Authorization: Bearer <token>. " +
				"Anonymous use may work with lower rate limits. Atlas stores the env reference only.",
		},
		{
			ID:                  BuiltinChromeDevTools,
			DisplayName:         "Chrome DevTools",
			Source:              SourceBuiltin,
			Transport:           TransportStdio,
			Command:             "npx",
			Args:                []string{"-y", "chrome-devtools-mcp@latest"},
			AuthRequirement:     AuthNone,
			Materializable:      true,
			PrerequisiteCommand: "npx",
		},
	}
}

// BuiltinByID returns a copy of a catalog entry.
func BuiltinByID(id string) (Definition, bool) {
	for _, d := range BuiltinCatalog() {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

// NativeKeyFor returns the stable native config key for a definition.
// Builtins use their catalog ID. Customs prefer a sanitized display name for
// readable agent configs, falling back to the generated custom ID.
func NativeKeyFor(def Definition) string {
	if def.Source == SourceCustom {
		if key := SanitizeNativeKey(def.DisplayName); key != "" && key != "mcp" {
			return key
		}
	}
	return SanitizeNativeKey(def.ID)
}

// SanitizeNativeKey produces a safe MCP server key.
func SanitizeNativeKey(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return "mcp"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "mcp"
	}
	return out
}
