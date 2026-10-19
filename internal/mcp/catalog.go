package mcp

import (
	"strings"
	"unicode"
)

// Builtin IDs. Stable for persistence and ownership.
// Managed MCP V1 catalog exposes only Context7 and Chrome DevTools.
// Retired IDs (filesystem, github, jira) remain as constants for legacy YAML
// compatibility and must not appear in BuiltinCatalog / UI.
const (
	BuiltinFilesystem     = "filesystem"
	BuiltinGitHub         = "github"
	BuiltinJira           = "jira"
	BuiltinContext7       = "context7"
	BuiltinChromeDevTools = "chrome_devtools"
)

// WorkspaceFolderToken is substituted by projectors that support Cursor-style interpolation.
const WorkspaceFolderToken = "${workspaceFolder}"

// BuiltinCatalog returns Atlas Managed MCP V1 definitions (all disabled by default).
//
// V1 managed set:
//   - Context7: https://opencode.ai/docs/mcp-servers/ example URL https://mcp.context7.com/mcp
//   - Chrome DevTools: https://github.com/ChromeDevTools/chrome-devtools-mcp
//     command npx -y chrome-devtools-mcp@latest
//
// Retired from Atlas Managed MCP (developer-managed or future skill-assisted setup):
// Filesystem, GitHub, Jira, and arbitrary Custom MCP. Legacy YAML may still carry
// those keys; Atlas does not offer them in Init/Configure UI.
func BuiltinCatalog() []Definition {
	return []Definition{
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
			CompatibilityNote: "Documentation and library context. Optional CONTEXT7_API_KEY for " +
				"Authorization: Bearer <token>. Anonymous use may work with lower rate limits. " +
				"Atlas stores the env reference only.",
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
			CompatibilityNote:   "Browser debugging and web development.",
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

// IsManagedBuiltin reports whether id is part of Atlas Managed MCP V1.
func IsManagedBuiltin(id string) bool {
	_, ok := BuiltinByID(id)
	return ok
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
