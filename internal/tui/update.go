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
		return m.loadCmd(m.route)
	}
	return nil
}

// Update handles resize, navigation, and async loads.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			return m, nil
		}
		m.loadErr = nil
		m.plan = msg.plan
		m.discovery = msg.discovery
		m.report = msg.report
		m.ready = true
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isForceQuit(msg) {
		m.quitting = true
		return m, tea.Quit
	}
	if isEsc(msg) {
		if m.route == RouteHome {
			m.quitting = true
			return m, tea.Quit
		}
		return m.goBack()
	}
	if isHelpKey(msg) {
		if m.route != RouteHelp {
			m.previous = m.route
			m.route = RouteHelp
			m.ready = true
			m.loadErr = nil
		}
		return m, nil
	}
	if isHomeKey(msg) && m.route != RouteHome {
		m.previous = RouteHome
		m.route = RouteHome
		m.ready = true
		m.loadErr = nil
		return m, nil
	}

	if m.route == RouteHome {
		return m.handleHomeKey(msg)
	}
	return m, nil
}

func (m Model) handleHomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.selected = clampSelected(m.selected - 1)
	case "down", "j":
		m.selected = clampSelected(m.selected + 1)
	case "enter":
		item := HomeItems[m.selected]
		if item.Exit {
			m.quitting = true
			return m, tea.Quit
		}
		return m.openRoute(item.Route)
	}
	return m, nil
}

func (m Model) openRoute(route Route) (Model, tea.Cmd) {
	m.previous = m.route
	m.route = route
	m.loadErr = nil
	if needsWorkspace(route) {
		m.ready = false
		return m, m.loadCmd(route)
	}
	m.ready = true
	return m, nil
}

func (m Model) goBack() (Model, tea.Cmd) {
	target := m.previous
	if target == m.route {
		target = RouteHome
	}
	m.previous = RouteHome
	m.route = target
	m.ready = !needsWorkspace(target)
	m.loadErr = nil
	if needsWorkspace(target) {
		return m, m.loadCmd(target)
	}
	return m, nil
}

func (m Model) loadCmd(route Route) tea.Cmd {
	getwd := m.getwd
	discover := m.discover
	return func() tea.Msg {
		root, err := getwd()
		if err != nil {
			return loadedMsg{route: route, err: fmt.Errorf("resolve working directory: %w", err)}
		}
		result, err := discover(root)
		if err != nil {
			return loadedMsg{route: route, err: fmt.Errorf("workspace discovery failed: %w", err)}
		}

		msg := loadedMsg{route: route, discovery: result}
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
