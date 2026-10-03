package tui

import "github.com/charmbracelet/lipgloss"

const (
	MinWidth  = 80
	MinHeight = 24
)

var (
	colorAccent = lipgloss.Color("51")
	colorMuted  = lipgloss.Color("245")
	colorText   = lipgloss.Color("252")
	colorFail   = lipgloss.Color("196")
	colorBorder = lipgloss.Color("63")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	bodyStyle = lipgloss.NewStyle().
			Foreground(colorText)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	failBadge = lipgloss.NewStyle().Bold(true).Foreground(colorFail)

	frameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	errorFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFail)
)
