package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func (m Model) handleMCPContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mcpMode == screens.MCPModeAdd {
		return m.handleMCPAddKey(msg)
	}
	return m.handleMCPListKey(msg)
}

func (m Model) handleMCPListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	customCount := len(m.mcpDraft.CustomServers)
	builtinCount := len(m.mcpDraft.Builtins)
	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return m.scroll(msg.String()), nil
	case "up", "k":
		switch m.mcpListFocus {
		case screens.MCPFocusClose:
			m.mcpListFocus = screens.MCPFocusAddBtn
			m.mcpFooterIdx = 0
		case screens.MCPFocusAddBtn:
			if customCount > 0 {
				m.mcpListFocus = screens.MCPFocusCustom
				m.mcpIndex = customCount - 1
			} else if builtinCount > 0 {
				m.mcpListFocus = screens.MCPFocusBuiltins
				m.mcpIndex = builtinCount - 1
			}
		case screens.MCPFocusCustom:
			if m.mcpIndex > 0 {
				m.mcpIndex--
			} else if builtinCount > 0 {
				m.mcpListFocus = screens.MCPFocusBuiltins
				m.mcpIndex = builtinCount - 1
			}
		case screens.MCPFocusBuiltins:
			if m.mcpIndex > 0 {
				m.mcpIndex--
			}
		}
		return m, nil
	case "down", "j":
		switch m.mcpListFocus {
		case screens.MCPFocusBuiltins:
			if m.mcpIndex < builtinCount-1 {
				m.mcpIndex++
			} else if customCount > 0 {
				m.mcpListFocus = screens.MCPFocusCustom
				m.mcpIndex = 0
			} else {
				m.mcpListFocus = screens.MCPFocusAddBtn
				m.mcpFooterIdx = 0
			}
		case screens.MCPFocusCustom:
			if m.mcpIndex < customCount-1 {
				m.mcpIndex++
			} else {
				m.mcpListFocus = screens.MCPFocusAddBtn
				m.mcpFooterIdx = 0
			}
		case screens.MCPFocusAddBtn:
			m.mcpListFocus = screens.MCPFocusClose
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "left", "h":
		if m.mcpListFocus == screens.MCPFocusClose {
			m.mcpListFocus = screens.MCPFocusAddBtn
			m.mcpFooterIdx = 0
		}
		return m, nil
	case "right", "l":
		if m.mcpListFocus == screens.MCPFocusAddBtn {
			m.mcpListFocus = screens.MCPFocusClose
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "enter", " ", "space":
		switch m.mcpListFocus {
		case screens.MCPFocusClose:
			return m.setRoute(DefaultRoute)
		case screens.MCPFocusAddBtn:
			m.resetMCPAddForm()
			m.focus = FocusContent
			return m, nil
		case screens.MCPFocusBuiltins:
			m.mcpDraft.ToggleBuiltin(m.mcpIndex)
			return m, nil
		case screens.MCPFocusCustom:
			m.mcpDraft.ToggleCustom(m.mcpIndex)
			return m, nil
		}
	case "d":
		return m.removeFocusedCustom()
	}
	return m, nil
}

