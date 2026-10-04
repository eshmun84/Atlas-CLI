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
	MCPFocusCancel    = "cancel"
	MCPFocusSubmit    = "submit"
)

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
	Initialized       bool
	ContentFocused    bool
	ListFocus         string
	AddFocus          string
	NameView          string
	ConnectionView    string
	ArgsView          string
	EnvView           string
	Error             string
	Notice            string
	Embedded          bool
	ShowAddRow        bool
}

// RenderMCP renders the MCP configuration foundation screen.
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
	fmt.Fprintln(&b, "  "+mcpMuted.Render("Configure external MCP integrations for Atlas."))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("No files will be changed in this slice."))
	fmt.Fprintln(&b)

	if view.Initialized {
		fmt.Fprintln(&b, "  "+mcpBody.Render("MCP integrations are edited in memory only in this slice."))
	} else {
		fmt.Fprintln(&b, "  "+mcpBody.Render("Atlas is not initialized yet."))
		fmt.Fprintln(&b, "  "+mcpBody.Render("MCP configuration is available as a preview only."))
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
		line := mark + " " + item.Name
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
			line := fmt.Sprintf("%s %s        %s         %s",
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
	fmt.Fprintln(&b, "  "+mcpMuted.Render("No files will be changed in this slice."))
	fmt.Fprintln(&b)
	fmt.Fprint(&b, renderMCPAddFields(view))
	return strings.TrimRight(b.String(), "\n")
}

func renderMCPAddFields(view MCPView) string {
	var b strings.Builder
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

	fmt.Fprintln(&b, mcpSection.Render("Command or URL"))
	writeMCPTextField(&b, view.ConnectionView, "(optional, not validated)", view.ContentFocused && view.AddFocus == MCPFocusConn)
	fmt.Fprintln(&b, "  "+mcpMuted.Render("Command or URL is not validated in this slice."))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, mcpSection.Render("Arguments"))
	writeMCPTextField(&b, view.ArgsView, "(optional)", view.ContentFocused && view.AddFocus == MCPFocusArgs)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, mcpSection.Render("Environment references"))
	writeMCPTextField(&b, view.EnvView, "(optional)", view.ContentFocused && view.AddFocus == MCPFocusEnv)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, mcpSection.Render("Notes"))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("No credentials are stored in this slice."))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("This custom MCP is kept in memory only."))
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
