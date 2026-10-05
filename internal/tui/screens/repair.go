package screens

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

// RepairView is the Runtime Repair review renderer input.
type RepairView struct {
	Plan           workspace.RuntimeRepairPlan
	Applied        bool
	ApplyMessage   string
	ContentFocused bool
}

// RenderRuntimeRepair renders the explicit Review -> Apply repair screen.
func RenderRuntimeRepair(view RepairView) string {
	plan := view.Plan
	var b strings.Builder

	title := initTitle.Render("Runtime Repair") + "  " + initMuted.Render("Review runtime drift")
	if view.ContentFocused {
		title += "  " + initFocus.Render("[content focus]")
	} else {
		title += "  " + initMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b)

	if view.Applied {
		fmt.Fprintln(&b, initSection.Render("Result"))
		fmt.Fprintln(&b, "  "+initOK.Render(workspace.RepairSuccessTitle))
		fmt.Fprintln(&b, "  "+initOK.Render(workspace.RepairSuccessBody))
		if view.ApplyMessage != "" && view.ApplyMessage != workspace.RepairSuccessTitle {
			fmt.Fprintln(&b, "  "+initWarn.Render(view.ApplyMessage))
		}
		fmt.Fprintln(&b)
	} else if view.ApplyMessage != "" {
		fmt.Fprintln(&b, initSection.Render("Result"))
		fmt.Fprintln(&b, "  "+initWarn.Render(view.ApplyMessage))
		fmt.Fprintln(&b)
	}

	if plan.Blocked {
		fmt.Fprintln(&b, initSection.Render("Blockers"))
		for _, blocker := range plan.Blockers {
			fmt.Fprintf(&b, "  - %s\n", initWarn.Render(blocker))
		}
		fmt.Fprintln(&b, "  "+initMuted.Render("Repair is disabled until blockers are resolved."))
		return strings.TrimRight(b.String(), "\n")
	}

	if plan.Healthy && !view.Applied {
		fmt.Fprintln(&b, initSection.Render("Status"))
		fmt.Fprintln(&b, "  "+initOK.Render(workspace.RepairNoopTitle))
		fmt.Fprintln(&b, "  "+initMuted.Render(workspace.RepairNoopBody))
		fmt.Fprintln(&b, "  "+initMuted.Render("Status and Doctor remain read-only. Apply is a no-op."))
		return strings.TrimRight(b.String(), "\n")
	}

	fmt.Fprintln(&b, initSection.Render("Detected drift"))
	if len(plan.Drift) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("none"))
	} else {
		for _, item := range plan.Drift {
			fmt.Fprintf(&b, "  - %s\n", item)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Planned actions"))
	if len(plan.Targets) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("none"))
	} else {
		for _, target := range plan.Targets {
			fmt.Fprintf(&b, "  - %s %s (%s)\n", target.Action, target.Path, target.Reason)
		}
	}
	fmt.Fprintln(&b)

	writePathSection(&b, "Files to create", plan.Creates)
	writePathSection(&b, "Files to replace", plan.Replaces)
	writePathSection(&b, "Conflicts to backup/quarantine", plan.Quarantines)
	writePathSection(&b, "Backups to create", plan.Backups)

	if len(plan.Warnings) > 0 {
		fmt.Fprintln(&b, initSection.Render("Warnings"))
		for _, warning := range plan.Warnings {
			fmt.Fprintf(&b, "  - %s\n", initWarn.Render(warning))
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, initMuted.Render("Backup is mandatory for conflicts. No skip, merge, or silent delete."))
	fmt.Fprintln(&b, initMuted.Render("Apply recomputes this plan immediately before writing."))
	return strings.TrimRight(b.String(), "\n")
}

func writePathSection(b *strings.Builder, title string, paths []string) {
	fmt.Fprintln(b, initSection.Render(title))
	if len(paths) == 0 {
		fmt.Fprintln(b, "  "+initMuted.Render("none"))
	} else {
		for _, path := range paths {
			fmt.Fprintf(b, "  - %s\n", path)
		}
	}
	fmt.Fprintln(b)
}
