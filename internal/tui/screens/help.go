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
  Tab          toggle sidebar/content focus (Init / Setup)
  ↑/↓ or k/j   sidebar menu, or Init fields when content focused
  ←/→          move cursor in project name, or select control
  type         edit project name when name field is focused
  enter        open sidebar item, confirm selection, or Next
  r            restore detected name + recommended mode
  PgUp/PgDn    scroll content
  Home/End     jump content / name edges
  h / ?        Help
  b / esc      Dashboard (quit from Dashboard)
  q / ctrl+c   quit (q types while editing project name)`
}
