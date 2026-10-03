package tui

import (
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

// Options configures a TUI session.
type Options struct {
	Route          Route
	UnknownCommand string
	Getwd          func() (string, error)
	Discover       func(string) (workspace.DiscoveryResult, error)
}

// Model is the sidebar shell Bubble Tea model.
type Model struct {
	width  int
	height int

	route         Route
	sidebarIndex  int
	contentOffset int

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
	if route != RouteStatus && route != RouteInitPlan && route != RouteDoctor && route != RouteHelp && route != RouteError {
		route = DefaultRoute
	}

	return Model{
		width:          MinWidth,
		height:         MinHeight,
		route:          route,
		sidebarIndex:   indexForRoute(route),
		unknownCommand: opts.UnknownCommand,
		getwd:          getwd,
		discover:       discover,
		ready:          !needsWorkspace(route),
	}
}

func needsWorkspace(route Route) bool {
	switch route {
	case RouteInitPlan, RouteStatus, RouteDoctor:
		return true
	default:
		return false
	}
}

func (m Model) Route() Route       { return m.route }
func (m Model) Width() int         { return m.width }
func (m Model) Height() int        { return m.height }
func (m Model) SidebarIndex() int  { return m.sidebarIndex }
func (m Model) ContentOffset() int { return m.contentOffset }
func (m Model) Quitting() bool     { return m.quitting }
func (m Model) ProjectName() string {
	if m.plan.ProjectName != "" {
		return m.plan.ProjectName
	}
	if m.discovery.RootPath != "" {
		return filepath.Base(m.discovery.RootPath)
	}
	return "—"
}
