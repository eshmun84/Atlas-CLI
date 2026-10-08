package tui

// Route identifies the active content panel.
type Route int

const (
	RouteInitPlan Route = iota
	RouteConfigure
	RouteMCP
	RouteStatus
	RouteDoctor
	RouteRuntimeRepair
	RouteContextEconomy
	RouteCodeIntelRefresh
	RouteHelp
	RouteError
)

// DefaultRoute is the content shown for plain `atlas` (Status overview).
const DefaultRoute = RouteStatus

func (r Route) String() string {
	switch r {
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
	case RouteCodeIntelRefresh:
		return "Code Intelligence"
	case RouteHelp:
		return "Help"
	case RouteError:
		return "Error"
	default:
		return "Unknown"
	}
}
