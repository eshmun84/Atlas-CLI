package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

const doctorUsage = `Run Atlas diagnostics.

Usage:
  atlas doctor

Runs read-only workspace diagnostics and prints PASS/WARN/FAIL checks.
Does not create or modify project files.
`

// Overridable for tests.
var (
	doctorGetwd    = os.Getwd
	doctorDiscover = workspace.Discover
)

func runDoctor(stdout io.Writer, args []string) error {
	if hasHelp(args) {
		fmt.Fprint(stdout, doctorUsage)
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument: %s\n\nRun 'atlas doctor --help' for usage", args[0])
	}

	root, err := doctorGetwd()
	if err != nil {
		report := doctor.EvaluateWorkingDirectoryError(err)
		doctor.WriteReport(stdout, report)
		return doctor.ErrUnhealthy
	}

	result, err := doctorDiscover(root)
	if err != nil {
		report := doctor.EvaluateDiscoveryError(err)
		doctor.WriteReport(stdout, report)
		return doctor.ErrUnhealthy
	}

	report := doctor.Evaluate(result)
	doctor.WriteReport(stdout, report)
	if report.Failed() {
		return doctor.ErrUnhealthy
	}
	return nil
}
