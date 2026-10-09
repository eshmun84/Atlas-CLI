package config

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
)

// MCP status values for in-memory drafts / UI.
const (
	MCPStatusNotSelected  = "not_selected"
	MCPStatusConfigured   = "configured"
	MCPStatusAuthRequired = "auth_required"
	MCPStatusNotProjected = "not_projected" // selected but not materializable
	// Legacy aliases kept for older tests/callers.
	MCPStatusInMemoryOnly  = "in_memory_only"
	MCPStatusNotConfigured = "not_configured"
)

// MCPBuiltinID identifies a built-in MCP integration.
type MCPBuiltinID string

const (
	MCPBuiltinFilesystem     MCPBuiltinID = mcp.BuiltinFilesystem
	MCPBuiltinGitHub         MCPBuiltinID = mcp.BuiltinGitHub
	MCPBuiltinJira           MCPBuiltinID = mcp.BuiltinJira
	MCPBuiltinContext7       MCPBuiltinID = mcp.BuiltinContext7
	MCPBuiltinChromeDevTools MCPBuiltinID = mcp.BuiltinChromeDevTools
)

// MCPTransport is a custom MCP transport kind.
type MCPTransport string

const (
	MCPTransportStdio          MCPTransport = MCPTransport(mcp.TransportStdio)
	MCPTransportStreamableHTTP MCPTransport = MCPTransport(mcp.TransportStreamableHTTP)
	// MCPTransportHTTP is a legacy alias accepted on load; normalizes to streamable_http.
	MCPTransportHTTP MCPTransport = "http"
	// MCPTransportSSE is legacy-only (load compatibility); not selectable for new entries.
	MCPTransportSSE MCPTransport = MCPTransport(mcp.TransportSSE)
)

// MCPAuthRequirement mirrors mcp.AuthRequirement for drafts.
type MCPAuthRequirement string

const (
	MCPAuthNone                 MCPAuthRequirement = MCPAuthRequirement(mcp.AuthNone)
	MCPAuthEnvironmentReference MCPAuthRequirement = MCPAuthRequirement(mcp.AuthEnvironmentReference)
	MCPAuthOAuthExternal        MCPAuthRequirement = MCPAuthRequirement(mcp.AuthOAuthExternal)
	MCPAuthProviderManaged      MCPAuthRequirement = MCPAuthRequirement(mcp.AuthProviderManaged)
)

// MCPBuiltinDraft is one built-in selectable MCP integration.
type MCPBuiltinDraft struct {
	ID          MCPBuiltinID
	Name        string
	Enabled     bool
	Description string
	Status      string
	Auth        MCPAuthRequirement
}

// MCPServerDraft is one user-added custom MCP entry.
type MCPServerDraft struct {
	ID                    string
	Name                  string
	Transport             MCPTransport
	CommandOrURL          string
	Arguments             string   // legacy free-form; prefer Args
	Args                  []string // structured args
	EnvironmentReferences string   // legacy free-form; prefer EnvRefs
	EnvRefs               []string
	HeaderRefs            map[string]mcp.HeaderValueRef
	AuthRequirement       MCPAuthRequirement
	Enabled               bool
	Status                string
}

// MCPDraft is the in-memory MCP configuration draft.
type MCPDraft struct {
	Builtins      []MCPBuiltinDraft
	CustomServers []MCPServerDraft
}

// MCPTransports returns selectable custom transport options (V1; no SSE).
func MCPTransports() []MCPTransport {
	return []MCPTransport{MCPTransportStdio, MCPTransportStreamableHTTP}
}

// TransportLabel returns a short display label for a transport.
func (t MCPTransport) TransportLabel() string {
	return mcp.TransportLabel(mcp.Transport(t))
}

// NormalizeTransport maps an input to a known transport.
func NormalizeTransport(value string) MCPTransport {
	return MCPTransport(mcp.NormalizeTransport(value))
}

