package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/config"
)

const (
	MCPModeList = "list"

	MCPFocusBuiltins = "builtins"
)

var (
	mcpTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	mcpSection  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	mcpFocus    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	mcpMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	mcpBody     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mcpOK       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	mcpSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33"))
)

// MCPView options for the MCP integrations screen (Managed MCP V1 only).
type MCPView struct {
	Mode           string
	Draft          config.MCPDraft
	ActiveIndex    int
	Initialized    bool
	ContentFocused bool
	ListFocus      string
	Notice         string
	Embedded       bool
}

// RenderMCP renders the MCP configuration screen.
func RenderMCP(view MCPView) string {
	return renderMCPList(view)
}

// RenderMCPPanel renders Atlas Managed MCP V1 rows for embedding in Init Step 2.
func RenderMCPPanel(view MCPView) string {
	view.Embedded = true
	view.Mode = MCPModeList
	return renderMCPListBody(view)
}

func renderMCPList(view MCPView) string {
	var b strings.Builder
	title := mcpTitle.Render("MCP integrations")
	if view.ContentFocused {
		title += "  " + mcpFocus.Render("[content focus]")
	} else {
		title += "  " + mcpMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b, "  "+mcpMuted.Render("Atlas Managed MCP V1: Context7 and Chrome DevTools (opt-in)."))
	fmt.Fprintln(&b, "  "+mcpMuted.Render("Other MCPs are developer-managed. Secrets are not stored."))
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
	fmt.Fprintln(&b, mcpSection.Render("MCP integrations"))
	for i, item := range view.Draft.Builtins {
		mark := "[ ]"
		if item.Enabled {
			mark = "[x]"
		}
		desc := strings.TrimSpace(item.Description)
		line := fmt.Sprintf("%s %s", mark, item.Name)
		focused := view.ContentFocused && view.ListFocus == MCPFocusBuiltins && i == view.ActiveIndex
		writeMCPCheckRow(&b, line, focused, item.Enabled)
		if desc != "" {
			fmt.Fprintln(&b, "  "+mcpMuted.Render("    "+desc))
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
