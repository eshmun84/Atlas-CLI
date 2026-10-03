package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

const statusUsage = `Show Atlas project status.

Usage:
  atlas status

Inspects the current workspace using read-only discovery.
Does not create or modify project files.
`

// Overridable for tests.
var (
	statusGetwd    = os.Getwd
	statusDiscover = workspace.Discover
)

func runStatus(stdout io.Writer, args []string) error {
	if hasHelp(args) {
		fmt.Fprint(stdout, statusUsage)
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument: %s\n\nRun 'atlas status --help' for usage", args[0])
	}

	root, err := statusGetwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	result, err := statusDiscover(root)
	if err != nil {
		return fmt.Errorf("status discovery failed: %w", err)
	}

	writeStatusReport(stdout, result)
	return nil
}
