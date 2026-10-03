package tui

import (
	"os"

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

// Model is the full-screen Bubble Tea application model.
type Model struct {
	width  int
	height int

	route    Route
	previous Route
	selected int

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
	route     Route
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

	m := Model{
		width:          MinWidth,
		height:         MinHeight,
		route:          opts.Route,
		selected:       0,
		unknownCommand: opts.UnknownCommand,
		getwd:          getwd,
		discover:       discover,
		ready:          !needsWorkspace(opts.Route),
	}
	if opts.Route != RouteHome {
		m.previous = RouteHome
	}
	return m
}

func needsWorkspace(route Route) bool {
	switch route {
	case RouteInitPlan, RouteStatus, RouteDoctor:
		return true
	default:
		return false
	}
}

// Route returns the current screen route.
func (m Model) Route() Route { return m.route }

// Previous returns the previous route used for back navigation.
func (m Model) Previous() Route { return m.previous }

// Width returns the current terminal width.
func (m Model) Width() int { return m.width }

// Height returns the current terminal height.
func (m Model) Height() int { return m.height }

// Selected returns the Home menu index.
func (m Model) Selected() int { return m.selected }

// Quitting reports whether the model requested exit.
func (m Model) Quitting() bool { return m.quitting }
