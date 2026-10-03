package tui

// SidebarItem is a navigable sidebar entry.
type SidebarItem struct {
	Label string
	Route Route
	Exit  bool
}

// SidebarItems are the real, currently implemented screens.
var SidebarItems = []SidebarItem{
	{Label: "Init / Setup", Route: RouteInitPlan},
	{Label: "Status", Route: RouteStatus},
	{Label: "Doctor", Route: RouteDoctor},
	{Label: "Help", Route: RouteHelp},
	{Label: "Exit", Exit: true},
}

func clampSidebar(index int) int {
	if index < 0 {
		return 0
	}
	if index >= len(SidebarItems) {
		return len(SidebarItems) - 1
	}
	return index
}

func indexForRoute(route Route) int {
	for i, item := range SidebarItems {
		if !item.Exit && item.Route == route {
			return i
		}
	}
	return indexForRoute(DefaultRoute)
}
