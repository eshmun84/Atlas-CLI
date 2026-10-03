package cli

import (
	"fmt"
	"io"
)

const doctorUsage = `Run Atlas diagnostics.

Usage:
  atlas doctor

Diagnostics are not implemented yet.
`

func runDoctor(stdout io.Writer, args []string) error {
	if hasHelp(args) {
		fmt.Fprint(stdout, doctorUsage)
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument: %s\n\nRun 'atlas doctor --help' for usage", args[0])
	}

	fmt.Fprintln(stdout, "diagnostics are not implemented yet")
	return nil
}
