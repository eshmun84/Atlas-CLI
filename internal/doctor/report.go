package doctor

import (
	"fmt"
	"io"
)

// WriteReport prints a human-readable doctor report.
func WriteReport(w io.Writer, report Report) {
	fmt.Fprintln(w, "Atlas Doctor")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Checks:")
	if len(report.Checks) == 0 {
		fmt.Fprintln(w, "  none")
	} else {
		for _, check := range report.Checks {
			fmt.Fprintf(w, "  %s\n", check.String())
		}
	}
	fmt.Fprintln(w)

	passed, warnings, failed := report.Counts()
	fmt.Fprintln(w, "Summary:")
	fmt.Fprintf(w, "  Passed: %d\n", passed)
	fmt.Fprintf(w, "  Warnings: %d\n", warnings)
	fmt.Fprintf(w, "  Failed: %d\n", failed)
	fmt.Fprintf(w, "  Result: %s\n", report.ResultLabel())
}
