package hooks

import (
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
)

// EventType is a canonical Atlas hook event identity.
// Adapters map native signals onto this closed catalog; unknown strings are invalid.
type EventType string

const (
	EventSessionStart            EventType = "session.start"
	EventSessionResume           EventType = "session.resume"
	EventBeforeContextCompaction EventType = "context.before_compaction"
	EventTaskStart               EventType = "task.start"
	EventTaskComplete            EventType = "task.complete"
	EventBeforeToolExecution     EventType = "tool.before_execution"
	EventAfterToolExecution      EventType = "tool.after_execution"
)

// ExecutionMode controls whether an action may abort the remaining dispatch chain.
type ExecutionMode string

const (
	// ModeNonBlocking runs the action but never aborts the dispatch chain.
	// Failures are recorded; subsequent actions still run.
	ModeNonBlocking ExecutionMode = "non_blocking"
	// ModeBlocking may abort the chain when FailurePolicy is FailAbort or when
	// Result.DenyContinuation is true / Outcome is OutcomeDenied.
	ModeBlocking ExecutionMode = "blocking"
)

// FailurePolicy controls dispatch behavior after an action error.
type FailurePolicy string

const (
	// FailContinue records the error and continues remaining actions.
	FailContinue FailurePolicy = "continue"
	// FailAbort stops remaining actions after a blocking action error.
	// Non-blocking actions always continue regardless of this policy.
	FailAbort FailurePolicy = "abort"
)

// Outcome is the classified per-action result status.
type Outcome string

const (
	OutcomeOK        Outcome = "ok"
	OutcomeSkipped   Outcome = "skipped"
	OutcomeFailed    Outcome = "failed"
	OutcomeTimeout   Outcome = "timeout"
	OutcomeCancelled Outcome = "cancelled"
	OutcomeDenied    Outcome = "denied"
)

// TaskSummaryState explicitly records whether a short task summary is available.
// Adapters must not invent summaries. Use TaskSummaryAbsent when the runtime
// cannot supply a safe short description.
type TaskSummaryState string

const (
	// TaskSummaryUnspecified is invalid on task.start / task.complete.
	TaskSummaryUnspecified TaskSummaryState = ""
	// TaskSummaryPresent means TaskSummary carries a bounded short description.
	TaskSummaryPresent TaskSummaryState = "present"
	// TaskSummaryAbsent means no summary is available (explicit missing context).
	TaskSummaryAbsent TaskSummaryState = "absent"
)

// DefaultActionTimeout is applied when a registration omits Timeout.
const DefaultActionTimeout = 5 * time.Second

// Payload holds bounded, agent-neutral event fields.
// Adapters must not place secrets, full transcripts, or filesystem trees here.
// Opaque extension keys are forbidden in v1 — extend typed fields deliberately.
//
// Fields are event-contract data only. The Hooks foundation does not persist
// Payload (including TaskSummary) to Memory, disk, or any other store.
type Payload struct {
	ProjectID string
	// SessionDirectory is the OpenCode session directory (OpenCode 2.0.20
	// Location.Ref or session.get location.directory). Empty for Cursor.
	// OpenCode maintenance requires it to match the resolved project root.
	SessionDirectory string
	SessionID        string
	TaskID           string
	ToolName         string
	ToolCallID       string
	// Status is optional outcome vocabulary for completion/after-tool events
	// (e.g. success|failure|cancelled). Empty when not applicable.
	Status string
	// Reason is optional human-safe context (e.g. compaction trigger).
	Reason string
	// TaskSummary is an optional short task description for task.* events.
	// It must never contain complete prompts or conversation transcripts.
	// Not persisted by this package.
	TaskSummary string
	// TaskSummaryState must be present or absent on task.* events.
	TaskSummaryState TaskSummaryState
}

// Event is one canonical occurrence delivered to registered Actions.
type Event struct {
	// ID uniquely identifies this occurrence (adapter- or Atlas-generated).
	ID   string
	Type EventType
	// OccurredAt is the event timestamp (UTC recommended).
	OccurredAt time.Time
	// Adapter is the translating runtime adapter. Empty only for synthetic tests.
	Adapter adapters.ID
	Payload Payload
}

// Result is the structured public outcome of one Action invocation.
//
// After Dispatch normalization:
//   - ActionID is always the registered action identity (Run-supplied IDs ignored).
//   - Outcome is always a canonical enum value (unknown → failed).
//   - Message is always a fixed predefined public string (never Action.Run text).
type Result struct {
	ActionID string
	Outcome  Outcome
	Message  string
	// DenyContinuation aborts the remaining dispatch chain when true.
	// Honored only for ModeBlocking actions with OutcomeOK or OutcomeDenied.
	// Zero value (false) allows continuation.
	DenyContinuation bool
}
