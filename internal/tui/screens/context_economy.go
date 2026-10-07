package screens

import (
	"fmt"
	"strings"

	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
)

// ContextEconomyView is the Context Economy review renderer input.
type ContextEconomyView struct {
	Plan           atlascontext.UpdatePlan
	Applied        bool
	ApplyMessage   string
	ContentFocused bool
}

// RenderContextEconomy renders the explicit Review -> Apply context update screen.
func RenderContextEconomy(view ContextEconomyView) string {
	plan := view.Plan
	var b strings.Builder

	title := initTitle.Render("Context Economy") + "  " + initMuted.Render("v0 · Index · Capsule · Pack")
	if view.ContentFocused {
		title += "  " + initFocus.Render("[content focus]")
	} else {
		title += "  " + initMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "  "+initMuted.Render("Context Economy v0: implemented, file-based, explicit Update under Atlas Home."))
	fmt.Fprintln(&b, "  "+initMuted.Render("CodeGraph and Atlas Context Graph: NOT IMPLEMENTED."))
	fmt.Fprintln(&b)

	if view.Applied {
		fmt.Fprintln(&b, initSection.Render("Result"))
		fmt.Fprintln(&b, "  "+initOK.Render(atlascontext.UpdateSuccessTitle))
		fmt.Fprintln(&b, "  "+initOK.Render(atlascontext.UpdateSuccessBody))
		if view.ApplyMessage != "" && view.ApplyMessage != atlascontext.UpdateSuccessTitle {
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
		fmt.Fprintln(&b, "  "+initMuted.Render("Update is disabled until blockers are resolved."))
		return strings.TrimRight(b.String(), "\n")
	}

	fmt.Fprintln(&b, initSection.Render("Current status"))
	ce := plan.Current
	fmt.Fprintf(&b, "  State: %s\n", ce.State)
	if ce.ProjectID != "" {
		fmt.Fprintf(&b, "  Project ID: %s\n", ce.ProjectID)
	}
	if ce.IndexPath != "" {
		fmt.Fprintf(&b, "  Index: %s\n", ce.IndexPath)
	}
	if ce.CapsulePath != "" {
		fmt.Fprintf(&b, "  Capsule: %s\n", ce.CapsulePath)
	}
	if ce.Message != "" {
		fmt.Fprintf(&b, "  Note: %s\n", initMuted.Render(ce.Message))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Objective"))
	fmt.Fprintf(&b, "  %s\n\n", plan.Objective)

	if !plan.NeedsApply() && !view.Applied {
		fmt.Fprintln(&b, initSection.Render("Status"))
		fmt.Fprintln(&b, "  "+initOK.Render(atlascontext.UpdateNoopTitle))
		fmt.Fprintln(&b, "  "+initMuted.Render(atlascontext.UpdateNoopBody))
		return strings.TrimRight(b.String(), "\n")
	}

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

	if len(plan.Warnings) > 0 {
		fmt.Fprintln(&b, initSection.Render("Warnings"))
		for _, warning := range plan.Warnings {
			fmt.Fprintf(&b, "  - %s\n", initWarn.Render(warning))
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, initMuted.Render("Writes under Atlas Home projects/<id>/context/ plus minimal .atlas/state.yaml refs."))
	fmt.Fprintln(&b, initMuted.Render("Status/Doctor never create index, capsule, or packs."))
	fmt.Fprintln(&b, initMuted.Render("Runtime Repair does not delete or rewrite Context Economy payloads."))
	fmt.Fprintln(&b, initMuted.Render("Configure Apply does not refresh Context Economy — use this explicit Update flow."))
	return strings.TrimRight(b.String(), "\n")
}
