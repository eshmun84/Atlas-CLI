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
	colorSelBg  = lipgloss.Color("33")
	colorActive = lipgloss.Color("39")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	bodyStyle = lipgloss.NewStyle().
			Foreground(colorText)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	failBadge = lipgloss.NewStyle().Bold(true).Foreground(colorFail)

	sidebarItemStyle = lipgloss.NewStyle().
				Foreground(colorText)

	sidebarSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(colorSelBg)

	sidebarActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorActive)

	sidebarIdleSelectedStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(colorActive)

	panelBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	errorBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFail)
)