func (m Model) handleInitMCPSectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	customCount := len(m.mcpDraft.CustomServers)
	builtinCount := len(m.mcpDraft.Builtins)
	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return m.scroll(msg.String()), nil
	case "left", "h":
		return m.leaveConfigFields(), nil
	case "up", "k":
		switch m.mcpListFocus {
		case screens.MCPFocusAddBtn:
			if customCount > 0 {
				m.mcpListFocus = screens.MCPFocusCustom
				m.mcpIndex = customCount - 1
			} else if builtinCount > 0 {
				m.mcpListFocus = screens.MCPFocusBuiltins
				m.mcpIndex = builtinCount - 1
			}
		case screens.MCPFocusCustom:
			if m.mcpIndex > 0 {
				m.mcpIndex--
			} else if builtinCount > 0 {
				m.mcpListFocus = screens.MCPFocusBuiltins
				m.mcpIndex = builtinCount - 1
			}
		case screens.MCPFocusBuiltins:
			if m.mcpIndex > 0 {
				m.mcpIndex--
			} else {
				return m.leaveConfigFields(), nil
			}
		}
		return m, nil
	case "down", "j":
		switch m.mcpListFocus {
		case screens.MCPFocusBuiltins:
			if m.mcpIndex < builtinCount-1 {
				m.mcpIndex++
			} else if customCount > 0 {
				m.mcpListFocus = screens.MCPFocusCustom
				m.mcpIndex = 0
			} else {
				m.mcpListFocus = screens.MCPFocusAddBtn
			}
		case screens.MCPFocusCustom:
			if m.mcpIndex < customCount-1 {
				m.mcpIndex++
			} else {
				m.mcpListFocus = screens.MCPFocusAddBtn
			}
		case screens.MCPFocusAddBtn:
			m.configPanel = screens.ConfigPanelFooter
			// Prefer Apply changes when Configure exposes it.
			if m.route == RouteConfigure && m.configShowNext {
				m.configFooterIdx = 1
			} else {
				m.configFooterIdx = 0
			}
		}
		return m, nil
	case "enter", " ", "space":
		switch m.mcpListFocus {
		case screens.MCPFocusAddBtn:
			m.resetMCPAddForm()
			m.focus = FocusContent
			return m, nil
		case screens.MCPFocusBuiltins:
			m.mcpDraft.ToggleBuiltin(m.mcpIndex)
			if m.route == RouteConfigure {
				m.configureNotice = ""
			}
			return m, nil
		case screens.MCPFocusCustom:
			m.mcpDraft.ToggleCustom(m.mcpIndex)
			if m.route == RouteConfigure {
				m.configureNotice = ""
			}
			return m, nil
		}
	case "d":
		return m.removeFocusedCustom()
	}
	return m, nil
}

func (m Model) removeFocusedCustom() (tea.Model, tea.Cmd) {
	if m.mcpListFocus != screens.MCPFocusCustom {
		return m, nil
	}
	if m.mcpIndex < 0 || m.mcpIndex >= len(m.mcpDraft.CustomServers) {
		return m, nil
	}
	name := m.mcpDraft.CustomServers[m.mcpIndex].Name
	if !m.mcpDraft.RemoveCustom(m.mcpIndex) {
		return m, nil
	}
	m.mcpNotice = "Removed " + name + ". Apply changes to save."
	if m.route == RouteConfigure {
		m.configureNotice = ""
	}
	if len(m.mcpDraft.CustomServers) == 0 {
		m.mcpListFocus = screens.MCPFocusBuiltins
		m.mcpIndex = 0
		return m, nil
	}
	if m.mcpIndex >= len(m.mcpDraft.CustomServers) {
		m.mcpIndex = len(m.mcpDraft.CustomServers) - 1
	}
	return m, nil
}

