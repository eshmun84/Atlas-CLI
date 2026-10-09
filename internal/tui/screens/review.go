package screens

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

// ReviewView is the Init Step 3 preview renderer input.
type ReviewView struct {
	Plan           initplan.MaterializationPlan
	ApplyMessage   string
	Applied        bool
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
	if view.Applied {
		fmt.Fprintln(&b, "  "+initOK.Render(config.ApplySuccessTitle))
		fmt.Fprintln(&b, "  "+initOK.Render(config.ApplySuccessBody))
		if view.ApplyMessage != "" && view.ApplyMessage != config.ApplySuccessTitle {
			fmt.Fprintln(&b, "  "+initWarn.Render(view.ApplyMessage))
		}
	} else {
		fmt.Fprintln(&b, "  "+initMuted.Render("Apply is the only mutation step. Status and Doctor remain read-only."))
		if view.ApplyMessage != "" {
			fmt.Fprintln(&b, "  "+initWarn.Render(view.ApplyMessage))
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Summary"))
	fmt.Fprintln(&b, initLabel.Render("Project:"))
	fmt.Fprintf(&b, "  - Name: %s\n", plan.ProjectName)
	if plan.ProjectRoot != "" {
		fmt.Fprintf(&b, "  - Root: %s\n", plan.ProjectRoot)
	}
	fmt.Fprintf(&b, "  - Mode: %s\n", plan.ProjectModeLabel)
	fmt.Fprintln(&b, initLabel.Render("Governance:"))
	fmt.Fprintf(&b, "  - Workflow: %s\n", plan.Workflow)
	fmt.Fprintf(&b, "  - Spec engine: %s\n", plan.SpecEngine)
	fmt.Fprintf(&b, "  - Testing required: %s\n", plan.TestingRequired)
	fmt.Fprintf(&b, "  - Review required: %s\n", plan.ReviewRequired)
	fmt.Fprintf(&b, "  - Evidence required: %s\n", plan.EvidenceRequired)
	fmt.Fprintln(&b, initLabel.Render("Adapters:"))
	fmt.Fprintf(&b, "  - Selected: %s\n", plan.Adapters)
	fmt.Fprintln(&b, initLabel.Render("Delivery:"))
	fmt.Fprintf(&b, "  - Platform: %s\n", plan.DeliveryPlatform)
	fmt.Fprintf(&b, "  - Governance files: %s\n", plan.GovernanceStorage)
	fmt.Fprintf(&b, "  - Assisted operations: %s\n", plan.DeliveryAssistance)
	fmt.Fprintf(&b, "  - MCP desired state: %d selected\n", plan.MCPCount)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Project writes"))
	for _, file := range plan.Creates {
		status := file.Status
		if status == "" {
			status = "planned"
		}
		fmt.Fprintf(&b, "  - %s (%s)\n", file.Path, status)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Atlas Home writes"))
	if len(plan.HomeWrites) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("none"))
	} else {
		for _, file := range plan.HomeWrites {
			fmt.Fprintf(&b, "  - %s (%s)\n", file.Path, file.Status)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Atlas Home reset"))
	switch plan.HomeDataPresence {
	case "unknown":
		msg := "could not inspect Atlas Home project data"
		if plan.HomeDataError != "" {
			msg = msg + ": " + plan.HomeDataError
		}
		fmt.Fprintln(&b, "  "+initWarn.Render(msg))
		fmt.Fprintln(&b, "  "+initMuted.Render("Do not treat Home data as absent; Apply re-checks before mutation."))
	case "present":
		fmt.Fprintf(&b, "  Project name: %s\n", plan.ProjectName)
		if plan.ProjectRoot != "" {
			fmt.Fprintf(&b, "  Project root: %s\n", plan.ProjectRoot)
		}
		fmt.Fprintln(&b, "  "+initWarn.Render("Home project data detected"))
		fmt.Fprintln(&b, "  Action: reset local Atlas data before initialization")
		for _, file := range plan.HomeReset {
			fmt.Fprintf(&b, "  - %s (%s)\n", file.Path, file.Status)
		}
		fmt.Fprintln(&b, "  "+initMuted.Render("Press x to accept. Deletes only this project's Home data."))
		marker := "[ ]"
		if plan.AcceptHomeReset {
			marker = "[x]"
		}
		label := "I accept resetting local Atlas Home data for this project"
		if view.ContentFocused && !view.Applied {
			fmt.Fprintln(&b, "  "+initSelected.Render(marker+" "+label))
		} else {
			fmt.Fprintln(&b, "  "+initOption.Render(marker+" "+label))
		}
	default:
		fmt.Fprintln(&b, "  "+initMuted.Render("none"))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Config vs later flows"))
	fmt.Fprintln(&b, "  "+initMuted.Render("Init Apply writes config and materializes selected runtime files."))
	fmt.Fprintln(&b, "  "+initMuted.Render("Later Configure Apply saves .atlas/config.yaml and reconciles MCP projections when MCP/adapters change."))
	fmt.Fprintln(&b, "  "+initMuted.Render("Runtime Repair remains required for non-MCP runtime artifacts (AGENTS.md, rules, agents)."))
	fmt.Fprintln(&b, "  "+initMuted.Render("Context Economy Update is a separate explicit flow."))
	fmt.Fprintln(&b, "  "+initMuted.Render("MCP desired state is stored in Atlas config; Atlas-owned projections materialize to selected agents."))
	fmt.Fprintln(&b, "  "+initMuted.Render("Authentication/connection/verification may still depend on the provider or agent. Secrets are not stored."))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("No Git operations"))
	fmt.Fprintln(&b, "  "+initOK.Render(plan.GitSafetyStatement))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Runtime conflicts"))
	if len(plan.ExistingArtifacts) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("No conflicting runtime surfaces detected."))
	} else {
		fmt.Fprintln(&b, "  "+initWarn.Render("Conflicts were listed at Init preflight; this slice requires manual cleanup before Setup."))
		for _, path := range plan.ExistingArtifacts {
			fmt.Fprintf(&b, "  - %s\n", path)
		}
	}
	if len(plan.Backups) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("No Atlas-managed backups required for current selection."))
	} else {
		fmt.Fprintf(&b, "  Backup root: %s\n", initplan.BackupPlaceholderDir())
		for _, backup := range plan.Backups {
			fmt.Fprintf(&b, "  - %s\n", backup.Path)
		}
	}
	if len(plan.Replacements) > 0 {
		fmt.Fprintln(&b, "  Replace after backup:")
		for _, repl := range plan.Replacements {
			fmt.Fprintf(&b, "  - %s\n", repl.Path)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Preservations"))
	for _, item := range plan.Preservations {
		fmt.Fprintf(&b, "  - %s\n", item.Statement)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Governance storage"))
	fmt.Fprintln(&b, "  "+plan.GovernanceNote)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("MCP integrations"))
	if len(plan.MCPEntries) == 0 {
		fmt.Fprintln(&b, "  - none")
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
	fmt.Fprintln(&b, "  "+initMuted.Render("MCP: desired state in Atlas config · Atlas-owned projections to selected agents · auth/connection/verification may still depend on provider/agent."))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Git / delivery policy"))
	fmt.Fprintln(&b, "  "+initOK.Render("Init performs no Git operations."))
	for _, line := range plan.DeliveryPolicy {
		fmt.Fprintf(&b, "  - %s\n", line)
	}
	fmt.Fprintln(&b, "  "+initOK.Render(plan.GitSafetyStatement))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Warnings / blockers"))
	for _, warn := range plan.Warnings {
		fmt.Fprintf(&b, "  - %s\n", warn.Message)
	}
	if len(plan.Blockers) == 0 {
		if view.Applied {
			fmt.Fprintln(&b, "  "+initMuted.Render("No blockers."))
		} else {
			fmt.Fprintln(&b, "  "+initMuted.Render("No blockers. Apply writes .atlas/ files and selected runtime gateway projections."))
		}
	} else {
		for _, blocker := range plan.Blockers {
			fmt.Fprintf(&b, "  - %s\n", initWarn.Render(blocker.Message))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
