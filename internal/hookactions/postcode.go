package hookactions

import (
	"context"
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// ActionIDPostCode is the stable identity for post-code maintenance admission.
const ActionIDPostCode = "context.postcode.maintain"

// PostCodeAction admits debounced Home maintenance after a possible code mutation.
// It never waits for CE/CI — only bumps generations and ensures a scheduler.
//
// An allowed event is a possible mutation signal, not confirmed content change.
// CE/CI fingerprints remain the freshness authority.
type PostCodeAction struct {
	// Resolve supplies workspace identity. nil → context / DefaultResolve.
	Resolve ResolveFunc
}

var _ hooks.Action = (*PostCodeAction)(nil)

func (a *PostCodeAction) ID() string { return ActionIDPostCode }

func (a *PostCodeAction) Events() []hooks.EventType {
	return []hooks.EventType{hooks.EventAfterToolExecution}
}

func (a *PostCodeAction) Mode() hooks.ExecutionMode {
	return hooks.ModeNonBlocking
}

func (a *PostCodeAction) Run(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return hooks.Result{Outcome: hooks.OutcomeCancelled}, err
	}
	if !TriggersAreEnabled() {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	if ev.Type != hooks.EventAfterToolExecution {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	if !toolStatusAllowsPostCode(ev.Payload.Status) {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	tool := strings.TrimSpace(ev.Payload.ToolName)
	if tool == "" {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	if !ToolAllowsPostCode(ev.Adapter, tool) {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}

	ref, err := resolveForAdmission(ctx, a.Resolve)
	if err != nil {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	if payloadID := strings.TrimSpace(ev.Payload.ProjectID); payloadID != "" && payloadID != ref.ProjectID {
		return hooks.Result{Outcome: hooks.OutcomeFailed}, fmt.Errorf("%w: id mismatch", errAdmitProject)
	}
	if err := openCodeSessionAllowed(ev.Adapter, ref.Root, ev.Payload.SessionDirectory); err != nil {
		return hooks.Result{Outcome: hooks.OutcomeFailed}, err
	}

	if err := BumpPendingAndEnsureScheduler(ref); err != nil {
		// Fail-open toward the agent (non_blocking + fail_continue); fail-closed
		// for Atlas pending mutations is expressed as OutcomeFailed.
		return hooks.Result{Outcome: hooks.OutcomeFailed}, err
	}
	return hooks.Result{Outcome: hooks.OutcomeOK}, nil
}

// toolStatusAllowsPostCode accepts success / empty. Rejects error/failed/denied/cancelled.
func toolStatusAllowsPostCode(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "success", "ok", "completed":
		return true
	default:
		return false
	}
}

// ToolAllowsPostCode reports whether adapter+tool_name is on the closed allowlist.
// Exact match only — no regex, no argv inspection.
func ToolAllowsPostCode(adapter adapters.ID, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	switch adapter {
	case adapters.Cursor:
		return cursorPostCodeTools[toolName]
	case adapters.OpenCode:
		return openCodePostCodeTools[toolName]
	default:
		return false
	}
}

// Cursor postToolUse tool types (Cursor docs + Atlas native tests):
// Write (create/overwrite; Claude Edit maps to Write), Delete.
// Explicitly excluded: Shell, Read, Grep, Task, MCP:*, WebFetch, WebSearch.
var cursorPostCodeTools = map[string]bool{
	"Write":  true,
	"Delete": true,
}

// OpenCode built-in mutating tools (opencode.ai/docs/tools):
// write, edit, apply_patch. Excluded: bash, read, grep, glob, webfetch, …
var openCodePostCodeTools = map[string]bool{
	"write":       true,
	"edit":        true,
	"apply_patch": true,
}

// CursorPostCodeAllowlist returns a copy of Cursor tool names (tests/docs).
func CursorPostCodeAllowlist() []string {
	return []string{"Write", "Delete"}
}

// OpenCodePostCodeAllowlist returns a copy of OpenCode tool names (tests/docs).
func OpenCodePostCodeAllowlist() []string {
	return []string{"write", "edit", "apply_patch"}
}
