package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

// HooksCommand is a resolved non-TUI hooks CLI action.
type HooksCommand struct {
	Name string // dispatch
	JSON bool
}

func resolveHooks(args []string) Action {
	if len(args) == 0 {
		return Action{
			Mode:           ModeHooks,
			Hooks:          HooksCommand{},
			UnknownCommand: "hooks",
		}
	}
	cmd := HooksCommand{Name: args[0]}
	rest := args[1:]

	switch cmd.Name {
	case "dispatch":
	default:
		return Action{
			Mode:           ModeHooks,
			Hooks:          HooksCommand{},
			UnknownCommand: joinCommand(append([]string{"hooks"}, args...)),
		}
	}

	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--json":
			cmd.JSON = true
		case strings.HasPrefix(arg, "-"):
			return Action{Mode: ModeHooks, Hooks: cmd, UnknownCommand: joinCommand(append([]string{"hooks"}, args...))}
		default:
			return Action{Mode: ModeHooks, Hooks: cmd, UnknownCommand: joinCommand(append([]string{"hooks"}, args...))}
		}
	}
	return Action{Mode: ModeHooks, Hooks: cmd}
}

// HooksEngine builds the hooks execution engine. Overridable in tests.
var HooksEngine = func() hooksengine.Engine {
	return hooksengine.New()
}

// HooksInput returns the request body reader for dispatch. Overridable in tests.
var HooksInput = func() io.Reader {
	return os.Stdin
}

func executeHooks(stdout io.Writer, cmd HooksCommand, unknown string) error {
	var resp hooksengine.Response
	switch {
	case cmd.Name == "":
		resp = hooksUsage("hooks", "usage: atlas hooks dispatch --json < request.json")
	case unknown != "":
		name := cmd.Name
		if name == "" {
			name = "hooks"
		}
		resp = hooksUsage(name, "invalid hooks command usage")
	case !cmd.JSON:
		resp = hooksUsage(cmd.Name, "--json is required")
	case cmd.Name != "dispatch":
		resp = hooksUsage("hooks", "usage: atlas hooks dispatch --json < request.json")
	default:
		req, err := hooksengine.DecodeRequest(HooksInput())
		if err != nil {
			status, msg := hooksClassifyDecode(err)
			resp = hooksengine.Response{
				SchemaVersion: hooksengine.SchemaVersion,
				Command:       hooksengine.CommandDispatch,
				Status:        status,
				Error:         &hooksengine.ErrorBody{Code: status, Message: msg},
			}
		} else {
			resp = HooksEngine().Dispatch(nil, req)
		}
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("hooks: encode json: %w", err)
	}
	if hooksengine.ShouldExitNonZero(resp.Status) {
		return hooksengine.ExitError{Status: resp.Status}
	}
	return nil
}

func hooksUsage(command, message string) hooksengine.Response {
	return hooksengine.Response{
		SchemaVersion: hooksengine.SchemaVersion,
		Command:       command,
		Status:        hooksengine.StatusInvalidUsage,
		Count:         0,
		Error: &hooksengine.ErrorBody{
			Code:    hooksengine.StatusInvalidUsage,
			Message: message,
		},
	}
}

func hooksClassifyDecode(err error) (status, message string) {
	// Decode errors are always invalid_request; keep messages generic.
	_ = err
	return hooksengine.StatusInvalidRequest, "invalid hooks request"
}
