package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderErrorLayout() string {
	innerW := max(m.width-2, 1)

	header := m.renderHeader()
	footer := m.renderFooter()
	divider := mutedStyle.Render(strings.Repeat("─", innerW))
	dialog := m.errorDialogBox(innerW)

	body := lipgloss.JoinVertical(lipgloss.Center,
		header,
		divider,
		"",
		dialog,
		"",
		divider,
		footer,
	)

	return panelBorder.Width(innerW).Render(body)
}

func (m Model) errorDialogBox(availableWidth int) string {
	cmd := strings.TrimSpace(m.unknownCommand)
	if cmd == "" {
		cmd = "(empty)"
	}
	if !strings.HasPrefix(cmd, "atlas ") {
		cmd = "atlas " + cmd
	}

	dialogW := 44
	if availableWidth < dialogW+2 {
		dialogW = max(availableWidth-2, 20)
	}

	title := titleStyle.Render("Error")
	message := bodyStyle.Width(dialogW - 4).Render(fmt.Sprintf("Unsupported command: %s", cmd))
	action := sidebarSelectedStyle.Render(" [ Salir ] ")

	body := lipgloss.JoinVertical(lipgloss.Center,
		title,
		"",
		message,
		"",
		action,
	)

	return errorBorder.
		Width(dialogW).
		Padding(1, 2).
		Render(body)
}
