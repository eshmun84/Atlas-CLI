package tui

// Route identifies the initial TUI screen.
type Route int

const (
	RouteHome Route = iota
	RouteHelp
	RouteInitPlan
	RouteStatus
	RouteDoctor
	RouteError
)

func (r Route) String() string {
	switch r {
	case RouteHome:
		return "home"
	case RouteHelp:
		return "help"
	case RouteInitPlan:
		return "init"
	case RouteStatus:
		return "status"
	case RouteDoctor:
		return "doctor"
	case RouteError:
		return "error"
	default:
		return "unknown"
	}
}
