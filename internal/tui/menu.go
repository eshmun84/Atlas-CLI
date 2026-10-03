package tui

// HomeItem is a selectable Home menu entry.
type HomeItem struct {
	Label string
	Route Route
	Exit  bool
}

// HomeItems is the Home screen menu.
var HomeItems = []HomeItem{
	{Label: "Init / Setup", Route: RouteInitPlan},
	{Label: "Status", Route: RouteStatus},
	{Label: "Doctor", Route: RouteDoctor},
	{Label: "Help", Route: RouteHelp},
	{Label: "Exit", Exit: true},
}

func clampSelected(index int) int {
	if index < 0 {
		return 0
	}
	if index >= len(HomeItems) {
		return len(HomeItems) - 1
	}
	return index
}
