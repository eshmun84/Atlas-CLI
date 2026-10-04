package tui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/config"
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
		if m.route == RouteConfigure {
			m.rebuildConfigDraft(config.ConfigModeConfigure, true, false)
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
		m.initWizardStep = screens.InitWizardStepProject
		m.initConfigConfirmed = false
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

	editingText := m.focus == FocusContent &&
		((m.route == RouteInitPlan &&
			m.initWizardStep == screens.InitWizardStepProject &&
			m.initField == screens.InitFieldName) ||
			(m.route == RouteMCP &&
				m.mcpMode == screens.MCPModeAdd &&
				(m.mcpAddFocus == screens.MCPFocusName || m.mcpAddFocus == screens.MCPFocusConn)))

	if msg.String() == "ctrl+c" || (!editingText && msg.String() == "q") {
		m.quitting = true
		return m, tea.Quit
	}
	if isEsc(msg) && !editingText {
		if m.route == DefaultRoute {
			m.quitting = true
			return m, tea.Quit
		}
		return m.setRoute(DefaultRoute)
	}
	if !editingText && isHelpKey(msg) {
		return m.setRoute(RouteHelp)
	}
	if !editingText && isDefaultRouteKey(msg) {
		return m.setRoute(DefaultRoute)
	}

	if (m.route == RouteInitPlan || m.route == RouteConfigure || m.route == RouteMCP) && msg.String() == "tab" {
		if m.focus == FocusSidebar {
			m.focus = FocusContent
			if m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepProject {
				m.initField = m.clampInitField(m.initField)
			}
		} else {
			m.focus = FocusSidebar
		}
		m.syncNameInputFocus()
		return m, nil
	}

	if m.focus == FocusContent && m.route == RouteInitPlan {
		return m.handleInitContentKey(msg)
	}
	if m.focus == FocusContent && m.route == RouteConfigure {
		return m.handleConfigFormKey(msg)
	}
	if m.focus == FocusContent && m.route == RouteMCP {
		return m.handleMCPContentKey(msg)
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
	if m.initWizardStep == screens.InitWizardStepConfig {
		return m.handleConfigFormKey(msg)
	}
	return m.handleInitStep1Key(msg)
}

func (m Model) handleInitStep1Key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fields := m.initFields()
	fieldIndex := 0
	for i, f := range fields {
		if f == m.initField {
			fieldIndex = i
			break
		}
	}

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
			break
		}
		m.nameInput.SetValue(m.detectedName)
		m.nameInput.CursorEnd()
		m.initModeConfirmed = m.recommendedMode
		m.initDecision = InitDecisionInitialize
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

	switch msg.String() {
	case "left", "right":
		return m.activateInitField(), nil
	}
	return m, nil
}

func (m Model) handleConfigFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sections := m.configDraft.SelectorSections()
	if len(sections) == 0 {
		return m, nil
	}
	m.configSectionIdx, m.configFieldIdx, m.configOptionIdx = screens.ClampSelectorState(
		m.configDraft, m.configSectionIdx, m.configFieldIdx, m.configOptionIdx,
	)

	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return m.scroll(msg.String()), nil
	case "up", "k":
		return m.moveConfigFocus(-1), nil
	case "down", "j":
		return m.moveConfigFocus(1), nil
	case "left":
		if m.configPanel == screens.ConfigPanelFooter {
			if m.configFooterIdx > 0 {
				m.configFooterIdx--
				return m, nil
			}
		}
		return m.leaveConfigFields(), nil
	case "right":
		if m.configPanel == screens.ConfigPanelFooter {
			if m.configFooterIdx < m.maxConfigFooterIndex() {
				m.configFooterIdx++
			}
			return m, nil
		}
		return m.enterConfigFields(), nil
	case "enter", " ", "space":
		return m.activateConfigSelector()
	}
	return m, nil
}

func (m Model) currentConfigRowIndex() int {
	rows := screens.FocusRows(m.configDraft, m.configSectionIdx)
	for i, row := range rows {
		if row.FieldIndex == m.configFieldIdx && row.OptionIndex == m.configOptionIdx {
			return i
		}
	}
	return 0
}

func (m Model) applyConfigRow(row screens.ConfigFocusRow) Model {
	m.configFieldIdx = row.FieldIndex
	m.configOptionIdx = row.OptionIndex
	return m
}

