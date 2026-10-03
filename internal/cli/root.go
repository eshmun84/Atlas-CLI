package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/version"
)

const rootUsage = `Atlas CLI — governed AI-assisted software engineering.

Usage:
  atlas [command]

Available Commands:
  init        Initialize Atlas in a project
  status      Show Atlas project status
  doctor      Run Atlas diagnostics

Flags:
  -h, --help      Show help
      --version   Print Atlas version

Use "atlas [command] --help" for more information about a command.
`

// Execute parses args and runs the matching Atlas command.
// args should not include the program name.
func Execute(stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(stdout, rootUsage)
		return nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, rootUsage)
		return nil
	case "--version", "-version", "version":
		fmt.Fprintln(stdout, version.Version)
		return nil
	case "init":
		return runInit(stdout, args[1:])
	case "status":
		return runStatus(stdout, args[1:])
	case "doctor":
		return runDoctor(stdout, args[1:])
	default:
		if strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("unknown flag: %s\n\nRun 'atlas --help' for usage", args[0])
		}
		return fmt.Errorf("unknown command: %s\n\nRun 'atlas --help' for usage", args[0])
	}
}
