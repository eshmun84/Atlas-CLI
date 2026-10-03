package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

var (
	initHead   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	initCreate = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	initSkip   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	initFuture = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	initNote   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// InitPlan renders the dry-run initialization plan screen.
func InitPlan(plan initplan.Plan) string {
	var b strings.Builder
	fmt.Fprintln(&b, initHead.Render("Atlas Init Plan"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, initHead.Render("Project"))
	fmt.Fprintf(&b, "  Name: %s\n", plan.ProjectName)
	fmt.Fprintf(&b, "  Mode: %s\n", plan.ProjectMode)
	fmt.Fprintf(&b, "  Root: %s\n\n", plan.RootPath)

	fmt.Fprintln(&b, initHead.Render("Planned Files"))
	if len(plan.Steps) == 0 {
		fmt.Fprintln(&b, "  none")
	} else {
		for _, step := range plan.Steps {
			style := initCreate
			switch step.Status {
			case initplan.StatusSkipExisting:
				style = initSkip
			case initplan.StatusFuture, initplan.StatusWarning:
				style = initFuture
			}
			fmt.Fprintf(&b, "  %s %s — %s\n", style.Render(step.Status), step.Path, step.Reason)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, initHead.Render("Notes"))
	if len(plan.Warnings) == 0 {
		fmt.Fprintln(&b, "  "+initNote.Render("This is a dry-run plan."))
		fmt.Fprintln(&b, "  "+initNote.Render("No files were created."))
	} else {
		for _, note := range plan.Warnings {
			fmt.Fprintf(&b, "  - %s\n", initNote.Render(note))
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, initHead.Render("Result"))
	fmt.Fprintln(&b, "  ready to initialize")
	return strings.TrimRight(b.String(), "\n")
}