func (m Model) enterConfigFields() Model {
	if m.configPanel == screens.ConfigPanelSections {
		m.configPanel = screens.ConfigPanelFields
		rows := screens.FocusRows(m.configDraft, m.configSectionIdx)
		if len(rows) > 0 {
			return m.applyConfigRow(rows[0])
		}
	}
	return m
}

func (m Model) leaveConfigFields() Model {
	if m.configPanel == screens.ConfigPanelFields || m.configPanel == screens.ConfigPanelFooter {
		m.configPanel = screens.ConfigPanelSections
	}
	return m
}

func (m Model) moveConfigFocus(delta int) Model {
	sections := m.configDraft.SelectorSections()
	switch m.configPanel {
	case screens.ConfigPanelSections:
		next := m.configSectionIdx + delta
		if next < 0 || next >= len(sections) {
			return m
		}
		m.configSectionIdx = next
		rows := screens.FocusRows(m.configDraft, m.configSectionIdx)
		if len(rows) > 0 {
			m = m.applyConfigRow(rows[0])
		} else {
			m.configFieldIdx = 0
			m.configOptionIdx = 0
		}
	case screens.ConfigPanelFields:
		rows := screens.FocusRows(m.configDraft, m.configSectionIdx)
		if len(rows) == 0 {
			if delta < 0 {
				m.configPanel = screens.ConfigPanelSections
			} else if m.configShowBack || m.configShowNext {
				m.configPanel = screens.ConfigPanelFooter
				m.configFooterIdx = 0
			}
			return m
		}
		idx := m.currentConfigRowIndex()
		next := idx + delta
		if next < 0 {
			m.configPanel = screens.ConfigPanelSections
			return m
		}
		if next >= len(rows) {
			if m.configShowBack || m.configShowNext {
				m.configPanel = screens.ConfigPanelFooter
				m.configFooterIdx = 0
			}
			return m
		}
		return m.applyConfigRow(rows[next])
	case screens.ConfigPanelFooter:
		if delta < 0 {
			m.configPanel = screens.ConfigPanelFields
			rows := screens.FocusRows(m.configDraft, m.configSectionIdx)
			if len(rows) > 0 {
				return m.applyConfigRow(rows[len(rows)-1])
			}
			return m
		}
		maxFooter := m.maxConfigFooterIndex()
		if m.configFooterIdx < maxFooter {
			m.configFooterIdx++
		}
	default:
		m.configPanel = screens.ConfigPanelSections
	}
	return m
}

func (m Model) activateConfigSelector() (tea.Model, tea.Cmd) {
	sections := m.configDraft.SelectorSections()
	switch m.configPanel {
	case screens.ConfigPanelSections:
		return m.enterConfigFields(), nil
	case screens.ConfigPanelFields:
		if m.configSectionIdx >= len(sections) {
			return m, nil
		}
		fields := sections[m.configSectionIdx].Fields
		if m.configFieldIdx < 0 || m.configFieldIdx >= len(fields) {
			return m, nil
		}
		field := fields[m.configFieldIdx]
		if !field.Editable(m.configDraft.Mode) {
			return m, nil
		}
		switch field.Type {
		case config.FieldTypeBool:
			m.configDraft.ToggleBool(field.Key)
		case config.FieldTypeChoice:
			opts := field.VisibleOptions()
			if m.configOptionIdx < 0 || m.configOptionIdx >= len(opts) {
				return m, nil
			}
			m.configDraft.SelectOption(field.Key, opts[m.configOptionIdx].Value)
		case config.FieldTypeMulti:
			opts := field.VisibleOptions()
			if m.configOptionIdx < 0 || m.configOptionIdx >= len(opts) {
				return m, nil
			}
			m.configDraft.ToggleMulti(field.Key, opts[m.configOptionIdx].Value)
		}
		return m, nil
	case screens.ConfigPanelFooter:
		return m.activateConfigFooter()
	}
	return m, nil
}

func (m Model) activateConfigFooter() (tea.Model, tea.Cmd) {
	action := m.configFooterAction()
	switch action {
	case "back":
		if m.route == RouteConfigure {
			return m.setRoute(DefaultRoute)
		}
		m.initWizardStep = screens.InitWizardStepProject
		m.initConfigConfirmed = false
		m.initField = screens.InitFieldNext
		m.contentOffset = 0
		m.syncNameInputFocus()
		return m, nil
	case "next":
		m.initConfigConfirmed = true
		return m, nil
	}
	return m, nil
}

