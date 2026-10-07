package tui

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
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
	initReviewPlan      initplan.MaterializationPlan
	initReviewMessage   string
	initReviewFooterIdx int
	initApplied         bool
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
	configureNotice  string

	mcpDraft          config.MCPDraft
	mcpMode           string
	mcpIndex          int
	mcpListFocus      string
	mcpFooterIdx      int
	mcpAddFocus       string
	mcpTransportFocus int
	mcpTransport      config.MCPTransport
	mcpAddError       string
	mcpNotice         string
	mcpNameInput      textinput.Model
	mcpConnInput      textinput.Model
	mcpArgsInput      textinput.Model
	mcpEnvInput       textinput.Model

	unknownCommand  string
	plan            initplan.Plan
	discovery       workspace.DiscoveryResult
	report          doctor.Report
	repairPlan      workspace.RuntimeRepairPlan
	repairResult    workspace.RuntimeRepairResult
	repairMessage   string
	repairFooterIdx int
	repairApplied   bool
	repairSignature string

	contextPlan      atlascontext.UpdatePlan
	contextResult    atlascontext.UpdateResult
	contextMessage   string
	contextFooterIdx int
	contextApplied   bool
	contextSignature string
	contextObjective string

	loadErr  error
	ready    bool
	quitting bool

	getwd    func() (string, error)
	discover func(string) (workspace.DiscoveryResult, error)
}

type loadedMsg struct {
	plan        initplan.Plan
	discovery   workspace.DiscoveryResult
	report      doctor.Report
	repairPlan  workspace.RuntimeRepairPlan
	contextPlan atlascontext.UpdatePlan
	err         error
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

	items := SidebarItems(false, false, false)
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
		mcpDraft:          config.DefaultMCPDraft(),
		mcpMode:           screens.MCPModeList,
		mcpListFocus:      screens.MCPFocusBuiltins,
		mcpAddFocus:       screens.MCPFocusName,
		mcpTransport:      config.MCPTransportStdio,
		mcpTransportFocus: 0,
		mcpNameInput:      newTextInput("name"),
		mcpConnInput:      newTextInput("command or url"),
		mcpArgsInput:      newTextInput("arguments"),
		mcpEnvInput:       newTextInput("environment references"),
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
	case RouteInitPlan, RouteConfigure, RouteStatus, RouteDoctor, RouteRuntimeRepair, RouteContextEconomy, RouteHelp, RouteError:
		return true
	default:
		return false
	}
}

func needsWorkspace(route Route) bool {
	switch route {
	case RouteInitPlan, RouteConfigure, RouteStatus, RouteDoctor, RouteRuntimeRepair, RouteContextEconomy:
		return true
	default:
		return false
	}
}

