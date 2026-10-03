package tui

// Route identifies the active content panel.
type Route int

const (
	RouteStatus Route = iota
	RouteInitPlan
	RouteDoctor
	RouteHelp
	RouteError
)

// DefaultRoute is the content shown for plain `atlas`.
const DefaultRoute = RouteStatus

func (r Route) String() string {
	switch r {
	case RouteStatus:
		return "Status"
	case RouteInitPlan:
		return "Init / Setup"
	case RouteDoctor:
		return "Doctor"
	case RouteHelp:
		return "Help"
	case RouteError:
		return "Error"
	default:
		return "Unknown"
	}
}
