package hooks

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxIDLen          = 128
	maxFieldLen       = 256
	maxReasonLen      = 512
	maxStatusLen      = 64
	maxTaskSummaryLen = 256
)

// KnownEventTypes returns the closed canonical catalog in stable order.
func KnownEventTypes() []EventType {
	return []EventType{
		EventSessionStart,
		EventSessionResume,
		EventBeforeContextCompaction,
		EventTaskStart,
		EventTaskComplete,
		EventBeforeToolExecution,
		EventAfterToolExecution,
	}
}

// IsKnownEventType reports whether t is in the canonical catalog.
func IsKnownEventType(t EventType) bool {
	switch t {
	case EventSessionStart,
		EventSessionResume,
		EventBeforeContextCompaction,
		EventTaskStart,
		EventTaskComplete,
		EventBeforeToolExecution,
		EventAfterToolExecution:
		return true
	default:
		return false
	}
}

// ValidateEventType rejects unknown event identities.
func ValidateEventType(t EventType) error {
	if !IsKnownEventType(t) {
		return fmt.Errorf("%w: %q", ErrUnknownEventType, t)
	}
	return nil
}

// ValidateEvent checks identity, catalog membership, and payload boundaries.
func ValidateEvent(ev Event) error {
	if err := validateToken("event id", ev.ID, maxIDLen, true); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	if err := ValidateEventType(ev.Type); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}
	if ev.OccurredAt.IsZero() {
		return fmt.Errorf("%w: occurred_at is required", ErrInvalidEvent)
	}
	if err := validatePayload(ev.Type, ev.Payload); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	return nil
}

func validatePayload(t EventType, p Payload) error {
	if err := validateToken("project_id", p.ProjectID, maxIDLen, false); err != nil {
		return err
	}
	if err := validateToken("session_id", p.SessionID, maxIDLen, false); err != nil {
		return err
	}
	if err := validateToken("task_id", p.TaskID, maxIDLen, false); err != nil {
		return err
	}
	if err := validateToken("tool_name", p.ToolName, maxFieldLen, false); err != nil {
		return err
	}
	if err := validateToken("tool_call_id", p.ToolCallID, maxIDLen, false); err != nil {
		return err
	}
	if err := validateToken("status", p.Status, maxStatusLen, false); err != nil {
		return err
	}
	if err := validateReason(p.Reason); err != nil {
		return err
	}
	if err := validateTaskSummary(t, p); err != nil {
		return err
	}

	switch t {
	case EventBeforeToolExecution, EventAfterToolExecution:
		if strings.TrimSpace(p.ToolName) == "" {
			return fmt.Errorf("tool_name is required for %s", t)
		}
	case EventTaskStart, EventTaskComplete:
		if strings.TrimSpace(p.TaskID) == "" {
			return fmt.Errorf("task_id is required for %s", t)
		}
	case EventSessionStart, EventSessionResume:
		if strings.TrimSpace(p.SessionID) == "" {
			return fmt.Errorf("session_id is required for %s", t)
		}
	}
	return nil
}

func validateTaskSummary(t EventType, p Payload) error {
	isTask := t == EventTaskStart || t == EventTaskComplete
	if !isTask {
		if p.TaskSummaryState != TaskSummaryUnspecified {
			return fmt.Errorf("task_summary_state is only valid on task events")
		}
		if strings.TrimSpace(p.TaskSummary) != "" {
			return fmt.Errorf("task_summary is only valid on task events")
		}
		return nil
	}

	switch p.TaskSummaryState {
	case TaskSummaryAbsent:
		if strings.TrimSpace(p.TaskSummary) != "" {
			return fmt.Errorf("task_summary must be empty when state is absent")
		}
		return nil
	case TaskSummaryPresent:
		return validateTaskSummaryText(p.TaskSummary)
	default:
		return fmt.Errorf("task_summary_state must be present or absent for %s", t)
	}
}

func validateTaskSummaryText(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return fmt.Errorf("task_summary is required when state is present")
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("task_summary is not valid UTF-8")
	}
	if utf8.RuneCountInString(s) > maxTaskSummaryLen {
		return fmt.Errorf("task_summary too long")
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return fmt.Errorf("task_summary contains control characters")
		}
		if unicode.IsSpace(r) && r != ' ' {
			return fmt.Errorf("task_summary contains control whitespace")
		}
	}
	return nil
}

func validateReason(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("reason is not valid UTF-8")
	}
	if utf8.RuneCountInString(s) > maxReasonLen {
		return fmt.Errorf("reason too long")
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return fmt.Errorf("reason contains control characters")
		}
	}
	return nil
}

// validateToken enforces length, control-char, and path-element safety.
// required=true rejects empty after trim.
func validateToken(name, raw string, max int, required bool) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	if len(s) > max {
		return fmt.Errorf("%s too long", name)
	}
	if strings.Contains(s, "..") || strings.Contains(s, `\`) {
		return fmt.Errorf("%s must not contain path elements", name)
	}
	if strings.HasPrefix(s, "/") {
		return fmt.Errorf("%s must not be absolute", name)
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return fmt.Errorf("%s contains control characters", name)
		}
		if unicode.IsSpace(r) && r != ' ' {
			return fmt.Errorf("%s contains control whitespace", name)
		}
	}
	return nil
}

// ValidateRegistration checks action registration contracts before insert.
func ValidateRegistration(reg Registration) error {
	if reg.Action == nil {
		return fmt.Errorf("%w: action is nil", ErrInvalidAction)
	}
	id := strings.TrimSpace(reg.Action.ID())
	if err := validateToken("action id", id, maxIDLen, true); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	switch reg.Action.Mode() {
	case ModeBlocking, ModeNonBlocking:
	default:
		return fmt.Errorf("%w: unknown execution mode %q", ErrInvalidAction, reg.Action.Mode())
	}
	switch reg.FailurePolicy {
	case FailContinue, FailAbort, "":
	default:
		return fmt.Errorf("%w: unknown failure policy %q", ErrInvalidAction, reg.FailurePolicy)
	}
	events := reg.Action.Events()
	if len(events) == 0 {
		return fmt.Errorf("%w: action %q subscribes to no events", ErrInvalidAction, id)
	}
	seen := map[EventType]struct{}{}
	for _, t := range events {
		if err := ValidateEventType(t); err != nil {
			return fmt.Errorf("%w: action %q: %v", ErrInvalidAction, id, err)
		}
		if _, ok := seen[t]; ok {
			return fmt.Errorf("%w: action %q duplicates event %q", ErrInvalidAction, id, t)
		}
		seen[t] = struct{}{}
	}
	if reg.Timeout < 0 {
		return fmt.Errorf("%w: action %q timeout must not be negative", ErrInvalidAction, id)
	}
	return nil
}