func (m Model) Route() Route                            { return m.route }
func (m Model) Width() int                              { return m.width }
func (m Model) Height() int                             { return m.height }
func (m Model) SidebarIndex() int                       { return m.sidebarIndex }
func (m Model) ContentOffset() int                      { return m.contentOffset }
func (m Model) Focus() Focus                            { return m.focus }
func (m Model) InitModeConfirmed() InitMode             { return m.initModeConfirmed }
func (m Model) InitDecision() InitDecision              { return m.initDecision }
func (m Model) DraftName() string                       { return m.nameInput.Value() }
func (m Model) InitField() int                          { return m.initField }
func (m Model) InitWizardStep() int                     { return m.initWizardStep }
func (m Model) InitReviewMessage() string               { return m.initReviewMessage }
func (m Model) ConfigureNotice() string                 { return m.configureNotice }
func (m Model) InitApplied() bool                       { return m.initApplied }
func (m Model) ConfigDraft() config.ConfigDraft         { return m.configDraft }
func (m Model) ConfigSectionIndex() int                 { return m.configSectionIdx }
func (m Model) ConfigFieldIndex() int                   { return m.configFieldIdx }
func (m Model) ConfigOptionIndex() int                  { return m.configOptionIdx }
func (m Model) ConfigPanel() string                     { return m.configPanel }
func (m Model) ConfigFooterIndex() int                  { return m.configFooterIdx }
func (m Model) MCPDraft() config.MCPDraft               { return m.mcpDraft }
func (m Model) MCPMode() string                         { return m.mcpMode }
func (m Model) MCPIndex() int                           { return m.mcpIndex }
func (m Model) MCPListFocus() string                    { return m.mcpListFocus }
func (m Model) MCPAddError() string                     { return m.mcpAddError }
func (m Model) MCPNotice() string                       { return m.mcpNotice }
func (m Model) MCPAddFocus() string                     { return m.mcpAddFocus }
func (m Model) MCPAddName() string                      { return m.mcpNameInput.Value() }
func (m Model) MCPAddConn() string                      { return m.mcpConnInput.Value() }
func (m Model) MCPAddArgs() string                      { return m.mcpArgsInput.Value() }
func (m Model) MCPAddEnv() string                       { return m.mcpEnvInput.Value() }
func (m Model) RepairApplied() bool                     { return m.repairApplied }
func (m Model) RepairMessage() string                   { return m.repairMessage }
func (m Model) RepairPlan() workspace.RuntimeRepairPlan { return m.repairPlan }
func (m Model) ContextApplied() bool                    { return m.contextApplied }
func (m Model) ContextMessage() string                  { return m.contextMessage }
func (m Model) ContextPlan() atlascontext.UpdatePlan    { return m.contextPlan }
func (m Model) NameCursor() int                         { return m.nameInput.Position() }
func (m Model) Quitting() bool                          { return m.quitting }

func (m Model) editingTextInput() bool {
	if m.focus != FocusContent {
		return false
	}
	if m.route == RouteInitPlan &&
		m.initWizardStep == screens.InitWizardStepProject &&
		m.initField == screens.InitFieldName {
		return true
	}
	return m.mcpAddActive() && m.mcpEditingAddText()
}
func (m Model) Initialized() bool { return m.discovery.Atlas.Initialized() }

