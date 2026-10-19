package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

// RunTUI launches the TUI. Overridable in tests.
var RunTUI = tui.Run

// Execute launches the resolved action.
// --version, memory --json, hooks --json, context compose --json, and
// capability plan --json write console/JSON output without opening the TUI.
// Unsupported commands still open the TUI error dialog, then return a non-nil
// error so the process exits non-zero.
//
// When ATLAS_INTERNAL_MAINTAIN=1, Execute runs the detached session-maintenance
// worker and returns without public command routing (not a public CLI verb).
func Execute(stdout, stderr io.Writer, args []string) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if handled, err := hookactions.MaybeRunInternalMaintain(stdout, stderr); handled {
		return err
	}

	action := Resolve(args)
	switch action.Mode {
	case ModeVersion:
		fmt.Fprintln(stdout, version.Version)
		return nil
	case ModeMemory:
		return executeMemory(stdout, action.Memory, action.UnknownCommand)
	case ModeHooks:
		return executeHooks(stdout, action.Hooks, action.UnknownCommand)
	case ModeContextCompose:
		return executeContextCompose(stdout, action.ContextCompose, action.UnknownCommand)
	case ModeCapability:
		return executeCapability(stdout, action.Capability, action.UnknownCommand)
	case ModeTUI:
		err := RunTUI(tui.Options{
			Route:          action.Route,
			UnknownCommand: action.UnknownCommand,
			CodeIntelFull:  action.CodeIntelFull,
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