func (m Model) configFooterAction() string {
	if m.configPanel != screens.ConfigPanelFooter {
		return ""
	}
	if m.configShowBack && m.configShowNext {
		if m.configFooterIdx <= 0 {
			return "back"
		}
		return "next"
	}
	if m.configShowBack {
		return "back"
	}
	if m.configShowNext {
		return "next"
	}
	return ""
}

func (m Model) maxConfigFooterIndex() int {
	count := 0
	if m.configShowBack {
		count++
	}
	if m.configShowNext {
		count++
	}
	if count == 0 {
		return 0
	}
	return count - 1
}

func (m Model) activateInitField() Model {
	switch m.initField {
	case screens.InitFieldModeNew:
		m.initModeConfirmed = InitModeNew
	case screens.InitFieldModeExisting:
		m.initModeConfirmed = InitModeExisting
	case screens.InitFieldDecisionInit:
		m.initDecision = InitDecisionInitialize
	case screens.InitFieldDecisionCancel:
		m.initDecision = InitDecisionCancel
	case screens.InitFieldNext:
		m.initWizardStep = screens.InitWizardStepConfig
		m.initConfigConfirmed = false
		m.rebuildConfigDraft(config.ConfigModeInit, true, true)
		m.contentOffset = 0
		m.syncNameInputFocus()
		return m
	}
	m = m.rebuildInitPlan()
	return m
}

func (m Model) handleMCPContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mcpMode == screens.MCPModeAdd {
		return m.handleMCPAddKey(msg)
	}
	return m.handleMCPListKey(msg)
}