func (m Model) Sidebar() []SidebarItem {
	return SidebarItems(m.Initialized(), workspace.ShowRuntimeRepair(m.discovery), m.Initialized())
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

func (m Model) mcpAddActive() bool {
	if m.mcpMode != screens.MCPModeAdd {
		return false
	}
	if m.route == RouteConfigure {
		return true
	}
	return m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepConfig
}

func (m *Model) syncMCPInputFocus() {
	active := m.focus == FocusContent && m.mcpAddActive()
	setInputFocus(&m.mcpNameInput, active && m.mcpAddFocus == screens.MCPFocusName)
	setInputFocus(&m.mcpConnInput, active && m.mcpAddFocus == screens.MCPFocusConn)
	setInputFocus(&m.mcpArgsInput, active && m.mcpAddFocus == screens.MCPFocusArgs)
	setInputFocus(&m.mcpEnvInput, active && m.mcpAddFocus == screens.MCPFocusEnv)
}

func setInputFocus(ti *textinput.Model, focused bool) {
	if focused {
		ti.Focus()
		return
	}
	ti.Blur()
}

func (m *Model) resetMCPAddForm() {
	m.mcpMode = screens.MCPModeAdd
	m.mcpAddFocus = screens.MCPFocusName
	m.mcpTransport = config.MCPTransportStdio
	m.mcpTransportFocus = 0
	m.mcpAddError = ""
	m.mcpNotice = ""
	m.mcpFooterIdx = 0
	m.mcpNameInput.SetValue("")
	m.mcpNameInput.CursorEnd()
	m.mcpConnInput.SetValue("")
	m.mcpConnInput.CursorEnd()
	m.mcpArgsInput.SetValue("")
	m.mcpArgsInput.CursorEnd()
	m.mcpEnvInput.SetValue("")
	m.mcpEnvInput.CursorEnd()
	m.syncMCPInputFocus()
}

func (m *Model) leaveMCPAddForm() {
	m.mcpMode = screens.MCPModeList
	m.mcpAddError = ""
	m.mcpFooterIdx = 0
	if m.route == RouteInitPlan || m.route == RouteConfigure {
		idx := m.mcpSelectorIndex()
		if idx >= 0 {
			m.configSectionIdx = idx
			m.configPanel = screens.ConfigPanelFields
		}
	}
	if len(m.mcpDraft.CustomServers) == 0 {
		m.mcpListFocus = screens.MCPFocusBuiltins
		m.mcpIndex = 0
	} else {
		m.mcpListFocus = screens.MCPFocusCustom
		if m.mcpIndex >= len(m.mcpDraft.CustomServers) {
			m.mcpIndex = len(m.mcpDraft.CustomServers) - 1
		}
		if m.mcpIndex < 0 {
			m.mcpIndex = 0
		}
	}
	m.syncMCPInputFocus()
}

func (m Model) mcpSelectorIndex() int {
	for i, section := range m.configDraft.SelectorSections() {
		if section.Key == "mcp" {
			return i
		}
	}
	return -1
}

func (m Model) mcpSectionActive() bool {
	sections := m.configDraft.SelectorSections()
	if m.configSectionIdx < 0 || m.configSectionIdx >= len(sections) {
		return false
	}
	return sections[m.configSectionIdx].Key == "mcp"
}

func (m Model) mcpView() screens.MCPView {
	return screens.MCPView{
		Mode:              m.mcpMode,
		Draft:             m.mcpDraft,
		ActiveIndex:       m.mcpIndex,
		TransportFocus:    m.mcpTransportFocus,
		TransportSelected: m.mcpTransport,
		Initialized:       m.Initialized(),
		ContentFocused:    m.focus == FocusContent,
		ListFocus:         m.mcpListFocus,
		AddFocus:          m.mcpAddFocus,
		NameView:          m.mcpNameInput.View(),
		ConnectionView:    m.mcpConnInput.View(),
		ArgsView:          m.mcpArgsInput.View(),
		EnvView:           m.mcpEnvInput.View(),
		Error:             m.mcpAddError,
		Notice:            m.mcpNotice,
	}
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
	var persisted *config.ProjectDocument
	if mode == config.ConfigModeConfigure && m.discovery.Atlas.Initialized() {
		cfg := m.discovery.Atlas.Config
		if cfg.Project.Name != "" {
			setup.ProjectName = cfg.Project.Name
		}
		if cfg.Project.Mode != "" {
			setup.ProjectMode = cfg.Project.Mode
		}
		if doc, err := config.LoadProjectDocument(m.discovery.Atlas.ConfigPath); err == nil {
			persisted = &doc
			setup.ProjectName = doc.Project.Name
			setup.ProjectMode = doc.Project.Mode
			if doc.SourceControl.DefaultRemote != "" {
				setup.DefaultRemote = doc.SourceControl.DefaultRemote
			}
			setup.CursorDetected = containsString(doc.Adapters.Selected, "cursor")
			setup.OpenCodeDetected = containsString(doc.Adapters.Selected, "opencode")
		}
	}
	setup.ProjectID = config.PreviewProjectID(setup.ProjectName)
	m.configDraft = config.BuildConfigDraft(mode, setup)
	if persisted != nil {
		config.ApplyProjectDocument(&m.configDraft, *persisted)
		m.mcpDraft = persisted.ToMCPDraft()
	}
	m.configShowBack = includeBack
	m.configShowNext = includeNext
	m.configPanel = screens.ConfigPanelSections
	m.configSectionIdx = 0
	m.configFooterIdx = 0
	m.mcpMode = screens.MCPModeList
	m.mcpAddError = ""
	m.mcpNotice = ""
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
