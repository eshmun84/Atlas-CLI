package screens

import "github.com/charmbracelet/lipgloss"

var helpHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))

// Help renders Atlas TUI help for the content panel.
func Help() string {
	return helpHead.Render("Atlas Help") + `

TUI-first shell with sidebar navigation.
The only normal console output is: atlas --version

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
  ↑/↓ or k/j   move sidebar selection
  enter        open selected item
  PgUp/PgDn    scroll content
  Home/End     jump content
  h / ?        Help
  b / esc      Status (quit from Status)
  q / ctrl+c   quit`
}
