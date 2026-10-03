package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

// Init loads workspace-backed screens when needed.
func (m Model) Init() tea.Cmd {
	if needsWorkspace(m.route) {
		return m.loadCmd()
	}
	return nil
}

// Update handles resize, sidebar navigation, scrolling, and loads.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.contentOffset = clampOffset(m.contentOffset, m.maxContentOffset())
		return m, nil

	case tea.KeyMsg:
		if tooSmall(m.width, m.height) {
			if isForceQuit(msg) || isEsc(msg) {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
		return m.handleKey(msg)

	case loadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			m.ready = true
			m.contentOffset = 0
			return m, nil
		}
		m.loadErr = nil
		m.plan = msg.plan
		m.discovery = msg.discovery
		m.report = msg.report
		m.ready = true
		m.contentOffset = 0
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.route == RouteError {
		return m.handleErrorKey(msg)
	}

	if isForceQuit(msg) {
		m.quitting = true
		return m, tea.Quit
	}
	if isEsc(msg) {
		if m.route == DefaultRoute {
			m.quitting = true
			return m, tea.Quit
		}
		return m.setRoute(DefaultRoute)
	}
	if isHelpKey(msg) {
		return m.setRoute(RouteHelp)
	}
	if isDefaultRouteKey(msg) {
		return m.setRoute(DefaultRoute)
	}

	switch msg.String() {
	case "up", "k":
		m.sidebarIndex = clampSidebar(m.sidebarIndex - 1)
		return m, nil
	case "down", "j":
		m.sidebarIndex = clampSidebar(m.sidebarIndex + 1)
		return m, nil
	case "enter":
		item := SidebarItems[m.sidebarIndex]
		if item.Exit {
			m.quitting = true
			return m, tea.Quit
		}
		return m.setRoute(item.Route)
	case "pgup", "pgdown", "home", "end":
		return m.scroll(msg.String()), nil
	}
	return m, nil
}

func (m Model) handleErrorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isForceQuit(msg), isEsc(msg), msg.String() == "enter":
		m.quitting = true
		return m, tea.Quit
	default:
		// Ignore navigation/help keys on the error dialog.
		return m, nil
	}
}

func (m Model) setRoute(route Route) (Model, tea.Cmd) {
	if m.route == route && m.ready {
		if route != RouteError {
			m.sidebarIndex = indexForRoute(route)
		}
		return m, nil
	}
	m.route = route
	if route != RouteError {
		m.sidebarIndex = indexForRoute(route)
	}
	m.loadErr = nil
	m.contentOffset = 0
	if needsWorkspace(route) {
		m.ready = false
		return m, m.loadCmd()
	}
	m.ready = true
	return m, nil
}

func (m Model) scroll(key string) Model {
	maxOff := m.maxContentOffset()
	switch key {
	case "pgup":
		m.contentOffset = clampOffset(m.contentOffset-m.contentViewportHeight(), maxOff)
	case "pgdown":
		m.contentOffset = clampOffset(m.contentOffset+m.contentViewportHeight(), maxOff)
	case "home":
		m.contentOffset = 0
	case "end":
		m.contentOffset = maxOff
	}
	return m
}

func (m Model) loadCmd() tea.Cmd {
	getwd := m.getwd
	discover := m.discover
	route := m.route
	return func() tea.Msg {
		root, err := getwd()
		if err != nil {
			return loadedMsg{err: fmt.Errorf("resolve working directory: %w", err)}
		}
		result, err := discover(root)
		if err != nil {
			return loadedMsg{err: fmt.Errorf("workspace discovery failed: %w", err)}
		}

		msg := loadedMsg{discovery: result}
		switch route {
		case RouteInitPlan:
			plan, err := initplan.Build(root, result)
			if err != nil {
				msg.err = fmt.Errorf("build init plan: %w", err)
				return msg
			}
			msg.plan = plan
		case RouteDoctor:
			msg.report = doctor.Evaluate(result)
		}
		return msg
	}
}
