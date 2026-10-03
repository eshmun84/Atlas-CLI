package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

const initUsage = `Initialize Atlas in a project.

Usage:
  atlas init
  atlas init --dry-run

Prints a read-only initialization plan. No files are created in this slice.
`

// Overridable for tests.
var (
	initGetwd    = os.Getwd
	initDiscover = workspace.Discover
)

func runInit(stdout io.Writer, args []string) error {
	if hasHelp(args) {
		fmt.Fprint(stdout, initUsage)
		return nil
	}
	if err := validateInitArgs(args); err != nil {
		return err
	}

	root, err := initGetwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	result, err := initDiscover(root)
	if err != nil {
		return fmt.Errorf("init discovery failed: %w", err)
	}

	plan, err := initplan.Build(root, result)
	if err != nil {
		return fmt.Errorf("build init plan: %w", err)
	}

	initplan.WriteReport(stdout, plan)
	return nil
}

func validateInitArgs(args []string) error {
	for _, arg := range args {
		if arg == "--dry-run" {
			continue
		}
		return fmt.Errorf("unexpected argument: %s\n\nRun 'atlas init --help' for usage", arg)
	}
	return nil
}
