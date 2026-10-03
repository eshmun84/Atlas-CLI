package tui

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
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

	initField         int
	initModeConfirmed InitMode
	initDecision      InitDecision
	initHydrated      bool
	initStepConfirmed bool
	nameInput         textinput.Model
	detectedName      string
	detectedMode      string
	recommendedMode   InitMode

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
		initModeConfirmed: InitModeExisting,
		initDecision:      InitDecisionInitialize,
		recommendedMode:   InitModeExisting,
		nameInput:         newNameInput(""),
		unknownCommand:    opts.UnknownCommand,
		getwd:             getwd,
		discover:          discover,
		ready:             !needsWorkspace(route),
	}
}

func newNameInput(value string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "project name"
	ti.CharLimit = 64
	ti.Width = 32
	ti.Prompt = ""
	// Keep value/cursor neutral; the bordered field chrome is rendered by the Init screen.
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	ti.SetValue(value)
	ti.CursorEnd()
	return ti
}

func validRoute(route Route) bool {
	switch route {
	case RouteDashboard, RouteInitPlan, RouteConfigure, RouteStatus, RouteDoctor, RouteHelp, RouteError:
		return true
	default:
		return false
	}
}

func needsWorkspace(route Route) bool {
	switch route {
	case RouteDashboard, RouteInitPlan, RouteConfigure, RouteStatus, RouteDoctor:
		return true
	default:
		return false
	}
}

func (m Model) Route() Route                { return m.route }
func (m Model) Width() int                  { return m.width }
func (m Model) Height() int                 { return m.height }
func (m Model) SidebarIndex() int           { return m.sidebarIndex }
func (m Model) ContentOffset() int          { return m.contentOffset }
func (m Model) Focus() Focus                { return m.focus }
func (m Model) InitModeConfirmed() InitMode { return m.initModeConfirmed }
func (m Model) InitDecision() InitDecision  { return m.initDecision }
func (m Model) DraftName() string           { return m.nameInput.Value() }
func (m Model) InitField() int              { return m.initField }
func (m Model) InitStepConfirmed() bool     { return m.initStepConfirmed }
func (m Model) NameCursor() int             { return m.nameInput.Position() }
func (m Model) Quitting() bool              { return m.quitting }
func (m Model) Initialized() bool           { return m.discovery.Atlas.Initialized() }

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
	if m.focus == FocusContent && m.initField == screens.InitFieldName {
		m.nameInput.Focus()
		return
	}
	m.nameInput.Blur()
}
