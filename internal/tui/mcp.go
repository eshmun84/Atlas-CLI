package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

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
	auths := screens.MCPAuthModes()
	editingText := m.mcpEditingAddText()
	http := m.mcpTransport == config.MCPTransportStreamableHTTP || m.mcpTransport == config.MCPTransportHTTP

	switch msg.String() {
	case "pgup", "pgdown":
		return m.scroll(msg.String()), nil
	case "up", "k":
		if m.mcpAddFocus == screens.MCPFocusName {
			return m, nil
		}
		if m.mcpAddFocus == screens.MCPFocusTransport {
			if m.mcpTransportFocus > 0 {
				m.mcpTransportFocus--
				return m, nil
			}
			m.mcpAddFocus = screens.MCPFocusName
			m.syncMCPInputFocus()
			return m, nil
		}
		if m.mcpAddFocus == screens.MCPFocusAuth {
			if m.mcpAuthFocus > 0 {
				m.mcpAuthFocus--
				return m, nil
			}
			m.mcpAddFocus = screens.MCPFocusConn
			m.syncMCPInputFocus()
			return m, nil
		}
		m.mcpAddFocus = m.mcpAddFocusPrev(http)
		if m.mcpAddFocus == screens.MCPFocusTransport {
			m.mcpTransportFocus = len(transports) - 1
		}
		if m.mcpAddFocus == screens.MCPFocusAuth {
			m.mcpAuthFocus = len(auths) - 1
		}
		if m.mcpAddFocus == screens.MCPFocusCancel {
			m.mcpFooterIdx = 0
		}
		m.syncMCPInputFocus()
		return m, nil
	case "down", "j":
		if m.mcpAddFocus == screens.MCPFocusName {
			m.mcpAddFocus = screens.MCPFocusTransport
			m.mcpTransportFocus = 0
			m.syncMCPInputFocus()
			return m, nil
		}
		if m.mcpAddFocus == screens.MCPFocusTransport {
			if m.mcpTransportFocus < len(transports)-1 {
				m.mcpTransportFocus++
				return m, nil
			}
		}
		if m.mcpAddFocus == screens.MCPFocusAuth {
			if m.mcpAuthFocus < len(auths)-1 {
				m.mcpAuthFocus++
				return m, nil
			}
		}
		m.mcpAddFocus = m.mcpAddFocusNext(http)
		if m.mcpAddFocus == screens.MCPFocusCancel {
			m.mcpFooterIdx = 0
		}
		if m.mcpAddFocus == screens.MCPFocusSubmit {
			m.mcpFooterIdx = 1
		}
		m.syncMCPInputFocus()
		return m, nil
	case "left", "right", "h", "l":
		if editingText {
			break
		}
		if m.mcpAddFocus == screens.MCPFocusTransport || m.mcpAddFocus == screens.MCPFocusAuth {
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
		if m.mcpEditingAddText() {
			m.mcpAddFocus = m.mcpAddFocusNext(http)
			if m.mcpAddFocus == screens.MCPFocusCancel {
				m.mcpFooterIdx = 0
			}
			m.syncMCPInputFocus()
			return m, nil
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusTransport:
			m.mcpTransport = transports[m.mcpTransportFocus]
			return m, nil
		case screens.MCPFocusAuth:
			m.mcpAuth = auths[m.mcpAuthFocus]
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
		case screens.MCPFocusAuth:
			m.mcpAuth = auths[m.mcpAuthFocus]
			return m, nil
		case screens.MCPFocusCancel:
			m.leaveMCPAddForm()
			return m, nil
		case screens.MCPFocusSubmit:
			return m.submitMCPAdd()
		}
		return m, nil
	}

	switch m.mcpAddFocus {
	case screens.MCPFocusName:
		var cmd tea.Cmd
		m.mcpNameInput, cmd = m.mcpNameInput.Update(msg)
		m.mcpAddError = ""
		return m, cmd
	case screens.MCPFocusConn:
		var cmd tea.Cmd
		m.mcpConnInput, cmd = m.mcpConnInput.Update(msg)
		return m, cmd
	case screens.MCPFocusArgs:
		var cmd tea.Cmd
		m.mcpArgsInput, cmd = m.mcpArgsInput.Update(msg)
		return m, cmd
	case screens.MCPFocusEnv:
		var cmd tea.Cmd
		m.mcpEnvInput, cmd = m.mcpEnvInput.Update(msg)
		return m, cmd
	case screens.MCPFocusHeader:
		var cmd tea.Cmd
		m.mcpHeaderInput, cmd = m.mcpHeaderInput.Update(msg)
		return m, cmd
	case screens.MCPFocusHeaderEnv:
		var cmd tea.Cmd
		m.mcpHeaderEnvInput, cmd = m.mcpHeaderEnvInput.Update(msg)
		return m, cmd
	case screens.MCPFocusPrefix:
		var cmd tea.Cmd
		m.mcpPrefixInput, cmd = m.mcpPrefixInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) mcpHTTPAdd() bool {
	return m.mcpTransport == config.MCPTransportStreamableHTTP || m.mcpTransport == config.MCPTransportHTTP
}

func (m Model) mcpAddFocusNext(http bool) string {
	switch m.mcpAddFocus {
	case screens.MCPFocusTransport:
		return screens.MCPFocusConn
	case screens.MCPFocusConn:
		if http {
			return screens.MCPFocusAuth
		}
		return screens.MCPFocusArgs
	case screens.MCPFocusArgs:
		return screens.MCPFocusEnv
	case screens.MCPFocusEnv:
		return screens.MCPFocusCancel
	case screens.MCPFocusAuth:
		if m.mcpAuth == config.MCPAuthEnvironmentReference {
			return screens.MCPFocusHeader
		}
		return screens.MCPFocusCancel
	case screens.MCPFocusHeader:
		return screens.MCPFocusHeaderEnv
	case screens.MCPFocusHeaderEnv:
		return screens.MCPFocusPrefix
	case screens.MCPFocusPrefix:
		return screens.MCPFocusCancel
	case screens.MCPFocusCancel:
		return screens.MCPFocusSubmit
	default:
		return screens.MCPFocusCancel
	}
}

func (m Model) mcpAddFocusPrev(http bool) string {
	switch m.mcpAddFocus {
	case screens.MCPFocusSubmit:
		return screens.MCPFocusCancel
	case screens.MCPFocusCancel:
		if http {
			if m.mcpAuth == config.MCPAuthEnvironmentReference {
				return screens.MCPFocusPrefix
			}
			return screens.MCPFocusAuth
		}
		return screens.MCPFocusEnv
	case screens.MCPFocusPrefix:
		return screens.MCPFocusHeaderEnv
	case screens.MCPFocusHeaderEnv:
		return screens.MCPFocusHeader
	case screens.MCPFocusHeader:
		return screens.MCPFocusAuth
	case screens.MCPFocusAuth:
		return screens.MCPFocusConn
	case screens.MCPFocusEnv:
		return screens.MCPFocusArgs
	case screens.MCPFocusArgs:
		return screens.MCPFocusConn
	case screens.MCPFocusConn:
		return screens.MCPFocusTransport
	case screens.MCPFocusTransport:
		return screens.MCPFocusName
	default:
		return screens.MCPFocusName
	}
}

func (m Model) mcpEditingAddText() bool {
	switch m.mcpAddFocus {
	case screens.MCPFocusName, screens.MCPFocusConn, screens.MCPFocusArgs, screens.MCPFocusEnv,
		screens.MCPFocusHeader, screens.MCPFocusHeaderEnv, screens.MCPFocusPrefix:
		return true
	default:
		return false
	}
}

func (m Model) submitMCPAdd() (tea.Model, tea.Cmd) {
	var err error
	if m.mcpHTTPAdd() {
		_, err = m.mcpDraft.AddCustomRemote(
			m.mcpNameInput.Value(),
			m.mcpConnInput.Value(),
			m.mcpAuth,
			m.mcpHeaderInput.Value(),
			m.mcpHeaderEnvInput.Value(),
			m.mcpPrefixInput.Value(),
		)
	} else {
		_, err = m.mcpDraft.AddCustom(
			m.mcpNameInput.Value(),
			m.mcpTransport,
			m.mcpConnInput.Value(),
			m.mcpArgsInput.Value(),
			m.mcpEnvInput.Value(),
		)
	}
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