func (m Model) handleMCPListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	servers := len(m.mcpDraft.Servers)
	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		return m.scroll(msg.String()), nil
	case "up", "k":
		switch m.mcpListFocus {
		case screens.MCPFocusClose:
			m.mcpListFocus = screens.MCPFocusAddBtn
			m.mcpFooterIdx = 0
		case screens.MCPFocusAddBtn:
			if servers > 0 {
				m.mcpListFocus = screens.MCPFocusServers
				m.mcpIndex = servers - 1
			}
		case screens.MCPFocusServers:
			if m.mcpIndex > 0 {
				m.mcpIndex--
			}
		}
		return m, nil
	case "down", "j":
		switch m.mcpListFocus {
		case screens.MCPFocusServers:
			if m.mcpIndex < servers-1 {
				m.mcpIndex++
			} else {
				m.mcpListFocus = screens.MCPFocusAddBtn
				m.mcpFooterIdx = 0
			}
		case screens.MCPFocusAddBtn:
			m.mcpListFocus = screens.MCPFocusClose
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "left", "h":
		if m.mcpListFocus == screens.MCPFocusClose {
			m.mcpListFocus = screens.MCPFocusAddBtn
			m.mcpFooterIdx = 0
		}
		return m, nil
	case "right", "l":
		if m.mcpListFocus == screens.MCPFocusAddBtn {
			m.mcpListFocus = screens.MCPFocusClose
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "enter", " ", "space":
		switch m.mcpListFocus {
		case screens.MCPFocusClose:
			return m.setRoute(DefaultRoute)
		case screens.MCPFocusAddBtn:
			m.resetMCPAddForm()
			m.focus = FocusContent
			return m, nil
		case screens.MCPFocusServers:
			m.mcpDraft.ToggleEnabled(m.mcpIndex)
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleMCPAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	kinds := config.MCPKindTemplates()
	editingName := m.mcpAddFocus == screens.MCPFocusName
	editingConn := m.mcpAddFocus == screens.MCPFocusConn

	switch msg.String() {
	case "pgup", "pgdown":
		return m.scroll(msg.String()), nil
	case "up", "k":
		if editingName {
			return m, nil
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusSubmit:
			m.mcpAddFocus = screens.MCPFocusCancel
			m.mcpFooterIdx = 0
		case screens.MCPFocusCancel:
			m.mcpAddFocus = screens.MCPFocusConn
			m.syncMCPInputFocus()
		case screens.MCPFocusConn:
			m.mcpAddFocus = screens.MCPFocusKind
			m.syncMCPInputFocus()
		case screens.MCPFocusKind:
			if m.mcpKindFocus > 0 {
				m.mcpKindFocus--
			} else {
				m.mcpAddFocus = screens.MCPFocusName
				m.syncMCPInputFocus()
			}
		}
		return m, nil
	case "down", "j":
		if editingName {
			m.mcpAddFocus = screens.MCPFocusKind
			m.syncMCPInputFocus()
			return m, nil
		}
		if editingConn {
			m.mcpAddFocus = screens.MCPFocusCancel
			m.mcpFooterIdx = 0
			m.syncMCPInputFocus()
			return m, nil
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusName:
			m.mcpAddFocus = screens.MCPFocusKind
			m.syncMCPInputFocus()
		case screens.MCPFocusKind:
			if m.mcpKindFocus < len(kinds)-1 {
				m.mcpKindFocus++
			} else {
				m.mcpAddFocus = screens.MCPFocusConn
				m.syncMCPInputFocus()
			}
		case screens.MCPFocusCancel:
			m.mcpAddFocus = screens.MCPFocusSubmit
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "left", "right", "h", "l":
		if editingName || editingConn {
			break
		}
		if msg.String() == "left" || msg.String() == "h" {
			if m.mcpAddFocus == screens.MCPFocusSubmit {
				m.mcpAddFocus = screens.MCPFocusCancel
				m.mcpFooterIdx = 0
			}
			return m, nil
		}
		if m.mcpAddFocus == screens.MCPFocusCancel {
			m.mcpAddFocus = screens.MCPFocusSubmit
			m.mcpFooterIdx = 1
		}
		return m, nil
	case "enter":
		if editingName {
			m.mcpAddFocus = screens.MCPFocusKind
			m.syncMCPInputFocus()
			return m, nil
		}
		if editingConn {
			m.mcpAddFocus = screens.MCPFocusCancel
			m.mcpFooterIdx = 0
			m.syncMCPInputFocus()
			return m, nil
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusKind:
			m.mcpKind = kinds[m.mcpKindFocus].Kind
			return m, nil
		case screens.MCPFocusCancel:
			m.leaveMCPAddForm()
			return m, nil
		case screens.MCPFocusSubmit:
			return m.submitMCPAdd()
		}
		return m, nil
	case " ", "space":
		if editingName || editingConn {
			break
		}
		switch m.mcpAddFocus {
		case screens.MCPFocusKind:
			m.mcpKind = kinds[m.mcpKindFocus].Kind
			return m, nil
		case screens.MCPFocusCancel:
			m.leaveMCPAddForm()
			return m, nil
		case screens.MCPFocusSubmit:
			return m.submitMCPAdd()
		}
		return m, nil
	}

	if editingName {
		var cmd tea.Cmd
		m.mcpNameInput, cmd = m.mcpNameInput.Update(msg)
		m.mcpAddError = ""
		return m, cmd
	}
	if editingConn {
		var cmd tea.Cmd
		m.mcpConnInput, cmd = m.mcpConnInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) submitMCPAdd() (tea.Model, tea.Cmd) {
	_, err := m.mcpDraft.AddServer(m.mcpNameInput.Value(), m.mcpKind, m.mcpConnInput.Value())
	if err != nil {
		m.mcpAddError = err.Error()
		m.mcpAddFocus = screens.MCPFocusName
		m.syncMCPInputFocus()
		return m, nil
	}
	m.mcpIndex = len(m.mcpDraft.Servers) - 1
	m.leaveMCPAddForm()
	m.mcpListFocus = screens.MCPFocusServers
	return m, nil
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
	if route == RouteInitPlan {
		m.initWizardStep = screens.InitWizardStepProject
		m.initConfigConfirmed = false
	}
	if route == RouteMCP {
		m.mcpMode = screens.MCPModeList
		m.mcpAddError = ""
		m.mcpFooterIdx = 0
		if len(m.mcpDraft.Servers) == 0 {
			m.mcpListFocus = screens.MCPFocusAddBtn
			m.mcpIndex = 0
		} else {
			m.mcpListFocus = screens.MCPFocusServers
			if m.mcpIndex < 0 || m.mcpIndex >= len(m.mcpDraft.Servers) {
				m.mcpIndex = 0
			}
		}
	}
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
