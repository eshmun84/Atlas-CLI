package opencode

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
	NativeSessionCreated    = "session.created"
	NativeSessionPrompt     = "session.prompt"
	NativeToolExecuteBefore = "tool.execute.before"
	NativeToolExecuteAfter  = "tool.execute.after"
	NativeSessionCompaction = "session.compaction"

	// MaxNativeBytes bounds untrusted OpenCode plugin stdin.
	MaxNativeBytes = 64 << 10

	publicDenyMessage = "blocked by atlas hooks"

	// StatusDenied matches hooksengine.StatusDenied without importing that package.
	StatusDenied = "denied"
)

// NativePayload is a bounded decode of OpenCode plugin stdin JSON.
// Sensitive fields (prompt/tool args/output) are dropped immediately if present.
type NativePayload struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
	TaskID        string `json:"task_id"`
	ToolName      string `json:"tool_name"`
	ToolCallID    string `json:"tool_call_id"`
	// SessionDirectory is the verified OpenCode session directory.
	// Required before any maintenance write. Not inferred from the plugin cwd.
	SessionDirectory string          `json:"session_directory"`
	Status           string          `json:"status"`
	Reason           string          `json:"reason"`
	Prompt           json.RawMessage `json:"prompt"`
	ToolInput        json.RawMessage `json:"tool_input"`
	ToolOutput       json.RawMessage `json:"tool_output"`
}

// CanonicalDispatch is the adapter-neutral envelope for hooksengine.Request.
type CanonicalDispatch struct {
	SchemaVersion int
	Adapter       adapters.ID
	Event         hooks.Event
}