// DefaultMCPBuiltins returns the built-in MCP catalog, all unselected.
func DefaultMCPBuiltins() []MCPBuiltinDraft {
	out := make([]MCPBuiltinDraft, 0, len(mcp.BuiltinCatalog()))
	for _, def := range mcp.BuiltinCatalog() {
		status := MCPStatusNotSelected
		desc := def.CompatibilityNote
		if desc == "" {
			switch def.ID {
			case mcp.BuiltinFilesystem:
				desc = "Local stdio filesystem access for the project workspace."
			case mcp.BuiltinGitHub:
				desc = "Remote GitHub MCP (Streamable HTTP). Auth via agent OAuth/PAT — Atlas stores no token."
			default:
				desc = def.DisplayName
			}
		}
		out = append(out, MCPBuiltinDraft{
			ID:          MCPBuiltinID(def.ID),
			Name:        def.DisplayName,
			Enabled:     false,
			Description: desc,
			Status:      status,
			Auth:        MCPAuthRequirement(def.AuthRequirement),
		})
	}
	return out
}

// DefaultMCPDraft returns built-ins unselected and no custom entries.
func DefaultMCPDraft() MCPDraft {
	return MCPDraft{
		Builtins:      DefaultMCPBuiltins(),
		CustomServers: nil,
	}
}

// EmptyMCPDraft returns the default in-memory MCP draft.
func EmptyMCPDraft() MCPDraft {
	return DefaultMCPDraft()
}

// SelectedBuiltinCount returns how many built-ins are enabled.
func (d MCPDraft) SelectedBuiltinCount() int {
	n := 0
	for _, item := range d.Builtins {
		if item.Enabled {
			n++
		}
	}
	return n
}

// SelectedCount returns enabled built-ins plus enabled custom entries.
func (d MCPDraft) SelectedCount() int {
	n := d.SelectedBuiltinCount()
	for _, s := range d.CustomServers {
		if s.Enabled {
			n++
		}
	}
	return n
}

// ConfiguredCount is selected built-ins plus custom entries (enabled or listed).
func (d MCPDraft) ConfiguredCount() int {
	return d.SelectedBuiltinCount() + len(d.CustomServers)
}

// ToggleBuiltin flips Enabled for the built-in at index.
func (d *MCPDraft) ToggleBuiltin(index int) bool {
	if index < 0 || index >= len(d.Builtins) {
		return false
	}
	d.Builtins[index].Enabled = !d.Builtins[index].Enabled
	d.Builtins[index].Status = builtinStatus(d.Builtins[index])
	return true
}

// ToggleCustom flips Enabled for the custom server at index.
func (d *MCPDraft) ToggleCustom(index int) bool {
	if index < 0 || index >= len(d.CustomServers) {
		return false
	}
	d.CustomServers[index].Enabled = !d.CustomServers[index].Enabled
	d.CustomServers[index].Status = customStatus(d.CustomServers[index])
	return true
}

// RemoveCustom deletes a custom MCP entry in memory.
func (d *MCPDraft) RemoveCustom(index int) bool {
	if index < 0 || index >= len(d.CustomServers) {
		return false
	}
	d.CustomServers = append(d.CustomServers[:index], d.CustomServers[index+1:]...)
	return true
}

// HasName reports whether a built-in or custom entry already uses name.
func (d MCPDraft) HasName(name string) bool {
	want := strings.TrimSpace(strings.ToLower(name))
	if want == "" {
		return false
	}
	for _, item := range d.Builtins {
		if strings.ToLower(strings.TrimSpace(item.Name)) == want {
			return true
		}
	}
	for _, server := range d.CustomServers {
		if strings.ToLower(strings.TrimSpace(server.Name)) == want {
			return true
		}
	}
	return false
}

