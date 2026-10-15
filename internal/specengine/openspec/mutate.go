package openspec

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ToolsForAdapters maps Atlas adapter IDs to an OpenSpec --tools value.
//
//	cursor only        -> "cursor"
//	opencode only      -> "opencode"
//	cursor + opencode  -> "cursor,opencode"
//	none               -> "none"
func ToolsForAdapters(adapters []string) string {
	var cursor, opencode bool
	for _, a := range adapters {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "cursor":
			cursor = true
		case "opencode":
			opencode = true
		}
	}
	switch {
	case cursor && opencode:
		return "cursor,opencode"
	case cursor:
		return "cursor"
	case opencode:
		return "opencode"
	default:
		return "none"
	}
}

// InitResult is the outcome of a reviewed openspec init invocation.
type InitResult struct {
	Tools    string
	Stdout   string
	Stderr   string
	ExitCode int
}

// RunInit executes `openspec init --tools <tools> --no-animation` in root.
// Caller must have completed preflight + human approval. Not for Status/Doctor.
func RunInit(ctx context.Context, runner Runner, executable, root, tools string) (InitResult, error) {
	if strings.TrimSpace(executable) == "" {
		return InitResult{}, ErrMissing
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return InitResult{}, fmt.Errorf("openspec: init root is required")
	}
	tools = strings.TrimSpace(tools)
	if tools == "" {
		tools = "none"
	}
	if runner == nil {
		runner = ExecRunner{Timeout: mutateTimeout}
	}
	// Ensure mutate timeout even if a probe runner was passed.
	if er, ok := runner.(ExecRunner); ok && er.Timeout < mutateTimeout {
		er.Timeout = mutateTimeout
		runner = er
	}

	out, err := runner.Run(ctx, executable, []string{
		"init",
		"--tools", tools,
		"--no-animation",
	}, root)
	res := InitResult{
		Tools:    tools,
		Stdout:   string(out.Stdout),
		Stderr:   string(out.Stderr),
		ExitCode: out.ExitCode,
	}
	if err != nil {
		return res, fmt.Errorf("openspec init: %w", err)
	}
	st, reason := ClassifyProject(root)
	if st != ProjectReady {
		if reason == "" {
			reason = string(st)
		}
		return res, fmt.Errorf("openspec init: project not ready after init: %s", reason)
	}
	return res, nil
}

// UpdateResult is the outcome of a reviewed openspec update invocation.
type UpdateResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// RunUpdate executes `openspec update` in root after human approval.
func RunUpdate(ctx context.Context, runner Runner, executable, root string) (UpdateResult, error) {
	if strings.TrimSpace(executable) == "" {
		return UpdateResult{}, ErrMissing
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return UpdateResult{}, fmt.Errorf("openspec: update root is required")
	}
	if runner == nil {
		runner = ExecRunner{Timeout: mutateTimeout}
	}
	if er, ok := runner.(ExecRunner); ok && er.Timeout < mutateTimeout {
		er.Timeout = mutateTimeout
		runner = er
	}
	out, err := runner.Run(ctx, executable, []string{"update"}, root)
	res := UpdateResult{
		Stdout:   string(out.Stdout),
		Stderr:   string(out.Stderr),
		ExitCode: out.ExitCode,
	}
	if err != nil {
		return res, fmt.Errorf("openspec update: %w", err)
	}
	return res, nil
}

// ProbeTimeout / MutateTimeout exported for tests and wiring.
func ProbeTimeout() time.Duration  { return probeTimeout }
func MutateTimeout() time.Duration { return mutateTimeout }
