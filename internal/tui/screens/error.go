package screens

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	errorCmd  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("196")).Padding(0, 1)
	errorBody = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	errorHint = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// Error renders an unsupported-command screen.
func Error(command string) string {
	if command == "" {
		command = "(empty)"
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s\n%s\n\n%s",
		errorCmd.Render(" "+command+" "),
		errorBody.Render("This command is not supported by Atlas CLI."),
		errorBody.Render("Daily workflow commands such as \"atlas start\" and \"atlas change\""),
		errorBody.Render("are intentionally excluded and are not real commands."),
		errorHint.Render("Press h for Help, b for Home, or esc to go back."),
	)
}