// AddCustom validates and appends an in-memory custom MCP entry.
// For streamable_http, envRefs are treated as process-level EnvRefs only
// (no Authorization header). Prefer AddCustomRemote / AddCustomDetailed for auth.
func (d *MCPDraft) AddCustom(name string, transport MCPTransport, commandOrURL, arguments, envRefs string) (MCPServerDraft, error) {
	return d.AddCustomDetailed(name, transport, commandOrURL, mcp.SplitLegacyArgs(arguments), mcp.SplitLegacyEnvRefs(envRefs), nil, "")
}

// AddCustomRemote appends a streamable_http custom MCP with structured auth.
// headerName/envName/prefix apply only when auth is environment_reference.
// Secrets are never accepted — only environment variable names and an optional prefix.
func (d *MCPDraft) AddCustomRemote(
	name string,
	commandOrURL string,
	auth MCPAuthRequirement,
	headerName, envName, prefix string,
) (MCPServerDraft, error) {
	var headers map[string]mcp.HeaderValueRef
	var envRefs []string
	switch auth {
	case MCPAuthEnvironmentReference:
		headerName = strings.TrimSpace(headerName)
		envName = strings.TrimSpace(envName)
		if headerName == "" {
			return MCPServerDraft{}, fmt.Errorf("header name is required for environment header auth")
		}
		if envName == "" {
			return MCPServerDraft{}, fmt.Errorf("environment variable is required for environment header auth")
		}
		headers = map[string]mcp.HeaderValueRef{
			headerName: {Env: envName, Prefix: prefix},
		}
	case MCPAuthNone, MCPAuthOAuthExternal, MCPAuthProviderManaged, "":
		// no headers
	default:
		return MCPServerDraft{}, fmt.Errorf("unknown auth requirement %q", auth)
	}
	return d.AddCustomDetailed(name, MCPTransportStreamableHTTP, commandOrURL, nil, envRefs, headers, auth)
}

// AddCustomDetailed validates and appends a structured custom MCP entry.
func (d *MCPDraft) AddCustomDetailed(
	name string,
	transport MCPTransport,
	commandOrURL string,
	args []string,
	envRefs []string,
	headerRefs map[string]mcp.HeaderValueRef,
	auth MCPAuthRequirement,
) (MCPServerDraft, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MCPServerDraft{}, fmt.Errorf("name is required")
	}
	if d.HasName(name) {
		return MCPServerDraft{}, fmt.Errorf("name already exists")
	}
	transport = NormalizeTransport(string(transport))
	if transport == MCPTransportSSE {
		return MCPServerDraft{}, fmt.Errorf("sse is not a selectable transport; use streamable_http")
	}
	if mcp.NormalizeTransport(string(transport)) == mcp.TransportInvalid {
		return MCPServerDraft{}, fmt.Errorf("unknown transport %q", transport)
	}
	auth = normalizeAuth(auth, transport, envRefs, headerRefs)
	server := MCPServerDraft{
		ID:                    nextCustomMCPID(*d),
		Name:                  name,
		Transport:             transport,
		CommandOrURL:          strings.TrimSpace(commandOrURL),
		Args:                  append([]string(nil), args...),
		Arguments:             strings.Join(args, " "),
		EnvRefs:               append([]string(nil), envRefs...),
		EnvironmentReferences: strings.Join(envRefs, ","),
		HeaderRefs:            copyHeaderRefs(headerRefs),
		AuthRequirement:       auth,
		Enabled:               false,
		Status:                MCPStatusNotSelected,
	}
	// Validate via domain model before accepting.
	spec := mcp.CustomSpec{
		ID:           server.ID,
		Name:         server.Name,
		Transport:    string(server.Transport),
		CommandOrURL: server.CommandOrURL,
		Args:         server.Args,
		EnvRefs:      server.EnvRefs,
		HeaderRefs:   server.HeaderRefs,
		Auth:         mcp.AuthRequirement(server.AuthRequirement),
		Enabled:      true, // validate as if enabled
	}
	if _, err := mcp.BuildDesiredState(mcp.Selection{Custom: []mcp.CustomSpec{spec}}); err != nil {
		return MCPServerDraft{}, err
	}
	server.Enabled = false
	server.Status = MCPStatusNotSelected
	d.CustomServers = append(d.CustomServers, server)
	return server, nil
}

