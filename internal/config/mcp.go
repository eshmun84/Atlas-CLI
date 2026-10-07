package config

import (
	"fmt"
	"strings"
)

// MCP status values for in-memory drafts.
const (
	MCPStatusInMemoryOnly  = "in_memory_only"
	MCPStatusNotConfigured = "not_configured"
)

// MCPBuiltinID identifies a built-in MCP integration.
type MCPBuiltinID string

const (
	MCPBuiltinJira           MCPBuiltinID = "jira"
	MCPBuiltinContext7       MCPBuiltinID = "context7"
	MCPBuiltinChromeDevTools MCPBuiltinID = "chrome_devtools"
)

// MCPTransport is a custom MCP transport kind.
type MCPTransport string

const (
	MCPTransportStdio MCPTransport = "stdio"
	MCPTransportHTTP  MCPTransport = "http"
	MCPTransportSSE   MCPTransport = "sse"
)

// MCPBuiltinDraft is one built-in selectable MCP integration.
type MCPBuiltinDraft struct {
	ID          MCPBuiltinID
	Name        string
	Enabled     bool
	Description string
	Status      string
}

// MCPServerDraft is one user-added custom MCP entry.
type MCPServerDraft struct {
	ID                    string
	Name                  string
	Transport             MCPTransport
	CommandOrURL          string
	Arguments             string
	EnvironmentReferences string
	Enabled               bool
	Status                string
}

// MCPDraft is the in-memory MCP configuration draft.
type MCPDraft struct {
	Builtins      []MCPBuiltinDraft
	CustomServers []MCPServerDraft
}

// MCPTransports returns selectable custom transport options.
func MCPTransports() []MCPTransport {
	return []MCPTransport{MCPTransportStdio, MCPTransportHTTP, MCPTransportSSE}
}

// TransportLabel returns a short display label for a transport.
func (t MCPTransport) TransportLabel() string {
	switch t {
	case MCPTransportHTTP:
		return "http"
	case MCPTransportSSE:
		return "sse"
	default:
		return "stdio"
	}
}

// NormalizeTransport maps an input to a known transport, defaulting to stdio.
func NormalizeTransport(value string) MCPTransport {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(MCPTransportHTTP):
		return MCPTransportHTTP
	case string(MCPTransportSSE):
		return MCPTransportSSE
	default:
		return MCPTransportStdio
	}
}

// DefaultMCPBuiltins returns the built-in MCP catalog, all unselected.
func DefaultMCPBuiltins() []MCPBuiltinDraft {
	return []MCPBuiltinDraft{
		{
			ID:          MCPBuiltinJira,
			Name:        "Jira",
			Enabled:     false,
			Description: "Preference only: record intent to use Jira context later. Not connected or verified.",
			Status:      MCPStatusNotConfigured,
		},
		{
			ID:          MCPBuiltinContext7,
			Name:        "Context7",
			Enabled:     false,
			Description: "Preference only: record intent to use Context7 docs later. Not connected or verified.",
			Status:      MCPStatusNotConfigured,
		},
		{
			ID:          MCPBuiltinChromeDevTools,
			Name:        "Chrome DevTools",
			Enabled:     false,
			Description: "Preference only: record intent to use Chrome DevTools later. Not connected or verified.",
			Status:      MCPStatusNotConfigured,
		},
	}
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

// ConfiguredCount is selected built-ins plus custom entries.
func (d MCPDraft) ConfiguredCount() int {
	return d.SelectedBuiltinCount() + len(d.CustomServers)
}

// ToggleBuiltin flips Enabled for the built-in at index.
func (d *MCPDraft) ToggleBuiltin(index int) bool {
	if index < 0 || index >= len(d.Builtins) {
		return false
	}
	d.Builtins[index].Enabled = !d.Builtins[index].Enabled
	if d.Builtins[index].Enabled {
		d.Builtins[index].Status = MCPStatusInMemoryOnly
	} else {
		d.Builtins[index].Status = MCPStatusNotConfigured
	}
	return true
}

// ToggleCustom flips Enabled for the custom server at index.
func (d *MCPDraft) ToggleCustom(index int) bool {
	if index < 0 || index >= len(d.CustomServers) {
		return false
	}
	d.CustomServers[index].Enabled = !d.CustomServers[index].Enabled
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
func (d *MCPDraft) AddCustom(name string, transport MCPTransport, commandOrURL, arguments, envRefs string) (MCPServerDraft, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MCPServerDraft{}, fmt.Errorf("name is required")
	}
	if d.HasName(name) {
		return MCPServerDraft{}, fmt.Errorf("name already exists")
	}
	server := MCPServerDraft{
		ID:                    nextCustomMCPID(*d),
		Name:                  name,
		Transport:             NormalizeTransport(string(transport)),
		CommandOrURL:          strings.TrimSpace(commandOrURL),
		Arguments:             strings.TrimSpace(arguments),
		EnvironmentReferences: strings.TrimSpace(envRefs),
		Enabled:               false,
		Status:                MCPStatusInMemoryOnly,
	}
	d.CustomServers = append(d.CustomServers, server)
	return server, nil
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
	case MCPStatusInMemoryOnly:
		return "preference recorded"
	default:
		return "not configured"
	}
}
