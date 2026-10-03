package screens

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	homeDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	homeItem = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	homeSel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33"))
)

var homeLabels = []string{
	"Init / Setup",
	"Status",
	"Doctor",
	"Help",
	"Exit",
}

// Home renders the selectable Home menu.
func Home(selected int) string {
	var b strings.Builder
	b.WriteString(homeDesc.Render("Atlas is not a coding agent."))
	b.WriteString("\n")
	b.WriteString(homeDesc.Render("Initialize governance, inspect status, and run diagnostics."))
	b.WriteString("\n\n")

	for i, label := range homeLabels {
		if i == selected {
			b.WriteString(homeSel.Render(" › " + label + " "))
		} else {
			b.WriteString(homeItem.Render("   " + label))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
