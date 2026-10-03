package tui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
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
		m.discovery = msg.discovery
		m.report = msg.report
		m.sidebarIndex = indexForRoute(m.Sidebar(), m.route)
		if m.route == RouteInitPlan {
			m.applyInitDiscovery(msg.plan)
			m = m.rebuildInitPlan()
		}
		m.ready = true
		m.contentOffset = 0
		return m, nil
	}
	return m, nil
}

func (m *Model) applyInitDiscovery(plan initplan.Plan) {
	m.detectedName = filepath.Base(m.discovery.RootPath)
	if m.detectedName == "" || m.detectedName == "." {
		m.detectedName = plan.ProjectName
	}
	m.detectedMode = plan.ProjectMode
	if m.detectedMode == "" {
		m.detectedMode = "existing"
	}
	m.recommendedMode = modeFromDetected(m.detectedMode)
	if !m.initHydrated {
		m.nameInput.SetValue(m.detectedName)
		m.nameInput.CursorEnd()
		m.initModeConfirmed = m.recommendedMode
		m.initDecision = InitDecisionInitialize
		m.initStepConfirmed = false
		m.initField = screens.InitFieldName
		m.initHydrated = true
	}
	if !m.hasRuntimeArtifacts() && m.initDecision == InitDecisionCancel {
		m.initDecision = InitDecisionInitialize
	}
	m.syncNameInputFocus()
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.route == RouteError {
		return m.handleErrorKey(msg)
	}

	editingName := m.focus == FocusContent && m.route == RouteInitPlan && m.initField == screens.InitFieldName

	// ctrl+c always quits; bare q quits unless typing in the name field.
	if msg.String() == "ctrl+c" || (!editingName && msg.String() == "q") {
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

	if m.route == RouteInitPlan && msg.String() == "tab" {
		if m.focus == FocusSidebar {
			m.focus = FocusContent
			m.initField = m.clampInitField(m.initField)
		} else {
			m.focus = FocusSidebar
		}
		m.syncNameInputFocus()
		return m, nil
	}

	if m.focus == FocusContent && m.route == RouteInitPlan {
		return m.handleInitContentKey(msg)
	}

	items := m.Sidebar()
	switch msg.String() {
	case "up", "k":
		m.sidebarIndex = clampSidebar(m.sidebarIndex-1, len(items))
		return m, nil
	case "down", "j":
		m.sidebarIndex = clampSidebar(m.sidebarIndex+1, len(items))
		return m, nil
	case "enter":
		item := items[m.sidebarIndex]
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

func (m Model) handleInitContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fields := m.initFields()
	fieldIndex := 0
	for i, f := range fields {
		if f == m.initField {
			fieldIndex = i
			break
		}
	}

	// Field navigation with up/down. k/j navigate only when not editing the name.
	switch msg.String() {
	case "up":
		if fieldIndex > 0 {
			m.initField = fields[fieldIndex-1]
			m.syncNameInputFocus()
		}
		return m, nil
	case "down":
		if fieldIndex < len(fields)-1 {
			m.initField = fields[fieldIndex+1]
			m.syncNameInputFocus()
		}
		return m, nil
	case "k", "j":
		if m.initField != screens.InitFieldName {
			if msg.String() == "k" && fieldIndex > 0 {
				m.initField = fields[fieldIndex-1]
			}
			if msg.String() == "j" && fieldIndex < len(fields)-1 {
				m.initField = fields[fieldIndex+1]
			}
			m.syncNameInputFocus()
			return m, nil
		}
	case "enter":
		return m.activateInitField(), nil
	case "r":
		if m.initField == screens.InitFieldName {
			// Let textinput receive 'r' while editing the name.
			break
		}
		m.nameInput.SetValue(m.detectedName)
		m.nameInput.CursorEnd()
		m.initModeConfirmed = m.recommendedMode
		m.initDecision = InitDecisionInitialize
		m.initStepConfirmed = false
		m.initField = screens.InitFieldName
		m.syncNameInputFocus()
		m = m.rebuildInitPlan()
		return m, nil
	case "pgup", "pgdown":
		return m.scroll(msg.String()), nil
	case "home", "end":
		if m.initField != screens.InitFieldName {
			return m.scroll(msg.String()), nil
		}
	}

	if m.initField == screens.InitFieldName {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		m = m.rebuildInitPlan()
		return m, cmd
	}

	// left/right select mode/decision when those fields are active.
	switch msg.String() {
	case "left", "right":
		return m.activateInitField(), nil
	}
	return m, nil
}

func (m Model) activateInitField() Model {
	switch m.initField {
	case screens.InitFieldModeNew:
		m.initModeConfirmed = InitModeNew
		m.initStepConfirmed = false
	case screens.InitFieldModeExisting:
		m.initModeConfirmed = InitModeExisting
		m.initStepConfirmed = false
	case screens.InitFieldDecisionInit:
		m.initDecision = InitDecisionInitialize
		m.initStepConfirmed = false
	case screens.InitFieldDecisionCancel:
		m.initDecision = InitDecisionCancel
		m.initStepConfirmed = false
	case screens.InitFieldNext:
		m.initStepConfirmed = true
	}
	m = m.rebuildInitPlan()
	return m
}

func (m Model) handleErrorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isForceQuit(msg), isEsc(msg), msg.String() == "enter":
		m.quitting = true
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m Model) setRoute(route Route) (Model, tea.Cmd) {
	if m.route == route && m.ready {
		if route != RouteError {
			m.sidebarIndex = indexForRoute(m.Sidebar(), route)
		}
		return m, nil
	}
	m.route = route
	if route != RouteError {
		m.sidebarIndex = indexForRoute(m.Sidebar(), route)
	}
	m.focus = FocusSidebar
	m.syncNameInputFocus()
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

func (m Model) rebuildInitPlan() Model {
	root := m.discovery.RootPath
	if root == "" {
		return m
	}
	plan, err := initplan.BuildWithOptions(root, m.discovery, initplan.Options{
		ModeOverride: string(m.initModeConfirmed),
		ProjectName:  m.nameInput.Value(),
	})
	if err != nil {
		m.loadErr = fmt.Errorf("build init plan: %w", err)
		return m
	}
	m.plan = plan
	m.loadErr = nil
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
