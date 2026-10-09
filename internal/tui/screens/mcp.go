package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/config"
)

const (
	MCPModeList = "list"
	MCPModeAdd  = "add"

	MCPFocusBuiltins  = "builtins"
	MCPFocusCustom    = "custom"
	MCPFocusAddBtn    = "add"
	MCPFocusClose     = "close"
	MCPFocusName      = "name"
	MCPFocusTransport = "transport"
	MCPFocusConn      = "connection"
	MCPFocusArgs      = "args"
	MCPFocusEnv       = "env"
	MCPFocusAuth      = "auth"
	MCPFocusHeader    = "header"
	MCPFocusHeaderEnv = "header_env"
	MCPFocusPrefix    = "prefix"
	MCPFocusCancel    = "cancel"
	MCPFocusSubmit    = "submit"
)

// MCPAuthModes returns selectable remote authentication modes for Add Custom.
func MCPAuthModes() []config.MCPAuthRequirement {
	return []config.MCPAuthRequirement{
		config.MCPAuthNone,
		config.MCPAuthEnvironmentReference,
		config.MCPAuthOAuthExternal,
		config.MCPAuthProviderManaged,
	}
}

// MCPAuthLabel returns a short UI label for an auth mode.
func MCPAuthLabel(auth config.MCPAuthRequirement) string {
	switch auth {
	case config.MCPAuthNone:
		return "None"
	case config.MCPAuthEnvironmentReference:
		return "Environment header"
	case config.MCPAuthOAuthExternal:
		return "External OAuth"
	case config.MCPAuthProviderManaged:
		return "Provider managed"
	default:
		return string(auth)
	}
}

var (
	mcpTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	mcpSection  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	mcpFocus    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	mcpMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	mcpBody     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mcpOK       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	mcpFail     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	mcpSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33"))
)

// MCPView options for the MCP integrations screen.
type MCPView struct {
	Mode              string
	Draft             config.MCPDraft
	ActiveIndex       int
	TransportFocus    int
	TransportSelected config.MCPTransport
	AuthFocus         int
	AuthSelected      config.MCPAuthRequirement
	Initialized       bool
	ContentFocused    bool
	ListFocus         string
	AddFocus          string
	NameView          string
	ConnectionView    string
	ArgsView          string
	EnvView           string
	HeaderView        string
	HeaderEnvView     string
	PrefixView        string
	Error             string
	Notice            string
	Embedded          bool
	ShowAddRow        bool
}

// RenderMCP renders the MCP configuration screen.
func RenderMCP(view MCPView) string {
	if view.Mode == MCPModeAdd {
		return renderMCPAdd(view)
	}
	return renderMCPList(view)
}

// RenderMCPPanel renders built-ins/custom rows for embedding in Init Step 2.
func RenderMCPPanel(view MCPView) string {
	view.Embedded = true
	view.ShowAddRow = true
	if view.Mode == MCPModeAdd {
		return renderMCPAddFields(view)
	}
	return renderMCPListBody(view)
}

func renderMCPList(view MCPView) string {
	var b strings.Builder
	title := mcpTitle.Render("MCP")
	if view.ContentFocused {
		title += "  " + mcpFocus.Render("[content focus]")
	} else {
		title += "  " + mcpMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b, "  "+mcpMuted.Render("External capabilities projected to selected agents on Apply."))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("Atlas-owned entries only; developer MCP configs are preserved."))
	fmt.Fprintln(&b)

	if view.Initialized {
		fmt.Fprintln(&b, "  "+mcpBody.Render("Apply saves .atlas/config.yaml and reconciles MCP projections."))
	} else {
		fmt.Fprintln(&b, "  "+mcpBody.Render("Atlas is not initialized yet."))
		fmt.Fprintln(&b, "  "+mcpBody.Render("MCP selections are draft-only until Init Apply."))
	}
	fmt.Fprintln(&b)
	fmt.Fprint(&b, renderMCPListBody(view))
	return strings.TrimRight(b.String(), "\n")
}

func renderMCPListBody(view MCPView) string {
	var b strings.Builder
	fmt.Fprintln(&b, mcpSection.Render("Built-in MCPs"))
	for i, item := range view.Draft.Builtins {
		mark := "[ ]"
		if item.Enabled {
			mark = "[x]"
		}
		line := fmt.Sprintf("%s %-16s %s", mark, item.Name, config.StatusLabel(item.Status))
		focused := view.ContentFocused && view.ListFocus == MCPFocusBuiltins && i == view.ActiveIndex
		writeMCPCheckRow(&b, line, focused, item.Enabled)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, mcpSection.Render("Custom MCPs"))
	if len(view.Draft.CustomServers) == 0 {
		fmt.Fprintln(&b, "  "+mcpMuted.Render("No custom MCPs configured yet."))
	} else {
		for i, server := range view.Draft.CustomServers {
			mark := "[ ]"
			if server.Enabled {
				mark = "[x]"
			}
			line := fmt.Sprintf("%s %-16s %-16s %s",
				mark, server.Name, server.Transport.TransportLabel(), config.StatusLabel(server.Status))
			focused := view.ContentFocused && view.ListFocus == MCPFocusCustom && i == view.ActiveIndex
			writeMCPCheckRow(&b, line, focused, server.Enabled)
		}
		fmt.Fprintln(&b, "  "+mcpMuted.Render("Space/Enter toggles enabled. d removes a custom entry (in memory only)."))
	}
	if view.ShowAddRow {
		fmt.Fprintln(&b)
		addLine := "[ Add MCP ]"
		if view.ContentFocused && view.ListFocus == MCPFocusAddBtn {
			fmt.Fprintln(&b, "  "+mcpSelected.Render("› "+addLine+" "))
		} else {
			fmt.Fprintln(&b, "  "+mcpBody.Render("  "+addLine))
		}
	}
	if view.Embedded && view.Initialized {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "  "+mcpMuted.Render("Close discards unsaved changes. Apply changes saves config and MCP projections."))
	}
	if view.Notice != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "  "+mcpMuted.Render(view.Notice))
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeMCPCheckRow(b *strings.Builder, line string, focused, enabled bool) {
	if focused {
		fmt.Fprintln(b, "  "+mcpSelected.Render("› "+line+" "))
		return
	}
	if enabled {
		fmt.Fprintln(b, "  "+mcpOK.Render("  "+line))
		return
	}
	fmt.Fprintln(b, "  "+mcpBody.Render("  "+line))
}

