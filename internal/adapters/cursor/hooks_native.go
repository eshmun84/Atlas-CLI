package cursor

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

const (
	NativeSessionStart       = "sessionStart"
	NativeBeforeSubmitPrompt = "beforeSubmitPrompt"
	NativePreToolUse         = "preToolUse"
	NativePostToolUse        = "postToolUse"
	NativePreCompact         = "preCompact"

	// MaxNativeBytes bounds untrusted Cursor stdin.
	MaxNativeBytes = 64 << 10

	publicDenyMessage = "blocked by atlas hooks"

	// StatusDenied matches hooksengine.StatusDenied without importing that package
	// (avoids config→cursor→hooksengine→config cycles).
	StatusDenied = "denied"
)

// NativePayload is a bounded decode of Cursor hook stdin JSON.
// Unknown fields are ignored so Cursor can evolve; only mapped fields are used.
// Prompt/transcript/tool bodies are never copied into canonical Payload.
type NativePayload struct {
	HookEventName  string          `json:"hook_event_name"`
	ConversationID string          `json:"conversation_id"`
	GenerationID   string          `json:"generation_id"`
	SessionID      string          `json:"session_id"`
	CursorVersion  string          `json:"cursor_version"`
	WorkspaceRoots []string        `json:"workspace_roots"`
	ToolName       string          `json:"tool_name"`
	ToolUseID      string          `json:"tool_use_id"`
	Trigger        string          `json:"trigger"`
	Prompt         json.RawMessage `json:"prompt"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolOutput     json.RawMessage `json:"tool_output"`
	Attachments    json.RawMessage `json:"attachments"`
	TranscriptPath string          `json:"transcript_path"`
}

// CanonicalDispatch is the adapter-neutral envelope for hooksengine.Request.
type CanonicalDispatch struct {
	SchemaVersion int
	Adapter       adapters.ID
	Event         hooks.Event
}

// DecodeNative reads and validates a bounded Cursor native payload.
// Exactly one top-level JSON value is accepted; concatenated values are rejected.
func DecodeNative(r io.Reader) (NativePayload, error) {
	if r == nil {
		return NativePayload{}, fmt.Errorf("cursor hooks: empty body")
	}
	limited := &io.LimitedReader{R: r, N: MaxNativeBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return NativePayload{}, fmt.Errorf("cursor hooks: read: %w", err)
	}
	if len(raw) == 0 {
		return NativePayload{}, fmt.Errorf("cursor hooks: empty body")
	}
	if len(raw) > MaxNativeBytes {
		return NativePayload{}, fmt.Errorf("cursor hooks: request exceeds %d bytes", MaxNativeBytes)
	}
	if !utf8.Valid(raw) {
		return NativePayload{}, fmt.Errorf("cursor hooks: body is not valid UTF-8")
	}
	for _, b := range raw {
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' {
			return NativePayload{}, fmt.Errorf("cursor hooks: body contains control characters")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var p NativePayload
	if err := dec.Decode(&p); err != nil {
		return NativePayload{}, fmt.Errorf("cursor hooks: invalid json: %w", err)
	}
	// dec.More() is insufficient for concatenated top-level values; require EOF.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return NativePayload{}, fmt.Errorf("cursor hooks: trailing data after JSON value")
	}
	p.HookEventName = strings.TrimSpace(p.HookEventName)
	p.ConversationID = strings.TrimSpace(p.ConversationID)
	p.GenerationID = strings.TrimSpace(p.GenerationID)
	p.SessionID = strings.TrimSpace(p.SessionID)
	p.ToolName = strings.TrimSpace(p.ToolName)
	p.ToolUseID = strings.TrimSpace(p.ToolUseID)
	p.Trigger = strings.TrimSpace(p.Trigger)
	// Drop sensitive/large fields immediately — never reach canonical translation
	// and never echo untrusted input in responses.
	p.Prompt = nil
	p.ToolInput = nil
	p.ToolOutput = nil
	p.Attachments = nil
	p.TranscriptPath = ""
	if p.HookEventName == "" {
		return NativePayload{}, fmt.Errorf("cursor hooks: hook_event_name required")
	}
	return p, nil
}

// SupportsNative reports whether event is in the approved Cursor mapping.
func SupportsNative(event string) bool {
	switch event {
	case NativeSessionStart, NativeBeforeSubmitPrompt, NativePreToolUse, NativePostToolUse, NativePreCompact:
		return true
	default:
		return false
	}
}

// TranslateToCanonical maps a native Cursor payload to an Atlas canonical event.
// Returns ok=false when the native event is unsupported or required identity is missing.
//
// Event.ID is an occurrence identifier (unique per invocation). Correlation fields
// remain in Payload (SessionID / TaskID / ToolCallID) and are never used alone as
// the occurrence ID.
func TranslateToCanonical(p NativePayload, occurredAt time.Time) (CanonicalDispatch, bool, error) {
	if !SupportsNative(p.HookEventName) {
		return CanonicalDispatch{}, false, nil
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	} else {
		occurredAt = occurredAt.UTC()
	}

	session := p.SessionID
	if session == "" {
		session = p.ConversationID
	}

	ev := hooks.Event{
		ID:         newOccurrenceID(p.HookEventName),
		OccurredAt: occurredAt,
		Adapter:    adapters.Cursor,
		Payload:    hooks.Payload{},
	}

	switch p.HookEventName {
	case NativeSessionStart:
		ev.Type = hooks.EventSessionStart
		if session == "" {
			return CanonicalDispatch{}, false, fmt.Errorf("cursor hooks: session_id required")
		}
		ev.Payload.SessionID = session
	case NativeBeforeSubmitPrompt:
		// Interaction activity — not a permanent task boundary.
		ev.Type = hooks.EventTaskStart
		if p.GenerationID == "" {
			return CanonicalDispatch{}, false, fmt.Errorf("cursor hooks: generation_id required")
		}
		ev.Payload.TaskID = p.GenerationID
		ev.Payload.TaskSummaryState = hooks.TaskSummaryAbsent
		ev.Payload.SessionID = session
	case NativePreToolUse:
		ev.Type = hooks.EventBeforeToolExecution
		if p.ToolName == "" {
			return CanonicalDispatch{}, false, fmt.Errorf("cursor hooks: tool_name required")
		}
		ev.Payload.ToolName = p.ToolName
		ev.Payload.ToolCallID = normalizeCursorToolUseID(p.ToolUseID)
		ev.Payload.SessionID = session
	case NativePostToolUse:
		ev.Type = hooks.EventAfterToolExecution
		if p.ToolName == "" {
			return CanonicalDispatch{}, false, fmt.Errorf("cursor hooks: tool_name required")
		}
		ev.Payload.ToolName = p.ToolName
		ev.Payload.ToolCallID = normalizeCursorToolUseID(p.ToolUseID)
		ev.Payload.Status = "success"
		ev.Payload.SessionID = session
	case NativePreCompact:
		ev.Type = hooks.EventBeforeContextCompaction
		ev.Payload.Reason = p.Trigger
		ev.Payload.SessionID = session
	default:
		return CanonicalDispatch{}, false, nil
	}

	return CanonicalDispatch{
		SchemaVersion: 1,
		Adapter:       adapters.Cursor,
		Event:         ev,
	}, true, nil
}

// newOccurrenceID returns a bounded, non-sensitive occurrence identifier.
// Uses crypto/rand (no network). Correlation IDs stay in Payload fields.
func newOccurrenceID(nativeEvent string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to nanosecond uniqueness without leaking native IDs.
		return trimID("cursor-" + sanitizeEventToken(nativeEvent) + "-" + fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	return trimID("cursor-" + sanitizeEventToken(nativeEvent) + "-" + hex.EncodeToString(b[:]))
}

func sanitizeEventToken(event string) string {
	event = strings.TrimSpace(event)
	if event == "" {
		return "event"
	}
	var b strings.Builder
	for _, r := range event {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := b.String()
	if out == "" {
		return "event"
	}
	return out
}

func trimID(id string) string {
	if len(id) > 128 {
		return id[:128]
	}
	return id
}

// normalizeCursorToolUseID maps Cursor tool_use_id onto the optional ToolCallID
// contract. hooks.ValidateEvent is unchanged.
//
// A value that already validates is returned unchanged.
// Cursor IDE 3.20.10 joins two identifiers with an interior newline
// ("call-…\nfc_…"). Segments are split on newline only. The first segment that
// itself validates is selected. When none validates, the result is empty:
// ToolCallID is optional, and this function does not invent a replacement.
func normalizeCursorToolUseID(raw string) string {
	if cursorToolCallIDAccepted(raw) {
		return raw
	}
	for _, seg := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r'
	}) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if cursorToolCallIDAccepted(seg) {
			return seg
		}
	}
	return ""
}

// cursorToolCallIDAccepted reports whether id can be stored in Payload.ToolCallID.
// The probe event is otherwise valid, so a rejection is the tool_call_id contract.
func cursorToolCallIDAccepted(id string) bool {
	ev := hooks.Event{
		ID:         "cursor-tool-id-probe",
		Type:       hooks.EventAfterToolExecution,
		OccurredAt: time.Unix(0, 0).UTC(),
		Payload: hooks.Payload{
			ToolName:   "Write",
			ToolCallID: id,
			Status:     "success",
		},
	}
	return hooks.ValidateEvent(ev) == nil
}

// NativeResult is Cursor-facing stdout JSON plus process exit guidance.
type NativeResult struct {
	Body     json.RawMessage
	ExitCode int
}

// TranslateResponse maps a dispatch status to Cursor native stdout JSON.
// Responses never echo untrusted native input.
func TranslateResponse(nativeEvent, status string) NativeResult {
	denied := status == StatusDenied

	switch nativeEvent {
	case NativeSessionStart:
		return nativeOK(map[string]any{})
	case NativeBeforeSubmitPrompt:
		if denied {
			return nativeOK(map[string]any{
				"continue":     false,
				"user_message": publicDenyMessage,
			})
		}
		return nativeOK(map[string]any{"continue": true})
	case NativePreToolUse:
		if denied {
			return nativeOK(map[string]any{
				"permission":   "deny",
				"user_message": publicDenyMessage,
			})
		}
		return nativeOK(map[string]any{"permission": "allow"})
	case NativePostToolUse, NativePreCompact:
		// Cursor requires {}. Dispatch status, including invalid_event, is not
		// part of this response. Surfacing it needs a separate observability
		// record and is intentionally not added here.
		return nativeOK(map[string]any{})
	default:
		return nativeOK(map[string]any{})
	}
}

// FailOpenResponse returns a safe Cursor-native response for decode/translate failures.
func FailOpenResponse(nativeEvent string) NativeResult {
	switch nativeEvent {
	case NativeBeforeSubmitPrompt:
		return nativeOK(map[string]any{"continue": true})
	case NativePreToolUse:
		return nativeOK(map[string]any{"permission": "allow"})
	default:
		return nativeOK(map[string]any{})
	}
}

func nativeOK(v map[string]any) NativeResult {
	data, err := json.Marshal(v)
	if err != nil {
		return NativeResult{Body: []byte("{}"), ExitCode: 0}
	}
	return NativeResult{Body: data, ExitCode: 0}
}

// DispatchFunc executes a canonical Atlas hooks dispatch and returns status.
type DispatchFunc func(ctx context.Context, req CanonicalDispatch) (status string)

// RunNative executes the Cursor native → Atlas → Cursor pipeline.
// It always writes valid Cursor JSON to out. Process exit is 0 (deny via JSON).
func RunNative(ctx context.Context, dispatch DispatchFunc, r io.Reader, out io.Writer) error {
	if dispatch == nil {
		return writeNative(out, FailOpenResponse(""))
	}
	p, err := DecodeNative(r)
	if err != nil {
		return writeNative(out, FailOpenResponse(""))
	}
	if !SupportsNative(p.HookEventName) {
		return writeNative(out, FailOpenResponse(p.HookEventName))
	}
	req, ok, terr := TranslateToCanonical(p, time.Now().UTC())
	if terr != nil || !ok {
		return writeNative(out, FailOpenResponse(p.HookEventName))
	}
	status := dispatch(ctx, req)
	return writeNative(out, TranslateResponse(p.HookEventName, status))
}

func writeNative(out io.Writer, res NativeResult) error {
	if out == nil {
		return fmt.Errorf("cursor hooks: nil stdout")
	}
	if len(res.Body) == 0 {
		res.Body = []byte("{}")
	}
	_, err := out.Write(append(append([]byte(nil), res.Body...), '\n'))
	return err
}
