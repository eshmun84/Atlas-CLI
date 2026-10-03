package screens

import "github.com/charmbracelet/lipgloss"

var helpHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))

// Help renders Atlas TUI help.
func Help() string {
	return helpHead.Render("TUI-first") + `

The only normal console output is: atlas --version
All other commands launch this full-screen TUI.

` + helpHead.Render("Supported routes") + `
  atlas
  atlas help
  atlas init
  atlas init --dry-run
  atlas status
  atlas doctor
  atlas --version

` + helpHead.Render("Intentionally unsupported") + `
  atlas start
  atlas change
  atlas change new

These are not Atlas CLI commands.

` + helpHead.Render("Keyboard") + `
  ↑/↓ or k/j   move Home selection
  enter        open selected item
  h / ?        Help
  b            Home
  esc          back (quit from Home)
  q / ctrl+c   quit`
}