func renderMCPAdd(view MCPView) string {
	var b strings.Builder
	title := mcpTitle.Render("Add MCP")
	if view.ContentFocused {
		title += "  " + mcpFocus.Render("[content focus]")
	} else {
		title += "  " + mcpMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b, "  "+mcpMuted.Render("Custom external MCP. Secrets are never stored — use env/header references only."))
	fmt.Fprintln(&b)
	fmt.Fprint(&b, renderMCPAddFields(view))
	return strings.TrimRight(b.String(), "\n")
}

func renderMCPAddFields(view MCPView) string {
	var b strings.Builder
	http := view.TransportSelected == config.MCPTransportStreamableHTTP || view.TransportSelected == config.MCPTransportHTTP

	fmt.Fprintln(&b, mcpSection.Render("Name"))
	writeMCPTextField(&b, view.NameView, "(required)", view.ContentFocused && view.AddFocus == MCPFocusName)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, mcpSection.Render("Transport"))
	for i, transport := range config.MCPTransports() {
		checked := transport == view.TransportSelected
		mark := "[ ]"
		if checked {
			mark = "[x]"
		}
		line := mark + " " + transport.TransportLabel()
		focused := view.ContentFocused && view.AddFocus == MCPFocusTransport && i == view.TransportFocus
		writeMCPCheckRow(&b, line, focused, checked)
	}
	fmt.Fprintln(&b)

	connLabel := "Command"
	connHint := "(required for stdio)"
	if http {
		connLabel = "URL"
		connHint = "(required https://...)"
	}
	fmt.Fprintln(&b, mcpSection.Render(connLabel))
	writeMCPTextField(&b, view.ConnectionView, connHint, view.ContentFocused && view.AddFocus == MCPFocusConn)
	fmt.Fprintln(&b)

	if !http {
		fmt.Fprintln(&b, mcpSection.Render("Arguments"))
		writeMCPTextField(&b, view.ArgsView, "(optional, space-separated)", view.ContentFocused && view.AddFocus == MCPFocusArgs)
		fmt.Fprintln(&b)

		fmt.Fprintln(&b, mcpSection.Render("Environment references"))
		writeMCPTextField(&b, view.EnvView, "(optional names, e.g. API_TOKEN)", view.ContentFocused && view.AddFocus == MCPFocusEnv)
		fmt.Fprintln(&b)
	} else {
		fmt.Fprintln(&b, mcpSection.Render("Authentication"))
		for i, auth := range MCPAuthModes() {
			checked := auth == view.AuthSelected
			mark := "[ ]"
			if checked {
				mark = "[x]"
			}
			line := mark + " " + MCPAuthLabel(auth)
			focused := view.ContentFocused && view.AddFocus == MCPFocusAuth && i == view.AuthFocus
			writeMCPCheckRow(&b, line, focused, checked)
		}
		fmt.Fprintln(&b)

		if view.AuthSelected == config.MCPAuthEnvironmentReference {
			fmt.Fprintln(&b, mcpSection.Render("Header name"))
			writeMCPTextField(&b, view.HeaderView, "(e.g. Authorization)", view.ContentFocused && view.AddFocus == MCPFocusHeader)
			fmt.Fprintln(&b)

			fmt.Fprintln(&b, mcpSection.Render("Environment variable"))
			writeMCPTextField(&b, view.HeaderEnvView, "(e.g. GITHUB_TOKEN)", view.ContentFocused && view.AddFocus == MCPFocusHeaderEnv)
			fmt.Fprintln(&b)

			fmt.Fprintln(&b, mcpSection.Render("Optional prefix"))
			writeMCPTextField(&b, view.PrefixView, `(e.g. Bearer )`, view.ContentFocused && view.AddFocus == MCPFocusPrefix)
			fmt.Fprintln(&b)
		}
	}

	fmt.Fprintln(&b, mcpSection.Render("Notes"))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("No credentials are stored. Auth uses env refs, agent OAuth, or provider-managed flows."))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("Apply projects Atlas-owned entries into selected agent MCP configs."))
	if view.Error != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "  "+mcpFail.Render(view.Error))
	}

	return strings.TrimRight(b.String(), "\n")
}

func writeMCPTextField(b *strings.Builder, value, emptyHint string, focused bool) {
	line := value
	if strings.TrimSpace(line) == "" {
		line = mcpMuted.Render(emptyHint)
	}
	if focused {
		fmt.Fprintln(b, "  "+mcpSelected.Render("› "+line+" "))
		return
	}
	fmt.Fprintln(b, "  "+mcpBody.Render("  "+line))
}
