package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

func init() {
	ensureCursorHooksBridge()
}

// HooksCommand is a resolved non-TUI hooks CLI action.
type HooksCommand struct {
	Name string // dispatch | cursor
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
	case "dispatch", "cursor":
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

// HooksInput returns the request body reader for dispatch/cursor. Overridable in tests.
var HooksInput = func() io.Reader {
	return os.Stdin
}

func executeHooks(stdout io.Writer, cmd HooksCommand, unknown string) error {
	switch {
	case cmd.Name == "cursor":
		return executeHooksCursor(stdout, cmd, unknown)
	default:
		return executeHooksDispatch(stdout, cmd, unknown)
	}
}

func executeHooksDispatch(stdout io.Writer, cmd HooksCommand, unknown string) error {
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

func executeHooksCursor(stdout io.Writer, cmd HooksCommand, unknown string) error {
	if unknown != "" || !cmd.JSON || cmd.Name != "cursor" {
		resp := hooksUsage("cursor", "usage: atlas hooks cursor --json < native.json")
		if unknown != "" {
			resp = hooksUsage("cursor", "invalid hooks command usage")
		} else if !cmd.JSON {
			resp = hooksUsage("cursor", "--json is required")
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("hooks: encode json: %w", err)
		}
		return hooksengine.ExitError{Status: resp.Status}
	}
	ensureCursorHooksBridge()
	dispatch := func(ctx context.Context, req adaptercursor.CanonicalDispatch) string {
		er := hooksengine.Request{
			SchemaVersion: req.SchemaVersion,
			Adapter:       string(req.Adapter),
			Event: hooksengine.EventInput{
				ID:         req.Event.ID,
				Type:       string(req.Event.Type),
				OccurredAt: req.Event.OccurredAt.UTC().Format(time.RFC3339Nano),
				Payload: hooksengine.PayloadInput{
					ProjectID:        req.Event.Payload.ProjectID,
					SessionID:        req.Event.Payload.SessionID,
					TaskID:           req.Event.Payload.TaskID,
					ToolName:         req.Event.Payload.ToolName,
					ToolCallID:       req.Event.Payload.ToolCallID,
					Status:           req.Event.Payload.Status,
					Reason:           req.Event.Payload.Reason,
					TaskSummary:      req.Event.Payload.TaskSummary,
					TaskSummaryState: string(req.Event.Payload.TaskSummaryState),
				},
			},
		}
		return HooksEngine().Dispatch(ctx, er).Status
	}
	if err := adaptercursor.RunNative(context.Background(), dispatch, HooksInput(), stdout); err != nil {
		return fmt.Errorf("hooks cursor: %w", err)
	}
	// Native Cursor contract: deny via JSON body; process exit stays 0.
	return nil
}

func ensureCursorHooksBridge() {
	hooksengine.RegisterBridge(adaptercursor.NewHooksBridge())
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