func (m Model) handleMCPAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	transports := config.MCPTransports()
	editingText := m.mcpEditingAddText()

	switch msg.String() {
	case "pgup", "pgdown":
		return m.scroll(msg.String()), nil
	case "up", "k":
		if m.mcpAddFocus == screens.MCPFocusName {
			return m, nil
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusSubmit:
			m.mcpAddFocus = screens.MCPFocusCancel
			m.mcpFooterIdx = 0
		case screens.MCPFocusCancel:
			m.mcpAddFocus = screens.MCPFocusEnv
			m.syncMCPInputFocus()
		case screens.MCPFocusEnv:
			m.mcpAddFocus = screens.MCPFocusArgs
			m.syncMCPInputFocus()
		case screens.MCPFocusArgs:
			m.mcpAddFocus = screens.MCPFocusConn
			m.syncMCPInputFocus()
		case screens.MCPFocusConn:
			m.mcpAddFocus = screens.MCPFocusTransport
			m.mcpTransportFocus = len(transports) - 1
			m.syncMCPInputFocus()
		case screens.MCPFocusTransport:
			if m.mcpTransportFocus > 0 {
				m.mcpTransportFocus--
			} else {
				m.mcpAddFocus = screens.MCPFocusName
				m.syncMCPInputFocus()
			}
		}
		return m, nil
	case "down", "j":
		if m.mcpAddFocus == screens.MCPFocusName {
			m.mcpAddFocus = screens.MCPFocusTransport
			m.mcpTransportFocus = 0
			m.syncMCPInputFocus()
			return m, nil
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusTransport:
			if m.mcpTransportFocus < len(transports)-1 {
				m.mcpTransportFocus++
			} else {
				m.mcpAddFocus = screens.MCPFocusConn
				m.syncMCPInputFocus()
			}
		case screens.MCPFocusConn:
			m.mcpAddFocus = screens.MCPFocusArgs
			m.syncMCPInputFocus()
		case screens.MCPFocusArgs:
			m.mcpAddFocus = screens.MCPFocusEnv
			m.syncMCPInputFocus()
		case screens.MCPFocusEnv:
			m.mcpAddFocus = screens.MCPFocusCancel
			m.mcpFooterIdx = 0
			m.syncMCPInputFocus()
		case screens.MCPFocusCancel:
			m.mcpAddFocus = screens.MCPFocusSubmit
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "left", "right", "h", "l":
		if editingText {
			break
		}
		if m.mcpAddFocus == screens.MCPFocusTransport {
			return m, nil
		}
		if msg.String() == "left" || msg.String() == "h" {
			if m.mcpAddFocus == screens.MCPFocusSubmit {
				m.mcpAddFocus = screens.MCPFocusCancel
				m.mcpFooterIdx = 0
			}
			return m, nil
		}
		if m.mcpAddFocus == screens.MCPFocusCancel {
			m.mcpAddFocus = screens.MCPFocusSubmit
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "enter":
		if m.mcpAddFocus == screens.MCPFocusName {
			m.mcpAddFocus = screens.MCPFocusTransport
			m.syncMCPInputFocus()
			return m, nil
		}
		if m.mcpAddFocus == screens.MCPFocusConn || m.mcpAddFocus == screens.MCPFocusArgs || m.mcpAddFocus == screens.MCPFocusEnv {
			switch m.mcpAddFocus {
			case screens.MCPFocusConn:
				m.mcpAddFocus = screens.MCPFocusArgs
			case screens.MCPFocusArgs:
				m.mcpAddFocus = screens.MCPFocusEnv
			case screens.MCPFocusEnv:
				m.mcpAddFocus = screens.MCPFocusCancel
				m.mcpFooterIdx = 0
			}
			m.syncMCPInputFocus()
			return m, nil
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusTransport:
			m.mcpTransport = transports[m.mcpTransportFocus]
			return m, nil
		case screens.MCPFocusCancel:
			m.leaveMCPAddForm()
			return m, nil
		case screens.MCPFocusSubmit:
			return m.submitMCPAdd()
		}
		return m, nil
	case " ", "space":
		if editingText {
			break
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusTransport:
			m.mcpTransport = transports[m.mcpTransportFocus]
			return m, nil
		case screens.MCPFocusCancel:
			m.leaveMCPAddForm()
			return m, nil
		case screens.MCPFocusSubmit:
			return m.submitMCPAdd()
		}
		return m, nil
	}

	if m.mcpAddFocus == screens.MCPFocusName {
		var cmd tea.Cmd
		m.mcpNameInput, cmd = m.mcpNameInput.Update(msg)
		m.mcpAddError = ""
		return m, cmd
	}
	if m.mcpAddFocus == screens.MCPFocusConn {
		var cmd tea.Cmd
		m.mcpConnInput, cmd = m.mcpConnInput.Update(msg)
		return m, cmd
	}
	if m.mcpAddFocus == screens.MCPFocusArgs {
		var cmd tea.Cmd
		m.mcpArgsInput, cmd = m.mcpArgsInput.Update(msg)
		return m, cmd
	}
	if m.mcpAddFocus == screens.MCPFocusEnv {
		var cmd tea.Cmd
		m.mcpEnvInput, cmd = m.mcpEnvInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) mcpEditingAddText() bool {
	switch m.mcpAddFocus {
	case screens.MCPFocusName, screens.MCPFocusConn, screens.MCPFocusArgs, screens.MCPFocusEnv:
		return true
	default:
		return false
	}
}

func (m Model) submitMCPAdd() (tea.Model, tea.Cmd) {
	_, err := m.mcpDraft.AddCustom(
		m.mcpNameInput.Value(),
		m.mcpTransport,
		m.mcpConnInput.Value(),
		m.mcpArgsInput.Value(),
		m.mcpEnvInput.Value(),
	)
	if err != nil {
		m.mcpAddError = err.Error()
		m.mcpAddFocus = screens.MCPFocusName
		m.syncMCPInputFocus()
		return m, nil
	}
	m.mcpIndex = len(m.mcpDraft.CustomServers) - 1
	if m.route == RouteConfigure {
		m.configureNotice = ""
	}
	m.leaveMCPAddForm()
	m.mcpListFocus = screens.MCPFocusCustom
	return m, nil
}
