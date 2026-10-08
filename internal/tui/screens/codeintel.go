package screens

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
)

// CodeIntelRefreshView is the Review → Apply renderer for codeintel refresh.
type CodeIntelRefreshView struct {
	Plan           codeintel.RefreshPlan
	Outcome        codeintel.RefreshOutcome
	Applied        bool
	ApplyMessage   string
	ContentFocused bool
}

// RenderCodeIntelRefresh renders the explicit Code Intelligence refresh screen.
func RenderCodeIntelRefresh(view CodeIntelRefreshView) string {
	plan := view.Plan
	var b strings.Builder

	title := initTitle.Render("Code Intelligence") + "  " + initMuted.Render("Refresh graph")
	if view.ContentFocused {
		title += "  " + initFocus.Render("[content focus]")
	} else {
		title += "  " + initMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b)

	if view.Applied {
		fmt.Fprintln(&b, initSection.Render("Result"))
		msg := view.ApplyMessage
		if msg == "" {
			msg = view.Outcome.Message
		}
		fmt.Fprintln(&b, "  "+initOK.Render(msg))
		if view.Outcome.Mode != "" {
			fmt.Fprintf(&b, "  Mode: %s\n", view.Outcome.Mode)
		}
		if view.Outcome.Containment.JournalRemoved {
			fmt.Fprintln(&b, "  "+initMuted.Render("Contained .codegraph/changes.journal"))
		}
		for _, w := range view.Outcome.Warnings {
			fmt.Fprintf(&b, "  - %s\n", initWarn.Render(w))
		}
		return strings.TrimRight(b.String(), "\n")
	}

	if view.ApplyMessage != "" {
		fmt.Fprintln(&b, initSection.Render("Result"))
		fmt.Fprintln(&b, "  "+initWarn.Render(view.ApplyMessage))
		fmt.Fprintln(&b)
	}

	if plan.Blocked {
		fmt.Fprintln(&b, initSection.Render("Blockers"))
		for _, blocker := range plan.Blockers {
			fmt.Fprintf(&b, "  - %s\n", initWarn.Render(blocker))
		}
		fmt.Fprintln(&b, "  "+initMuted.Render("Refresh is disabled until blockers are resolved."))
		return strings.TrimRight(b.String(), "\n")
	}

	if plan.Noop {
		fmt.Fprintln(&b, initSection.Render("Status"))
		fmt.Fprintln(&b, "  "+initOK.Render("Graph already fresh."))
		fmt.Fprintln(&b, "  "+initMuted.Render("Apply is a no-op. Status and Doctor remain read-only."))
		return strings.TrimRight(b.String(), "\n")
	}

	fmt.Fprintln(&b, initSection.Render("Planned refresh"))
	fmt.Fprintf(&b, "  Mode: %s\n", plan.Mode)
	if plan.ForceFull {
		fmt.Fprintln(&b, "  Force full: yes")
	}
	fmt.Fprintf(&b, "  Provider: %s\n", plan.Provider)
	if plan.Version != "" {
		fmt.Fprintf(&b, "  Version: %s\n", plan.Version)
	}
	fmt.Fprintf(&b, "  Graph present: %v\n", plan.GraphPresent)
	if plan.Freshness != "" {
		fmt.Fprintf(&b, "  Freshness: %s\n", plan.Freshness)
	}
	if plan.Reason != "" {
		fmt.Fprintf(&b, "  Reason: %s\n", plan.Reason)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, initSection.Render("Atlas Home paths"))
	fmt.Fprintf(&b, "  DB: %s\n", plan.GraphDBPath)
	fmt.Fprintf(&b, "  Metadata: %s\n", plan.MetadataPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, initMuted.Render("Apply runs an explicit CodeGraph build. Status/Doctor never refresh."))
	return strings.TrimRight(b.String(), "\n")
}
