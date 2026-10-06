package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

// RunTUI launches the TUI. Overridable in tests.
var RunTUI = tui.Run

// Execute launches the resolved action.
// Only --version writes normal console output.
// Unsupported commands still open the TUI error dialog, then return a non-nil
// error so the process exits with a non-zero status.
func Execute(stdout, stderr io.Writer, args []string) error {
	_ = stderr

	action := Resolve(args)
	switch action.Mode {
	case ModeVersion:
		fmt.Fprintln(stdout, version.Version)
		return nil
	case ModeTUI:
		err := RunTUI(tui.Options{
			Route:          action.Route,
			UnknownCommand: action.UnknownCommand,
		})
		if err != nil {
			return err
		}
		if action.Route == tui.RouteError {
			return UnsupportedCommandError(action.UnknownCommand)
		}
		return nil
	default:
		return fmt.Errorf("unknown launcher mode")
	}
}

// UnsupportedCommandError is returned after the TUI error dialog closes for an
// unsupported or unknown CLI command. Callers should exit non-zero.
func UnsupportedCommandError(command string) error {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return fmt.Errorf("unsupported command")
	}
	return fmt.Errorf("unsupported command: %s", cmd)
}
