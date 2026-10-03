package cli

import (
	"fmt"
	"io"

	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

// RunTUI launches the TUI. Overridable in tests.
var RunTUI = tui.Run

// Execute launches the resolved action.
// Only --version writes normal console output.
func Execute(stdout, stderr io.Writer, args []string) error {
	_ = stderr

	action := Resolve(args)
	switch action.Mode {
	case ModeVersion:
		fmt.Fprintln(stdout, version.Version)
		return nil
	case ModeTUI:
		return RunTUI(tui.Options{
			Route:          action.Route,
			UnknownCommand: action.UnknownCommand,
		})
	default:
		return fmt.Errorf("unknown launcher mode")
	}
}
