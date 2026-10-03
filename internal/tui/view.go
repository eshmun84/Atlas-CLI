package tui

import "github.com/eshmun84/Atlas-CLI/internal/tui/screens"

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	title, subtitle := m.header()
	body := m.body()
	footer := m.footer()
	return renderFrame(m.width, m.height, title, subtitle, body, footer, m.route == RouteError)
}

func (m Model) header() (title, subtitle string) {
	switch m.route {
	case RouteHome:
		return "Atlas", "Governed AI-assisted software engineering"
	case RouteHelp:
		return "Atlas Help", "TUI-first interface · --version is the only console output"
	case RouteInitPlan:
		return "Atlas Init Plan", "Dry-run only · no files will be created"
	case RouteStatus:
		return "Atlas Status", "Read-only workspace inspection"
	case RouteDoctor:
		return "Atlas Doctor", "Read-only diagnostics"
	case RouteError:
		return "Unknown command", "This is not a supported Atlas CLI command"
	default:
		return "Atlas", ""
	}
}

func (m Model) body() string {
	if m.loadErr != nil {
		return failBadge.Render("ERROR") + "  " + m.loadErr.Error()
	}
	if !m.ready {
		return mutedStyle.Render("Loading workspace…")
	}

	switch m.route {
	case RouteHome:
		return screens.Home(m.selected)
	case RouteHelp:
		return screens.Help()
	case RouteInitPlan:
		return screens.InitPlan(m.plan)
	case RouteStatus:
		return screens.Status(m.discovery)
	case RouteDoctor:
		return screens.Doctor(m.report)
	case RouteError:
		return screens.Error(m.unknownCommand)
	default:
		return screens.Error(m.unknownCommand)
	}
}

func (m Model) footer() string {
	if m.route == RouteHome {
		return "↑/↓ move  Enter select  h help  q quit"
	}
	return "h help  b home  esc back  q quit"
}
