package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func (m Model) handleInitMCPSectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	builtinCount := len(m.mcpDraft.Builtins)
	m.mcpListFocus = screens.MCPFocusBuiltins
	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return m.scroll(msg.String()), nil
	case "left", "h":
		return m.leaveConfigFields(), nil
	case "up", "k":
		if m.mcpIndex > 0 {
			m.mcpIndex--
			return m, nil
		}
		return m.leaveConfigFields(), nil
	case "down", "j":
		if m.mcpIndex < builtinCount-1 {
			m.mcpIndex++
			return m, nil
		}
		m.configPanel = screens.ConfigPanelFooter
		if m.route == RouteConfigure && m.configShowNext {
			m.configFooterIdx = 1
		} else {
			m.configFooterIdx = 0
		}
		return m, nil
	case "enter", " ", "space":
		m.mcpDraft.ToggleBuiltin(m.mcpIndex)
		if m.route == RouteConfigure {
			m.configureNotice = ""
		}
		return m, nil
	}
	return m, nil
}
