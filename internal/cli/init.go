package cli

import (
	"fmt"
	"io"
)

const initUsage = `Initialize Atlas in a project.

Usage:
  atlas init

Project initialization is not implemented yet.
`

func runInit(stdout io.Writer, args []string) error {
	if hasHelp(args) {
		fmt.Fprint(stdout, initUsage)
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument: %s\n\nRun 'atlas init --help' for usage", args[0])
	}

	fmt.Fprintln(stdout, "project initialization is not implemented yet")
	return nil
}