// DesiredState maps the draft onto the agent-neutral MCP desired state.
func (d MCPDraft) DesiredState() (mcp.DesiredState, error) {
	sel := mcp.Selection{
		BuiltinEnabled: map[string]bool{},
	}
	for _, b := range d.Builtins {
		sel.BuiltinEnabled[string(b.ID)] = b.Enabled
	}
	for _, s := range d.CustomServers {
		args := s.Args
		if len(args) == 0 {
			args = mcp.SplitLegacyArgs(s.Arguments)
		}
		envRefs := s.EnvRefs
		if len(envRefs) == 0 {
			envRefs = mcp.SplitLegacyEnvRefs(s.EnvironmentReferences)
		}
		sel.Custom = append(sel.Custom, mcp.CustomSpec{
			ID:           s.ID,
			Name:         s.Name,
			Transport:    string(s.Transport),
			CommandOrURL: s.CommandOrURL,
			Args:         args,
			EnvRefs:      envRefs,
			HeaderRefs:   s.HeaderRefs,
			Auth:         mcp.AuthRequirement(s.AuthRequirement),
			Enabled:      s.Enabled,
		})
	}
	return mcp.BuildDesiredState(sel)
}

func nextCustomMCPID(draft MCPDraft) string {
	n := 1
	for {
		id := fmt.Sprintf("custom-%d", n)
		used := false
		for _, server := range draft.CustomServers {
			if server.ID == id {
				used = true
				break
			}
		}
		if !used {
			return id
		}
		n++
	}
}

// StatusLabel returns a short UI label for a draft status.
func StatusLabel(status string) string {
	switch status {
	case MCPStatusConfigured, MCPStatusInMemoryOnly:
		return "configured"
	case MCPStatusAuthRequired:
		return "auth required"
	case MCPStatusNotProjected:
		return "not projected"
	case MCPStatusNotSelected, MCPStatusNotConfigured:
		return "not selected"
	default:
		return status
	}
}

func builtinStatus(item MCPBuiltinDraft) string {
	if !item.Enabled {
		return MCPStatusNotSelected
	}
	def, ok := mcp.BuiltinByID(string(item.ID))
	if !ok || !def.Materializable {
		return MCPStatusNotProjected
	}
	if def.AuthRequirement == mcp.AuthOAuthExternal || def.AuthRequirement == mcp.AuthEnvironmentReference {
		return MCPStatusAuthRequired
	}
	return MCPStatusConfigured
}

func customStatus(server MCPServerDraft) string {
	if !server.Enabled {
		return MCPStatusNotSelected
	}
	if server.AuthRequirement == MCPAuthOAuthExternal || server.AuthRequirement == MCPAuthEnvironmentReference {
		return MCPStatusAuthRequired
	}
	return MCPStatusConfigured
}

func normalizeAuth(auth MCPAuthRequirement, transport MCPTransport, envRefs []string, headerRefs map[string]mcp.HeaderValueRef) MCPAuthRequirement {
	switch auth {
	case MCPAuthNone, MCPAuthEnvironmentReference, MCPAuthOAuthExternal, MCPAuthProviderManaged:
		return auth
	}
	if transport == MCPTransportStreamableHTTP && (len(envRefs) > 0 || len(headerRefs) > 0) {
		return MCPAuthEnvironmentReference
	}
	return MCPAuthNone
}

func copyHeaderRefs(in map[string]mcp.HeaderValueRef) map[string]mcp.HeaderValueRef {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]mcp.HeaderValueRef, len(in))
	for k, v := range in {
		out[k] = mcp.HeaderValueRef{Env: v.Env, Prefix: v.Prefix}
	}
	return out
}
