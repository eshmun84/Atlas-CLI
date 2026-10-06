package tui

// Route identifies the active content panel.
type Route int

const (
	RouteDashboard Route = iota
	RouteInitPlan
	RouteConfigure
	RouteMCP
	RouteStatus
	RouteDoctor
	RouteRuntimeRepair
	RouteContextEconomy
	RouteHelp
	RouteError
)

// DefaultRoute is the content shown for plain `atlas`.
const DefaultRoute = RouteDashboard

func (r Route) String() string {
	switch r {
	case RouteDashboard:
		return "Dashboard"
	case RouteInitPlan:
		return "Init / Setup"
	case RouteConfigure:
		return "Configure"
	case RouteMCP:
		return "MCP"
	case RouteStatus:
		return "Status"
	case RouteDoctor:
		return "Doctor"
	case RouteRuntimeRepair:
		return "Runtime Repair"
	case RouteContextEconomy:
		return "Context Economy"
	case RouteHelp:
		return "Help"
	case RouteError:
		return "Error"
	default:
		return "Unknown"
	}
}
