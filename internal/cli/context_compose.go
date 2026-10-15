package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
)

// ContextComposeCommand is a resolved non-TUI context composition action.
// Internal integration transport — not a promoted developer-facing workflow.
type ContextComposeCommand struct {
	Task   string
	Budget int
	Limit  int
	JSON   bool
	// Sources is an optional explicit compose source list (memory, context_economy, codegraph).
	// Nil/omitted preserves composeapp default sources. When SourcesSet, empty is invalid.
	Sources    []string
	SourcesSet bool
}

func resolveContext(args []string) Action {
	if len(args) == 0 {
		return Action{
			Mode:           ModeContextCompose,
			ContextCompose: ContextComposeCommand{},
			UnknownCommand: "context",
		}
	}
	if args[0] != "compose" {
		return Action{
			Mode:           ModeContextCompose,
			ContextCompose: ContextComposeCommand{},
			UnknownCommand: joinCommand(append([]string{"context"}, args...)),
		}
	}

	cmd := ContextComposeCommand{}
	rest := args[1:]
	positional := make([]string, 0, 1)
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--json":
			cmd.JSON = true
		case arg == "--budget":
			if i+1 >= len(rest) {
				return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: "context compose --budget"}
			}
			n, err := strconv.Atoi(rest[i+1])
			if err != nil || n < 0 {
				return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: "context compose --budget"}
			}
			cmd.Budget = n
			i++
		case strings.HasPrefix(arg, "--budget="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--budget="))
			if err != nil || n < 0 {
				return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: "context compose --budget"}
			}
			cmd.Budget = n
		case arg == "--limit":
			if i+1 >= len(rest) {
				return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: "context compose --limit"}
			}
			n, err := strconv.Atoi(rest[i+1])
			if err != nil || n < 0 {
				return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: "context compose --limit"}
			}
			cmd.Limit = n
			i++
		case strings.HasPrefix(arg, "--limit="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--limit="))
			if err != nil || n < 0 {
				return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: "context compose --limit"}
			}
			cmd.Limit = n
		case arg == "--sources":
			if i+1 >= len(rest) {
				return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: "context compose --sources"}
			}
			cmd.SourcesSet = true
			cmd.Sources = parseComposeSourcesCSV(rest[i+1])
			i++
		case strings.HasPrefix(arg, "--sources="):
			cmd.SourcesSet = true
			cmd.Sources = parseComposeSourcesCSV(strings.TrimPrefix(arg, "--sources="))
		case strings.HasPrefix(arg, "-"):
			return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: joinCommand(append([]string{"context", "compose"}, rest...))}
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 1 {
		cmd.Task = positional[0]
	} else if len(positional) > 1 {
		return Action{Mode: ModeContextCompose, ContextCompose: cmd, UnknownCommand: joinCommand(append([]string{"context", "compose"}, rest...))}
	}
	return Action{Mode: ModeContextCompose, ContextCompose: cmd}
}

// knownComposeSources is the closed set accepted by --sources (contextcompose IDs).
var knownComposeSources = map[string]bool{
	contextcompose.SourceMemory:         true,
	contextcompose.SourceContextEconomy: true,
	contextcompose.SourceCodeGraph:      true,
}

func parseComposeSourcesCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func validateComposeSources(sources []string) (string, bool) {
	if len(sources) == 0 {
		return "empty --sources is invalid", false
	}
	for _, s := range sources {
		if !knownComposeSources[s] {
			return "unknown compose source: " + s, false
		}
	}
	return "", true
}

// ComposeService builds the composition result. Overridable in tests.
var ComposeService = func(in composeapp.Input) composeapp.Result {
	return composeapp.Compose(in)
}

func executeContextCompose(stdout io.Writer, cmd ContextComposeCommand, unknown string) error {
	var res composeapp.Result
	switch {
	case unknown != "":
		res = composeUsage("invalid context compose command usage")
	case !cmd.JSON:
		res = composeUsage("--json is required")
	case strings.TrimSpace(cmd.Task) == "":
		res = composeUsage("usage: atlas context compose \"<task>\" --json [--sources memory,context_economy,codegraph]")
	case cmd.SourcesSet:
		if msg, ok := validateComposeSources(cmd.Sources); !ok {
			res = composeUsage(msg)
		} else {
			res = ComposeService(composeapp.Input{
				Task:        cmd.Task,
				BudgetChars: cmd.Budget,
				MaxItems:    cmd.Limit,
				Sources:     append([]string{}, cmd.Sources...),
			})
		}
	default:
		res = ComposeService(composeapp.Input{
			Task:        cmd.Task,
			BudgetChars: cmd.Budget,
			MaxItems:    cmd.Limit,
		})
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(res); err != nil {
		return fmt.Errorf("context compose: encode json: %w", err)
	}
	if composeapp.ShouldExitNonZero(res) {
		return composeapp.ExitError{Status: res.Status}
	}
	return nil
}

func composeUsage(message string) composeapp.Result {
	return composeapp.Result{
		Command: composeapp.CommandCompose,
		Response: contextcompose.Response{
			SchemaVersion: contextcompose.SchemaVersion,
			Status:        contextcompose.StatusInvalidRequest,
			Count:         0,
			Error: &contextcompose.ErrorBody{
				Code:    contextcompose.StatusInvalidRequest,
				Message: message,
			},
		},
	}
}
