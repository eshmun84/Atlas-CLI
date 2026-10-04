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
  atlas mcp
  atlas status
  atlas doctor
  atlas --version

` + helpHead.Render("Intentionally unsupported") + `
  atlas start
  atlas change
  atlas change new

These are not Atlas CLI commands.

` + helpHead.Render("Init wizard") + `
  Step 1  Project Setup
  Step 2  Initial Configuration
            Governance · Adapters · Source Control · Memory · MCP
  Step 3  Review / Materialization Plan (preview only; Apply is not implemented)

` + helpHead.Render("MCP") + `
  Built-ins: Jira · Context7 · Chrome DevTools (multi-select, in-memory).
  Available in Init Step 2 and the standalone MCP screen (same draft).
  Add MCP creates custom entries. d removes a custom entry in memory.
  No connections, credentials, or file writes yet.

` + helpHead.Render("Keyboard") + `
  Tab          toggle sidebar/content focus (Init / Configure / MCP)
  ↑/↓ or k/j   move focus through sections, checkbox rows, MCP rows, footer
  ←/→          return to section list / enter section (never changes values)
  type         edit project name or MCP add fields when focused
  enter/space  select/toggle focused checkbox row, or activate footer action
  r            restore detected name + recommended mode (Step 1)
  PgUp/PgDn    scroll content
  Home/End     jump content / name edges
  h / ?        Help
  b            Dashboard
  esc          Dashboard (quit from Dashboard); leave screens/forms
  q / ctrl+c   quit the TUI (q types while editing text fields)`
}