// DecodeNative reads and validates a bounded OpenCode native payload.
func DecodeNative(r io.Reader) (NativePayload, error) {
	if r == nil {
		return NativePayload{}, fmt.Errorf("opencode hooks: empty body")
	}
	limited := &io.LimitedReader{R: r, N: MaxNativeBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return NativePayload{}, fmt.Errorf("opencode hooks: read: %w", err)
	}
	if len(raw) == 0 {
		return NativePayload{}, fmt.Errorf("opencode hooks: empty body")
	}
	if len(raw) > MaxNativeBytes {
		return NativePayload{}, fmt.Errorf("opencode hooks: request exceeds %d bytes", MaxNativeBytes)
	}
	if !utf8.Valid(raw) {
		return NativePayload{}, fmt.Errorf("opencode hooks: body is not valid UTF-8")
	}
	for _, b := range raw {
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' {
			return NativePayload{}, fmt.Errorf("opencode hooks: body contains control characters")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var p NativePayload
	if err := dec.Decode(&p); err != nil {
		return NativePayload{}, fmt.Errorf("opencode hooks: invalid json: %w", err)
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return NativePayload{}, fmt.Errorf("opencode hooks: trailing data after JSON value")
	}
	p.HookEventName = strings.TrimSpace(p.HookEventName)
	p.SessionID = strings.TrimSpace(p.SessionID)
	p.TaskID = strings.TrimSpace(p.TaskID)
	p.ToolName = strings.TrimSpace(p.ToolName)
	p.ToolCallID = strings.TrimSpace(p.ToolCallID)
	p.SessionDirectory = strings.TrimSpace(p.SessionDirectory)
	p.Status = strings.TrimSpace(p.Status)
	p.Reason = strings.TrimSpace(p.Reason)
	// Drop sensitive/large fields immediately.
	p.Prompt = nil
	p.ToolInput = nil
	p.ToolOutput = nil
	if p.HookEventName == "" {
		return NativePayload{}, fmt.Errorf("opencode hooks: hook_event_name required")
	}
	if SupportsNative(p.HookEventName) && p.SessionDirectory == "" {
		return NativePayload{}, fmt.Errorf("opencode hooks: session_directory required")
	}
	return p, nil
}

// SupportsNative reports whether event is in the approved OpenCode mapping.
func SupportsNative(event string) bool {
	switch event {
	case NativeSessionCreated, NativeSessionPrompt, NativeToolExecuteBefore, NativeToolExecuteAfter, NativeSessionCompaction:
		return true
	default:
		return false
	}
}

// TranslateToCanonical maps a native OpenCode payload to an Atlas canonical event.
func TranslateToCanonical(p NativePayload, occurredAt time.Time) (CanonicalDispatch, bool, error) {
	if !SupportsNative(p.HookEventName) {
		return CanonicalDispatch{}, false, nil
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	} else {
		occurredAt = occurredAt.UTC()
	}

	if p.SessionDirectory == "" {
		return CanonicalDispatch{}, false, fmt.Errorf("opencode hooks: session_directory required")
	}
	ev := hooks.Event{
		ID:         newOccurrenceID(p.HookEventName),
		OccurredAt: occurredAt,
		Adapter:    adapters.OpenCode,
		Payload:    hooks.Payload{SessionDirectory: p.SessionDirectory},
	}

	switch p.HookEventName {
	case NativeSessionCreated:
		ev.Type = hooks.EventSessionStart
		if p.SessionID == "" {
			return CanonicalDispatch{}, false, fmt.Errorf("opencode hooks: session_id required")
		}
		ev.Payload.SessionID = p.SessionID
	case NativeSessionPrompt:
		// Interaction boundary — not a permanent development-task boundary.
		ev.Type = hooks.EventTaskStart
		taskID := p.TaskID
		if taskID == "" {
			taskID = newOccurrenceID("prompt")
		}
		ev.Payload.TaskID = taskID
		ev.Payload.TaskSummaryState = hooks.TaskSummaryAbsent
		ev.Payload.SessionID = p.SessionID
	case NativeToolExecuteBefore:
		ev.Type = hooks.EventBeforeToolExecution
		if p.ToolName == "" {
			return CanonicalDispatch{}, false, fmt.Errorf("opencode hooks: tool_name required")
		}
		ev.Payload.ToolName = p.ToolName
		ev.Payload.ToolCallID = p.ToolCallID
		ev.Payload.SessionID = p.SessionID
	case NativeToolExecuteAfter:
		ev.Type = hooks.EventAfterToolExecution
		if p.ToolName == "" {
			return CanonicalDispatch{}, false, fmt.Errorf("opencode hooks: tool_name required")
		}
		ev.Payload.ToolName = p.ToolName
		ev.Payload.ToolCallID = p.ToolCallID
		ev.Payload.SessionID = p.SessionID
		status := p.Status
		if status == "" {
			status = "success"
		}
		ev.Payload.Status = status
	case NativeSessionCompaction:
		ev.Type = hooks.EventBeforeContextCompaction
		ev.Payload.Reason = p.Reason
		ev.Payload.SessionID = p.SessionID
	default:
		return CanonicalDispatch{}, false, nil
	}

	return CanonicalDispatch{
		SchemaVersion: 1,
		Adapter:       adapters.OpenCode,
		Event:         ev,
	}, true, nil
}

func newOccurrenceID(nativeEvent string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return trimID("opencode-" + sanitizeEventToken(nativeEvent) + "-" + fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	return trimID("opencode-" + sanitizeEventToken(nativeEvent) + "-" + hex.EncodeToString(b[:]))
}

func sanitizeEventToken(event string) string {
	event = strings.TrimSpace(event)
	if event == "" {
		return "event"
	}
	var b strings.Builder
	for _, r := range event {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			if r == '.' {
				b.WriteByte('-')
				continue
			}
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

// NativeResult is OpenCode-plugin-facing stdout JSON.
type NativeResult struct {
	Body     json.RawMessage
	ExitCode int
}

// TranslateResponse maps a dispatch status to OpenCode plugin JSON.
//
// Contract:
//   - tool.execute.before + denied → status=denied, decision=deny (blocking).
//   - Any other event, including denied informational outcomes → decision=allow
//     and status never "denied" (normalized to success). The plugin must not
//     observe status=denied with decision=allow.
func TranslateResponse(nativeEvent, status string) NativeResult {
	denied := status == StatusDenied
	if denied && nativeEvent == NativeToolExecuteBefore {
		return nativeOK(map[string]any{
			"schema_version": 1,
			"status":         StatusDenied,
			"decision":       "deny",
			"message":        publicDenyMessage,
		})
	}
	outStatus := statusOrSuccess(status)
	if outStatus == StatusDenied {
		// Informational path: never pair status=denied with decision=allow.
		outStatus = "success"
	}
	return nativeOK(map[string]any{
		"schema_version": 1,
		"status":         outStatus,
		"decision":       "allow",
	})
}

// FailOpenResponse returns a safe allow response for decode/translate failures.
func FailOpenResponse(nativeEvent string) NativeResult {
	_ = nativeEvent
	return nativeOK(map[string]any{
		"schema_version": 1,
		"status":         "success",
		"decision":       "allow",
	})
}

func statusOrSuccess(status string) string {
	if strings.TrimSpace(status) == "" {
		return "success"
	}
	return status
}

func nativeOK(v map[string]any) NativeResult {
	data, err := json.Marshal(v)
	if err != nil {
		return NativeResult{Body: []byte(`{"schema_version":1,"status":"success","decision":"allow"}`), ExitCode: 0}
	}
	return NativeResult{Body: data, ExitCode: 0}
}

// DispatchFunc executes a canonical Atlas hooks dispatch and returns status.
type DispatchFunc func(ctx context.Context, req CanonicalDispatch) (status string)

// RunNative executes the OpenCode native → Atlas → OpenCode pipeline.
// Process exit is always 0; deny is expressed via decision JSON.
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
		return fmt.Errorf("opencode hooks: nil stdout")
	}
	if len(res.Body) == 0 {
		res.Body = []byte(`{"schema_version":1,"status":"success","decision":"allow"}`)
	}
	_, err := out.Write(append(append([]byte(nil), res.Body...), '\n'))
	return err
}
