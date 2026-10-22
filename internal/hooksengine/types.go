package hooksengine

import "github.com/eshmun84/Atlas-CLI/internal/hooks"

const (
	SchemaVersion = 1

	CommandDispatch = "dispatch"

	StatusSuccess          = "success"
	StatusAborted          = "aborted"
	StatusDenied           = "denied"
	StatusHooksDisabled    = "hooks_disabled"
	StatusUnsupportedEvent = "unsupported_event"
	StatusInvalidRequest   = "invalid_request"
	StatusInvalidAdapter   = "invalid_adapter"
	StatusInvalidEvent     = "invalid_event"
	StatusProjectMismatch  = "project_mismatch"
	StatusNotInitialized   = "not_initialized"
	StatusUnsafeFilesystem = "unsafe_filesystem"
	StatusInvalidUsage     = "invalid_usage"
	StatusCancelled        = "cancelled"
	StatusInternalError    = "internal_error"

	// MaxRequestBytes bounds untrusted stdin/JSON request bodies.
	MaxRequestBytes = 64 << 10
)

// Request is the versioned canonical dispatch envelope (already translated).
// Native agent payloads are translated by adapters in later slices; this
// engine accepts only the Atlas canonical shape.
type Request struct {
	SchemaVersion int        `json:"schema_version"`
	Adapter       string     `json:"adapter"`
	Event         EventInput `json:"event"`
}

// EventInput is the untrusted canonical event wire form.
type EventInput struct {
	ID         string       `json:"id"`
	Type       string       `json:"type"`
	OccurredAt string       `json:"occurred_at"`
	Payload    PayloadInput `json:"payload"`
}

// PayloadInput mirrors hooks.Payload with JSON tags. Unknown fields are
// rejected by the decoder — opaque extension keys are forbidden.
type PayloadInput struct {
	ProjectID        string `json:"project_id,omitempty"`
	SessionDirectory string `json:"session_directory,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	TaskID           string `json:"task_id,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	ToolCallID       string `json:"tool_call_id,omitempty"`
	Status           string `json:"status,omitempty"`
	Reason           string `json:"reason,omitempty"`
	TaskSummary      string `json:"task_summary,omitempty"`
	TaskSummaryState string `json:"task_summary_state,omitempty"`
}

// Response is the versioned JSON envelope for hooks CLI commands.
type Response struct {
	SchemaVersion int         `json:"schema_version"`
	Command       string      `json:"command"`
	Status        string      `json:"status"`
	ProjectID     string      `json:"project_id,omitempty"`
	Adapter       string      `json:"adapter,omitempty"`
	EventID       string      `json:"event_id,omitempty"`
	EventType     string      `json:"event_type,omitempty"`
	Aborted       bool        `json:"aborted"`
	Count         int         `json:"count"`
	Results       []ResultDTO `json:"results,omitempty"`
	Error         *ErrorBody  `json:"error,omitempty"`
}

// ResultDTO is one sanitized per-action outcome.
type ResultDTO struct {
	ActionID         string `json:"action_id"`
	Outcome          string `json:"outcome"`
	Message          string `json:"message"`
	DenyContinuation bool   `json:"deny_continuation,omitempty"`
}

// ErrorBody is a non-sensitive structured error for agents.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// resultFromHook maps a foundation Result to the public DTO.
func resultFromHook(r hooks.Result) ResultDTO {
	return ResultDTO{
		ActionID:         r.ActionID,
		Outcome:          string(r.Outcome),
		Message:          r.Message,
		DenyContinuation: r.DenyContinuation,
	}
}
