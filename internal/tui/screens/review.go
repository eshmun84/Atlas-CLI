package screens

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

// ReviewView is the Init Step 3 preview renderer input.
type ReviewView struct {
	Plan           initplan.MaterializationPlan
	ApplyMessage   string
	ContentFocused bool
}

// RenderReview renders Init / Setup Step 3 — Review / Materialization Plan.
func RenderReview(view ReviewView) string {
	plan := view.Plan
	var b strings.Builder

	title := initTitle.Render("Init / Setup") + "  " + initMuted.Render("Step 3 — Review / Materialization Plan")
	if view.ContentFocused {
		title += "  " + initFocus.Render("[content focus]")
	} else {
		title += "  " + initMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Review / Materialization Plan"))
	fmt.Fprintln(&b, "  "+initMuted.Render("No files will be changed in this slice."))
	if view.ApplyMessage != "" {
		fmt.Fprintln(&b, "  "+initWarn.Render(view.ApplyMessage))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Summary"))
	fmt.Fprintln(&b, initLabel.Render("Project:"))
	fmt.Fprintf(&b, "  - Name: %s\n", plan.ProjectName)
	fmt.Fprintf(&b, "  - Mode: %s\n", plan.ProjectModeLabel)
	fmt.Fprintln(&b, initLabel.Render("Configuration:"))
	fmt.Fprintf(&b, "  - Workflow: %s\n", plan.Workflow)
	fmt.Fprintf(&b, "  - Spec engine: %s\n", plan.SpecEngine)
	fmt.Fprintf(&b, "  - Adapters: %s\n", plan.Adapters)
	fmt.Fprintf(&b, "  - Source control: %s\n", plan.SourceControl)
	fmt.Fprintf(&b, "  - Branch strategy: %s\n", plan.BranchStrategy)
	fmt.Fprintf(&b, "  - Atlas governance files: %s\n", plan.GovernanceStorage)
	fmt.Fprintf(&b, "  - Memory strategy: %s\n", plan.MemoryStrategy)
	fmt.Fprintf(&b, "  - MCP integrations: %d configured\n", plan.MCPCount)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Files Atlas would create"))
	for _, file := range plan.Creates {
		status := file.Status
		if status == "" {
			status = "planned"
		}
		fmt.Fprintf(&b, "  - %s (%s)\n", file.Path, status)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Existing artifacts detected"))
	if len(plan.ExistingArtifacts) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("No existing runtime artifacts detected."))
	} else {
		fmt.Fprintln(&b, "  Existing runtime artifacts detected:")
		for _, path := range plan.ExistingArtifacts {
			fmt.Fprintf(&b, "  - %s\n", path)
		}
		fmt.Fprintln(&b, "  "+initMuted.Render("Atlas would backup these files/directories before replacing Atlas-managed runtime entrypoints."))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Files Atlas would backup"))
	if len(plan.Backups) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("No backups required."))
	} else {
		fmt.Fprintf(&b, "  Backup root: %s\n", initplan.BackupPlaceholderDir())
		for _, backup := range plan.Backups {
			fmt.Fprintf(&b, "  - %s\n", backup.Path)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Files Atlas would replace"))
	if len(plan.Replacements) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("No existing runtime files need replacement."))
	} else {
		fmt.Fprintln(&b, "  Atlas would replace runtime artifacts after backup.")
		for _, repl := range plan.Replacements {
			fmt.Fprintf(&b, "  - %s\n", repl.Path)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Files Atlas would preserve"))
	for _, item := range plan.Preservations {
		fmt.Fprintf(&b, "  - %s\n", item.Statement)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Governance storage"))
	fmt.Fprintln(&b, "  "+plan.GovernanceNote)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("MCP integrations"))
	if len(plan.MCPEntries) == 0 {
		fmt.Fprintln(&b, "  - none configured")
	} else {
		for _, entry := range plan.MCPEntries {
			fmt.Fprintf(&b, "  - %s\n", entry.Name)
			fmt.Fprintf(&b, "    kind: %s\n", entry.Kind)
			if entry.Kind == "custom" && entry.Transport != "" {
				fmt.Fprintf(&b, "    transport: %s\n", entry.Transport)
			}
			fmt.Fprintf(&b, "    enabled: %t\n", entry.Enabled)
			fmt.Fprintf(&b, "    status: %s\n", entry.Status)
		}
	}
	fmt.Fprintln(&b, "  "+initMuted.Render("No MCP credentials, connections, or validation are implemented in this slice."))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Warnings / blockers"))
	for _, warn := range plan.Warnings {
		fmt.Fprintf(&b, "  - %s\n", warn.Message)
	}
	if len(plan.Blockers) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("No blockers. Apply is not implemented in this slice."))
	} else {
		for _, blocker := range plan.Blockers {
			fmt.Fprintf(&b, "  - %s\n", initWarn.Render(blocker.Message))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
