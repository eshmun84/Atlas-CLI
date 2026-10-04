package tui

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

// Options configures a TUI session.
type Options struct {
	Route          Route
	UnknownCommand string
	Getwd          func() (string, error)
	Discover       func(string) (workspace.DiscoveryResult, error)
}

// Focus identifies which panel receives navigation keys.
type Focus int

const (
	FocusSidebar Focus = iota
	FocusContent
)

// InitMode is the in-memory Project Mode draft for Init / Setup.
type InitMode string

const (
	InitModeNew      InitMode = "new"
	InitModeExisting InitMode = "existing"
)

// InitDecision is the in-memory runtime artifact gate choice.
type InitDecision string

const (
	InitDecisionInitialize InitDecision = "initialize"
	InitDecisionCancel     InitDecision = "cancel"
)

// Model is the sidebar shell Bubble Tea model.
type Model struct {
	width  int
	height int

	route         Route
	sidebarIndex  int
	contentOffset int
	focus         Focus

	initField           int
	initWizardStep      int
	initModeConfirmed   InitMode
	initDecision        InitDecision
	initHydrated        bool
	initConfigConfirmed bool
	nameInput           textinput.Model
	detectedName        string
	detectedMode        string
	recommendedMode     InitMode

	configDraft      config.ConfigDraft
	configSectionIdx int
	configFieldIdx   int
	configOptionIdx  int
	configPanel      string
	configFooterIdx  int
	configShowBack   bool
	configShowNext   bool

	mcpDraft     config.MCPDraft
	mcpMode      string
	mcpIndex     int
	mcpListFocus string
	mcpFooterIdx int
	mcpAddFocus  string
	mcpKindFocus int
	mcpKind      config.MCPServerKind
	mcpAddError  string
	mcpNameInput textinput.Model
	mcpConnInput textinput.Model

	unknownCommand string
	plan           initplan.Plan
	discovery      workspace.DiscoveryResult
	report         doctor.Report
	loadErr        error
	ready          bool
	quitting       bool

	getwd    func() (string, error)
	discover func(string) (workspace.DiscoveryResult, error)
}

type loadedMsg struct {
	plan      initplan.Plan
	discovery workspace.DiscoveryResult
	report    doctor.Report
	err       error
}

// NewModel builds a TUI model for the given options.
func NewModel(opts Options) Model {
	getwd := opts.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	discover := opts.Discover
	if discover == nil {
		discover = workspace.Discover
	}

	route := opts.Route
	if !validRoute(route) {
		route = DefaultRoute
	}

	items := SidebarItems(false)
	return Model{
		width:             MinWidth,
		height:            MinHeight,
		route:             route,
		sidebarIndex:      indexForRoute(items, route),
		focus:             FocusSidebar,
		initField:         screens.InitFieldName,
		initWizardStep:    screens.InitWizardStepProject,
		initModeConfirmed: InitModeExisting,
		initDecision:      InitDecisionInitialize,
		recommendedMode:   InitModeExisting,
		nameInput:         newTextInput("project name"),
		mcpDraft:          config.EmptyMCPDraft(),
		mcpMode:           screens.MCPModeList,
		mcpListFocus:      screens.MCPFocusAddBtn,
		mcpAddFocus:       screens.MCPFocusName,
		mcpKind:           config.MCPKindCustom,
		mcpKindFocus:      2,
		mcpNameInput:      newTextInput("name"),
		mcpConnInput:      newTextInput("connection"),
		unknownCommand:    opts.UnknownCommand,
		getwd:             getwd,
		discover:          discover,
		ready:             !needsWorkspace(route),
	}
}

func newTextInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 64
	ti.Width = 28
	ti.Prompt = ""
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	ti.CursorEnd()
	return ti
}

func validRoute(route Route) bool {
	switch route {
	case RouteDashboard, RouteInitPlan, RouteConfigure, RouteMCP, RouteStatus, RouteDoctor, RouteHelp, RouteError:
		return true
	default:
		return false
	}
}

