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

	MCPFocusServers = "servers"
	MCPFocusAddBtn  = "add"
	MCPFocusClose   = "close"
	MCPFocusName    = "name"
	MCPFocusKind    = "kind"
	MCPFocusConn    = "connection"
	MCPFocusCancel  = "cancel"
	MCPFocusSubmit  = "submit"
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
	Mode           string
	Draft          config.MCPDraft
	ActiveIndex    int
	KindFocus      int
	KindSelected   config.MCPServerKind
	Initialized    bool
	ContentFocused bool
	ListFocus      string
	AddFocus       string
	NameView       string
	ConnectionView string
	Error          string
}

// RenderMCP renders the MCP configuration foundation screen.
func RenderMCP(view MCPView) string {
	if view.Mode == MCPModeAdd {
		return renderMCPAdd(view)
	}
	return renderMCPList(view)
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

	if len(view.Draft.Servers) == 0 {
		fmt.Fprintln(&b, "  "+mcpBody.Render("No MCP integrations configured yet."))
	} else {
		fmt.Fprintln(&b, mcpSection.Render("Configured integrations"))
		for i, server := range view.Draft.Servers {
			mark := "[ ]"
			if server.Enabled {
				mark = "[x]"
			}
			line := fmt.Sprintf("%s %-16s  %-10s  %s", mark, server.Name, server.Kind.KindLabel(), server.ConfigurationStatus.StatusLabel())
			focused := view.ContentFocused && view.ListFocus == MCPFocusServers && i == view.ActiveIndex
			if focused {
				fmt.Fprintln(&b, "  "+mcpSelected.Render("› "+line+" "))
			} else if server.Enabled {
				fmt.Fprintln(&b, "  "+mcpOK.Render("  "+line))
			} else {
				fmt.Fprintln(&b, "  "+mcpBody.Render("  "+line))
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
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

	fmt.Fprintln(&b, mcpSection.Render("Name"))
	nameLine := view.NameView
	if strings.TrimSpace(nameLine) == "" {
		nameLine = mcpMuted.Render("(required)")
	}
	if view.ContentFocused && view.AddFocus == MCPFocusName {
		fmt.Fprintln(&b, "  "+mcpSelected.Render("› "+nameLine+" "))
	} else {
		fmt.Fprintln(&b, "  "+mcpBody.Render("  "+nameLine))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, mcpSection.Render("Kind"))
	for i, tmpl := range config.MCPKindTemplates() {
		checked := tmpl.Kind == view.KindSelected
		mark := "[ ]"
		if checked {
			mark = "[x]"
		}
		line := mark + " " + tmpl.Label
		focused := view.ContentFocused && view.AddFocus == MCPFocusKind && i == view.KindFocus
		if focused {
			fmt.Fprintln(&b, "  "+mcpSelected.Render("› "+line+" "))
		} else if checked {
			fmt.Fprintln(&b, "  "+mcpOK.Render("  "+line))
		} else {
			fmt.Fprintln(&b, "  "+mcpBody.Render("  "+line))
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, mcpSection.Render("Connection"))
	connLine := view.ConnectionView
	if strings.TrimSpace(connLine) == "" {
		connLine = mcpMuted.Render("(optional)")
	}
	if view.ContentFocused && view.AddFocus == MCPFocusConn {
		fmt.Fprintln(&b, "  "+mcpSelected.Render("› "+connLine+" "))
	} else {
		fmt.Fprintln(&b, "  "+mcpBody.Render("  "+connLine))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "  "+mcpMuted.Render("No credentials are stored in this slice."))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("This entry is kept in memory only."))
	if view.Error != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "  "+mcpFail.Render(view.Error))
	}

	return strings.TrimRight(b.String(), "\n")
}
