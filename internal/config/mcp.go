package config

import (
	"fmt"
	"strings"
)

// MCPConnectionState describes whether an MCP integration is configured.
type MCPConnectionState string

const (
	MCPNotConfigured MCPConnectionState = "not_configured"
	MCPConfigured    MCPConnectionState = "configured"
)

// MCPServerKind identifies a built-in or custom MCP integration kind.
type MCPServerKind string

const (
	MCPKindJira     MCPServerKind = "jira"
	MCPKindContext7 MCPServerKind = "context7"
	MCPKindCustom   MCPServerKind = "custom"
)

// MCPKindTemplate is an add-form option for creating an MCP entry.
type MCPKindTemplate struct {
	Kind        MCPServerKind
	Label       string
	Description string
}

// MCPServerDraft is one in-memory MCP integration entry.
type MCPServerDraft struct {
	ID                    string
	Name                  string
	Kind                  MCPServerKind
	Enabled               bool
	Connection            string
	Description           string
	ConfigurationStatus   MCPConnectionState
	RequiresConfiguration bool
	Help                  string
}

// MCPDraft is the in-memory MCP configuration draft.
type MCPDraft struct {
	Servers []MCPServerDraft
}

// EmptyMCPDraft returns an MCP draft with no configured integrations.
func EmptyMCPDraft() MCPDraft {
	return MCPDraft{Servers: nil}
}

// DefaultMCPDraft returns an empty draft. Built-in kinds are templates only.
func DefaultMCPDraft() MCPDraft {
	return EmptyMCPDraft()
}

// MCPKindTemplates returns kinds available when adding an MCP entry.
func MCPKindTemplates() []MCPKindTemplate {
	return []MCPKindTemplate{
		{
			Kind:        MCPKindJira,
			Label:       "Jira",
			Description: "Connect Atlas with Jira issues, project planning and delivery tracking.",
		},
		{
			Kind:        MCPKindContext7,
			Label:       "Context7",
			Description: "Provide Atlas with up-to-date library and framework documentation context.",
		},
		{
			Kind:        MCPKindCustom,
			Label:       "Custom",
			Description: "Add a custom MCP server definition.",
		},
	}
}

// KindLabel returns a short display label for a kind.
func (k MCPServerKind) KindLabel() string {
	for _, tmpl := range MCPKindTemplates() {
		if tmpl.Kind == k {
			return tmpl.Label
		}
	}
	if k == "" {
		return MCPKindCustom.KindLabel()
	}
	return string(k)
}

// StatusLabel returns a short UI label for a connection state.
func (s MCPConnectionState) StatusLabel() string {
	switch s {
	case MCPConfigured:
		return "configured"
	default:
		return "not configured"
	}
}

// ToggleEnabled flips Enabled for the server at index when present.
func (d *MCPDraft) ToggleEnabled(index int) bool {
	if index < 0 || index >= len(d.Servers) {
		return false
	}
	d.Servers[index].Enabled = !d.Servers[index].Enabled
	return true
}

// HasName reports whether an entry with the same name already exists.
func (d MCPDraft) HasName(name string) bool {
	want := strings.TrimSpace(strings.ToLower(name))
	if want == "" {
		return false
	}
	for _, server := range d.Servers {
		if strings.ToLower(strings.TrimSpace(server.Name)) == want {
			return true
		}
	}
	return false
}

// AddServer validates and appends an in-memory MCP entry.
func (d *MCPDraft) AddServer(name string, kind MCPServerKind, connection string) (MCPServerDraft, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return MCPServerDraft{}, fmt.Errorf("name is required")
	}
	if d.HasName(name) {
		return MCPServerDraft{}, fmt.Errorf("name already exists")
	}
	if kind == "" {
		kind = MCPKindCustom
	}
	valid := false
	var description string
	for _, tmpl := range MCPKindTemplates() {
		if tmpl.Kind == kind {
			valid = true
			description = tmpl.Description
			break
		}
	}
	if !valid {
		kind = MCPKindCustom
		description = "Add a custom MCP server definition."
	}

	server := MCPServerDraft{
		ID:                    nextMCPID(*d, kind),
		Name:                  name,
		Kind:                  kind,
		Enabled:               false,
		Connection:            strings.TrimSpace(connection),
		Description:           description,
		ConfigurationStatus:   MCPNotConfigured,
		RequiresConfiguration: true,
		Help:                  "No credentials are stored in this slice. This entry is kept in memory only.",
	}
	d.Servers = append(d.Servers, server)
	return server, nil
}

// ServerByID finds a server draft by id.
func (d MCPDraft) ServerByID(id string) (MCPServerDraft, bool) {
	for _, server := range d.Servers {
		if server.ID == id {
			return server, true
		}
	}
	return MCPServerDraft{}, false
}

func nextMCPID(draft MCPDraft, kind MCPServerKind) string {
	base := string(kind)
	if base == "" {
		base = "mcp"
	}
	n := 1
	for {
		id := fmt.Sprintf("%s-%d", base, n)
		if _, ok := draft.ServerByID(id); !ok {
			return id
		}
		n++
	}
}
