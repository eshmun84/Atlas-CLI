package initplan

import (
	"fmt"
	"io"
)

// WriteReport prints a human-readable dry-run init plan.
func WriteReport(w io.Writer, plan Plan) {
	fmt.Fprintln(w, "Atlas Init Plan")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Project:")
	fmt.Fprintf(w, "  Name: %s\n", plan.ProjectName)
	fmt.Fprintf(w, "  Mode: %s\n", plan.ProjectMode)
	fmt.Fprintf(w, "  Root: %s\n", plan.RootPath)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Planned Files:")
	if len(plan.Steps) == 0 {
		fmt.Fprintln(w, "  none")
	} else {
		for _, step := range plan.Steps {
			fmt.Fprintf(w, "  %s %s — %s\n", step.Status, step.Path, step.Reason)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Notes:")
	if len(plan.Warnings) == 0 {
		fmt.Fprintln(w, "  none")
	} else {
		for _, note := range plan.Warnings {
			fmt.Fprintf(w, "  - %s\n", note)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Result:")
	fmt.Fprintln(w, "  ready to initialize")
}
