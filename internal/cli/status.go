package cli

import (
	"fmt"
	"io"
)

const statusUsage = `Show Atlas project status.

Usage:
  atlas status

Status inspection is not implemented yet.
`

func runStatus(stdout io.Writer, args []string) error {
	if hasHelp(args) {
		fmt.Fprint(stdout, statusUsage)
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument: %s\n\nRun 'atlas status --help' for usage", args[0])
	}

	fmt.Fprintln(stdout, "status inspection is not implemented yet")
	return nil
}
