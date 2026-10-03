package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func tooSmall(width, height int) bool {
	return width > 0 && height > 0 && (width < MinWidth || height < MinHeight)
}

func renderFrame(width, height int, title, subtitle, body, footer string, errorFrame bool) string {
	if width <= 0 {
		width = MinWidth
	}
	if height <= 0 {
		height = MinHeight
	}

	if tooSmall(width, height) {
		return renderSmallTerminal(width, height)
	}

	innerW := max(width-2, 1)
	innerH := max(height-2, 1)

	header := titleStyle.Render(title)
	if subtitle != "" {
		header += "\n" + subtitleStyle.Width(innerW).Render(subtitle)
	}

	headerBlock := lipgloss.NewStyle().Width(innerW).Render(header)
	footerBlock := footerStyle.Width(innerW).Render(footer)
	divider := mutedStyle.Render(strings.Repeat("─", innerW))

	headerLines := countLines(headerBlock) + 2 // header + divider
	footerLines := countLines(footerBlock) + 1 // divider + footer
	bodyH := max(innerH-headerLines-footerLines, 1)

	bodyBlock := bodyStyle.
		Width(innerW).
		Height(bodyH).
		MaxHeight(bodyH).
		Render(body)

	content := lipgloss.JoinVertical(lipgloss.Left,
		headerBlock,
		divider,
		bodyBlock,
		divider,
		footerBlock,
	)

	style := frameStyle
	if errorFrame {
		style = errorFrameStyle
	}
	return style.Width(innerW).Height(innerH).Render(content)
}

func renderSmallTerminal(width, height int) string {
	msg := titleStyle.Render("Atlas") + "\n\n" +
		bodyStyle.Render("Atlas needs a larger terminal.") + "\n" +
		mutedStyle.Render("Minimum recommended size: 80x24.") + "\n\n" +
		footerStyle.Render("q / ctrl+c / esc quit")

	w := max(width-2, 1)
	h := max(height-2, 1)
	if width <= 2 || height <= 2 {
		return "Atlas needs a larger terminal.\nMinimum recommended size: 80x24."
	}
	return frameStyle.Width(w).Height(h).Render(lipgloss.NewStyle().Width(w).Height(h).Render(msg))
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