func needsWorkspace(route Route) bool {
	switch route {
	case RouteDashboard, RouteInitPlan, RouteConfigure, RouteMCP, RouteStatus, RouteDoctor:
		return true
	default:
		return false
	}
}

func (m Model) Route() Route                    { return m.route }
func (m Model) Width() int                      { return m.width }
func (m Model) Height() int                     { return m.height }
func (m Model) SidebarIndex() int               { return m.sidebarIndex }
func (m Model) ContentOffset() int              { return m.contentOffset }
func (m Model) Focus() Focus                    { return m.focus }
func (m Model) InitModeConfirmed() InitMode     { return m.initModeConfirmed }
func (m Model) InitDecision() InitDecision      { return m.initDecision }
func (m Model) DraftName() string               { return m.nameInput.Value() }
func (m Model) InitField() int                  { return m.initField }
func (m Model) InitWizardStep() int             { return m.initWizardStep }
func (m Model) InitConfigConfirmed() bool       { return m.initConfigConfirmed }
func (m Model) ConfigDraft() config.ConfigDraft { return m.configDraft }
func (m Model) ConfigSectionIndex() int         { return m.configSectionIdx }
func (m Model) ConfigFieldIndex() int           { return m.configFieldIdx }
func (m Model) ConfigOptionIndex() int          { return m.configOptionIdx }
func (m Model) ConfigPanel() string             { return m.configPanel }
func (m Model) ConfigFooterIndex() int          { return m.configFooterIdx }
func (m Model) MCPDraft() config.MCPDraft       { return m.mcpDraft }
func (m Model) MCPMode() string                 { return m.mcpMode }
func (m Model) MCPIndex() int                   { return m.mcpIndex }
func (m Model) MCPListFocus() string            { return m.mcpListFocus }
func (m Model) MCPAddError() string             { return m.mcpAddError }
func (m Model) NameCursor() int                 { return m.nameInput.Position() }
func (m Model) Quitting() bool                  { return m.quitting }
func (m Model) Initialized() bool               { return m.discovery.Atlas.Initialized() }

func (m Model) Sidebar() []SidebarItem {
	return SidebarItems(m.Initialized())
}

func (m Model) ProjectName() string {
	if name := m.nameInput.Value(); name != "" {
		return name
	}
	if m.plan.ProjectName != "" {
		return m.plan.ProjectName
	}
	if m.discovery.Atlas.Initialized() && m.discovery.Atlas.Config.Project.Name != "" {
		return m.discovery.Atlas.Config.Project.Name
	}
	if m.discovery.RootPath != "" {
		return filepath.Base(m.discovery.RootPath)
	}
	return "—"
}

func (m Model) hasRuntimeArtifacts() bool {
	return len(m.discovery.RuntimeArtifacts) > 0
}

func (m Model) initFields() []int {
	fields := []int{
		screens.InitFieldName,
		screens.InitFieldModeNew,
		screens.InitFieldModeExisting,
	}
	if m.hasRuntimeArtifacts() {
		fields = append(fields,
			screens.InitFieldDecisionInit,
			screens.InitFieldDecisionCancel,
		)
	}
	fields = append(fields, screens.InitFieldNext)
	return fields
}

func (m Model) clampInitField(field int) int {
	fields := m.initFields()
	for _, f := range fields {
		if f == field {
			return field
		}
	}
	if len(fields) == 0 {
		return screens.InitFieldName
	}
	return fields[0]
}

func modeFromDetected(detected string) InitMode {
	if detected == "greenfield" {
		return InitModeNew
	}
	return InitModeExisting
}

