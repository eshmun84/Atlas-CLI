package screens

import "github.com/charmbracelet/lipgloss"

var helpHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))

// Help renders Atlas TUI help for the content panel.
func Help() string {
	return helpHead.Render("Atlas Help") + `

TUI-first shell with sidebar navigation.
Status is the default landing screen (executive overview).
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
  atlas mcp

These are not Atlas CLI commands.

` + helpHead.Render("Init wizard") + `
  Preflight  Runtime conflict block (manual cleanup required; Refresh or Exit only)
  Step 1  Project Setup (Name, New/Existing, optional docs scaffold)
  Step 2  Initial Configuration
            Governance · Adapters · Delivery · MCP
  Step 3  Review / Materialization Plan (Apply writes .atlas/ + compact AGENTS/adapter projections)

` + helpHead.Render("Runtime Repair") + `
  Available after initialization. Review runtime drift, then Apply repair.
  Status, Doctor, discovery, and Configure Apply never repair automatically.
  Atlas-owned drift is backed up under Atlas Home before replace/quarantine. Developer surfaces (CLAUDE.md, GEMINI.md, .agents/, .claude/) are never moved by Repair.
  Backup is mandatory. No skip, merge, or silent delete.

` + helpHead.Render("MCP") + `
  Built-ins: Jira · Context7 · Chrome DevTools (multi-select).
  Configure MCP during Init Step 2, or later in Configure after initialization.
  Add MCP creates custom entries. d removes a custom entry in memory.
  Init Apply config and Configure Apply changes persist MCP in .atlas/config.yaml.
  In Configure, [ Close ] discards; [ Apply changes ] saves (visible in every section, including MCP).
  No connections, credentials, or validation yet.

` + helpHead.Render("Keyboard") + `
  Tab          toggle sidebar/content focus (Init / Configure)
  ↑/↓ or k/j   move focus through sections, checkbox rows, MCP rows, footer
  ←/→          return to section list / enter section (never changes values)
  type         edit project name or MCP add fields when focused
  enter/space  select/toggle focused checkbox row, or activate footer action
  r            restore detected name + recommended mode (Step 1)
  PgUp/PgDn    scroll content
  Home/End     jump content / name edges
  h / ?        Help
  b            Status (default landing)
  esc          Status (quit from Status); leave screens/forms
  q / ctrl+c   quit the TUI (q types while editing text fields)`
}
