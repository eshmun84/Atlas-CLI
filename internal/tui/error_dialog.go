package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderErrorLayout() string {
	innerW := max(m.width-2, 1)
	innerH := max(m.height-2, 1)
	bodyH := m.contentViewportHeight()

	header := m.renderHeader()
	footer := m.renderFooter()
	divider := mutedStyle.Render(strings.Repeat("─", innerW))
	middle := lipgloss.Place(
		innerW,
		bodyH,
		lipgloss.Center,
		lipgloss.Center,
		m.errorDialogBox(innerW),
	)

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		divider,
		middle,
		divider,
		footer,
	)

	return panelBorder.Width(innerW).Height(innerH).Render(content)
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
