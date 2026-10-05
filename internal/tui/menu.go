package tui

// SidebarItem is a navigable sidebar entry.
type SidebarItem struct {
	Label string
	Route Route
	Exit  bool
}

// SidebarItems returns sidebar entries for the current Atlas setup state.
// Init / Setup and Configure are mutually exclusive.
func SidebarItems(initialized, showRepair bool) []SidebarItem {
	setup := SidebarItem{Label: "Init / Setup", Route: RouteInitPlan}
	if initialized {
		setup = SidebarItem{Label: "Configure", Route: RouteConfigure}
	}
	items := []SidebarItem{
		{Label: "Dashboard", Route: RouteDashboard},
		setup,
		{Label: "Status", Route: RouteStatus},
		{Label: "Doctor", Route: RouteDoctor},
	}
	if showRepair {
		items = append(items, SidebarItem{Label: "Runtime Repair", Route: RouteRuntimeRepair})
	}
	return append(items,
		SidebarItem{Label: "Help", Route: RouteHelp},
		SidebarItem{Label: "Exit", Exit: true},
	)
}

func clampSidebar(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func indexForRoute(items []SidebarItem, route Route) int {
	for i, item := range items {
		if !item.Exit && item.Route == route {
			return i
		}
	}
	for i, item := range items {
		if !item.Exit && item.Route == DefaultRoute {
			return i
		}
	}
	return 0
}
