package tui

import tea "github.com/charmbracelet/bubbletea"

func isForceQuit(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "q", "ctrl+c":
		return true
	default:
		return false
	}
}

func isEsc(msg tea.KeyMsg) bool {
	return msg.String() == "esc"
}

func isHelpKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "h", "?":
		return true
	default:
		return false
	}
}

func isHomeKey(msg tea.KeyMsg) bool {
	return msg.String() == "b"
}
