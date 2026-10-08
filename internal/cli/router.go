package cli

import (
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/tui"
)

// Mode selects CLI launcher behavior.
type Mode int

const (
	// ModeVersion prints the version to stdout.
	ModeVersion Mode = iota
	// ModeTUI launches the Atlas TUI.
	ModeTUI
)

// Action is the resolved launcher action for a set of args.
type Action struct {
	Mode           Mode
	Route          tui.Route
	UnknownCommand string
	CodeIntelFull  bool
}

// Resolve maps CLI args to a launcher action. It does not render reports.
func Resolve(args []string) Action {
	if len(args) == 0 {
		return Action{Mode: ModeTUI, Route: tui.DefaultRoute}
	}

	switch args[0] {
	case "--version", "-version", "version":
		return Action{Mode: ModeVersion}
	case "help", "--help", "-h":
		return Action{Mode: ModeTUI, Route: tui.RouteHelp}
	case "init":
		if initArgsOK(args[1:]) {
			return Action{Mode: ModeTUI, Route: tui.RouteInitPlan}
		}
		return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: joinCommand(args)}
	case "status":
		if len(args) == 1 {
			return Action{Mode: ModeTUI, Route: tui.RouteStatus}
		}
		return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: joinCommand(args)}
	case "doctor":
		if len(args) == 1 {
			return Action{Mode: ModeTUI, Route: tui.RouteDoctor}
		}
		return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: joinCommand(args)}
	case "codeintel":
		return resolveCodeIntel(args[1:])
	case "mcp":
		return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: joinCommand(args)}
	default:
		return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: joinCommand(args)}
	}
}

func resolveCodeIntel(args []string) Action {
	if len(args) == 0 {
		return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: "codeintel"}
	}
	if args[0] != "refresh" {
		return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: joinCommand(append([]string{"codeintel"}, args...))}
	}
	full := false
	for _, arg := range args[1:] {
		switch arg {
		case "--full":
			full = true
		default:
			return Action{Mode: ModeTUI, Route: tui.RouteError, UnknownCommand: joinCommand(append([]string{"codeintel", "refresh"}, args[1:]...))}
		}
	}
	return Action{Mode: ModeTUI, Route: tui.RouteCodeIntelRefresh, CodeIntelFull: full}
}

func initArgsOK(args []string) bool {
	for _, arg := range args {
		if arg == "--dry-run" {
			continue
		}
		return false
	}
	return true
}

func joinCommand(args []string) string {
	return strings.Join(args, " ")
}