func (m *Model) syncNameInputFocus() {
	editing := m.focus == FocusContent &&
		m.route == RouteInitPlan &&
		m.initWizardStep == screens.InitWizardStepProject &&
		m.initField == screens.InitFieldName
	if editing {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
	m.syncMCPInputFocus()
}

func (m *Model) syncMCPInputFocus() {
	editingName := m.focus == FocusContent &&
		m.route == RouteMCP &&
		m.mcpMode == screens.MCPModeAdd &&
		m.mcpAddFocus == screens.MCPFocusName
	editingConn := m.focus == FocusContent &&
		m.route == RouteMCP &&
		m.mcpMode == screens.MCPModeAdd &&
		m.mcpAddFocus == screens.MCPFocusConn
	if editingName {
		m.mcpNameInput.Focus()
	} else {
		m.mcpNameInput.Blur()
	}
	if editingConn {
		m.mcpConnInput.Focus()
	} else {
		m.mcpConnInput.Blur()
	}
}

func (m *Model) resetMCPAddForm() {
	m.mcpMode = screens.MCPModeAdd
	m.mcpAddFocus = screens.MCPFocusName
	m.mcpKind = config.MCPKindCustom
	m.mcpKindFocus = 2
	m.mcpAddError = ""
	m.mcpFooterIdx = 0
	m.mcpNameInput.SetValue("")
	m.mcpNameInput.CursorEnd()
	m.mcpConnInput.SetValue("")
	m.mcpConnInput.CursorEnd()
	m.syncMCPInputFocus()
}

func (m *Model) leaveMCPAddForm() {
	m.mcpMode = screens.MCPModeList
	m.mcpAddError = ""
	m.mcpFooterIdx = 0
	if len(m.mcpDraft.Servers) == 0 {
		m.mcpListFocus = screens.MCPFocusAddBtn
	} else {
		m.mcpListFocus = screens.MCPFocusServers
		if m.mcpIndex >= len(m.mcpDraft.Servers) {
			m.mcpIndex = len(m.mcpDraft.Servers) - 1
		}
		if m.mcpIndex < 0 {
			m.mcpIndex = 0
		}
	}
	m.syncMCPInputFocus()
}

func (m Model) projectSetupInput() config.ProjectSetupInput {
	cursor, opencode := false, false
	for _, path := range m.discovery.RuntimeArtifacts {
		switch path {
		case ".cursor":
			cursor = true
		case ".opencode":
			opencode = true
		}
	}
	remote := m.discovery.Git.DefaultRemote
	if remote == "" {
		remote = "origin"
	}
	name := m.nameInput.Value()
	if name == "" {
		name = m.detectedName
	}
	return config.ProjectSetupInput{
		ProjectName:      name,
		ProjectMode:      string(m.initModeConfirmed),
		ProjectID:        config.PreviewProjectID(name),
		DefaultRemote:    remote,
		CursorDetected:   cursor,
		OpenCodeDetected: opencode,
	}
}

func (m *Model) rebuildConfigDraft(mode config.ConfigMode, includeBack, includeNext bool) {
	setup := m.projectSetupInput()
	if mode == config.ConfigModeConfigure && m.discovery.Atlas.Initialized() {
		cfg := m.discovery.Atlas.Config
		if cfg.Project.Name != "" {
			setup.ProjectName = cfg.Project.Name
		}
		if cfg.Project.Mode != "" {
			setup.ProjectMode = cfg.Project.Mode
		}
	}
	setup.ProjectID = config.PreviewProjectID(setup.ProjectName)
	m.configDraft = config.BuildConfigDraft(mode, setup)
	m.configShowBack = includeBack
	m.configShowNext = includeNext
	m.configPanel = screens.ConfigPanelSections
	m.configSectionIdx = 0
	m.configFooterIdx = 0
	rows := screens.FocusRows(m.configDraft, m.configSectionIdx)
	if len(rows) > 0 {
		m.configFieldIdx = rows[0].FieldIndex
		m.configOptionIdx = rows[0].OptionIndex
	} else {
		m.configFieldIdx = 0
		m.configOptionIdx = 0
	}
	m.configSectionIdx, m.configFieldIdx, m.configOptionIdx = screens.ClampSelectorState(
		m.configDraft, m.configSectionIdx, m.configFieldIdx, m.configOptionIdx,
	)
}
