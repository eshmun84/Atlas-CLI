package cursor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

func TestDecodeNative_BoundsAndSanitization(t *testing.T) {
	raw := `{
		"hook_event_name":"preToolUse",
		"conversation_id":"c1",
		"generation_id":"g1",
		"tool_name":"Shell",
		"tool_use_id":"tu1",
		"prompt":"SECRET PROMPT MUST NOT LEAK",
		"tool_input":{"command":"rm -rf /"},
		"transcript_path":"/tmp/transcript"
	}`
	p, err := cursor.DecodeNative(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if p.ToolName != "Shell" || p.Prompt != nil || p.ToolInput != nil || p.TranscriptPath != "" {
		t.Fatalf("sanitization failed: %#v", p)
	}

	big := `{"hook_event_name":"` + strings.Repeat("a", cursor.MaxNativeBytes) + `"}`
	if _, err := cursor.DecodeNative(strings.NewReader(big)); err == nil {
		t.Fatal("expected oversized rejection")
	}
	if _, err := cursor.DecodeNative(strings.NewReader("\xff\xfe")); err == nil {
		t.Fatal("expected invalid utf8 rejection")
	}
	if _, err := cursor.DecodeNative(strings.NewReader("{\"hook_event_name\":\"x\",\x00}")); err == nil {
		t.Fatal("expected control char rejection")
	}
	if _, err := cursor.DecodeNative(strings.NewReader(`{"hook_event_name":"preToolUse"}{"hook_event_name":"stop"}`)); err == nil {
		t.Fatal("expected concatenated JSON rejection")
	}
	if _, err := cursor.DecodeNative(strings.NewReader(`{"hook_event_name":"preToolUse"} trailing`)); err == nil {
		t.Fatal("expected trailing garbage rejection")
	}
	if _, err := cursor.DecodeNative(strings.NewReader(`{"tool_name":"Shell"}`)); err == nil {
		t.Fatal("expected missing hook_event_name rejection")
	}
}

func TestTranslateToCanonical_SupportedMappings(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		native cursor.NativePayload
		want   hooks.EventType
	}{
		{
			name: "sessionStart",
			native: cursor.NativePayload{
				HookEventName: cursor.NativeSessionStart,
				SessionID:     "s1",
			},
			want: hooks.EventSessionStart,
		},
		{
			name: "beforeSubmitPrompt",
			native: cursor.NativePayload{
				HookEventName:  cursor.NativeBeforeSubmitPrompt,
				ConversationID: "c1",
				GenerationID:   "g1",
			},
			want: hooks.EventTaskStart,
		},
		{
			name: "preToolUse",
			native: cursor.NativePayload{
				HookEventName: cursor.NativePreToolUse,
				ToolName:      "Write",
				ToolUseID:     "tu",
			},
			want: hooks.EventBeforeToolExecution,
		},
		{
			name: "postToolUse",
			native: cursor.NativePayload{
				HookEventName: cursor.NativePostToolUse,
				ToolName:      "Write",
			},
			want: hooks.EventAfterToolExecution,
		},
		{
			name: "preCompact",
			native: cursor.NativePayload{
				HookEventName: cursor.NativePreCompact,
				Trigger:       "auto",
			},
			want: hooks.EventBeforeContextCompaction,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, ok, err := cursor.TranslateToCanonical(tc.native, now)
			if err != nil || !ok {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			if req.Adapter != adapters.Cursor || req.SchemaVersion != 1 {
				t.Fatalf("envelope: %#v", req)
			}
			if req.Event.Type != tc.want {
				t.Fatalf("type=%q want %q", req.Event.Type, tc.want)
			}
			if tc.want == hooks.EventTaskStart {
				if req.Event.Payload.TaskSummaryState != hooks.TaskSummaryAbsent || req.Event.Payload.TaskSummary != "" {
					t.Fatalf("task summary must be absent: %#v", req.Event.Payload)
				}
				if req.Event.Payload.TaskID != "g1" {
					t.Fatalf("task_id=%q", req.Event.Payload.TaskID)
				}
			}
			if tc.want == hooks.EventBeforeContextCompaction && req.Event.Payload.Reason != "auto" {
				t.Fatalf("reason=%q", req.Event.Payload.Reason)
			}
		})
	}
}

func TestTranslateToCanonical_UnsupportedAndMissing(t *testing.T) {
	_, ok, err := cursor.TranslateToCanonical(cursor.NativePayload{HookEventName: "stop"}, time.Now())
	if err != nil || ok {
		t.Fatalf("stop must be unsupported ok=%v err=%v", ok, err)
	}
	_, ok, err = cursor.TranslateToCanonical(cursor.NativePayload{HookEventName: cursor.NativeSessionStart}, time.Now())
	if err == nil || ok {
		t.Fatal("sessionStart without session_id must fail")
	}
	_, ok, err = cursor.TranslateToCanonical(cursor.NativePayload{
		HookEventName: cursor.NativeBeforeSubmitPrompt,
	}, time.Now())
	if err == nil || ok {
		t.Fatal("beforeSubmitPrompt without generation_id must fail")
	}
}

func TestTranslateResponse_DenyAndFailOpen(t *testing.T) {
	res := cursor.TranslateResponse(cursor.NativePreToolUse, cursor.StatusDenied)
	var body map[string]any
	if err := json.Unmarshal(res.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["permission"] != "deny" {
		t.Fatalf("body=%v", body)
	}

	res = cursor.TranslateResponse(cursor.NativeBeforeSubmitPrompt, cursor.StatusDenied)
	if err := json.Unmarshal(res.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["continue"] != false {
		t.Fatalf("body=%v", body)
	}

	res = cursor.TranslateResponse(cursor.NativePreToolUse, "success")
	if err := json.Unmarshal(res.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["permission"] != "allow" {
		t.Fatalf("body=%v", body)
	}

	res = cursor.TranslateResponse(cursor.NativeSessionStart, cursor.StatusDenied)
	if string(res.Body) != "{}" {
		t.Fatalf("sessionStart must not block: %s", res.Body)
	}

	fo := cursor.FailOpenResponse(cursor.NativePreToolUse)
	if err := json.Unmarshal(fo.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["permission"] != "allow" {
		t.Fatalf("fail-open=%v", body)
	}
}

func TestRunNative_DenyPropagationAndNoPromptLeak(t *testing.T) {
	var saw cursor.CanonicalDispatch
	dispatch := func(_ context.Context, req cursor.CanonicalDispatch) string {
		saw = req
		return cursor.StatusDenied
	}
	in := `{"hook_event_name":"preToolUse","tool_name":"Shell","tool_use_id":"1","prompt":"LEAKME","conversation_id":"c"}`
	var out bytes.Buffer
	if err := cursor.RunNative(context.Background(), dispatch, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "LEAKME") {
		t.Fatalf("prompt leaked: %s", out.String())
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["permission"] != "deny" {
		t.Fatalf("body=%v", body)
	}
	if saw.Event.Payload.ToolName != "Shell" || saw.Event.Type != hooks.EventBeforeToolExecution {
		t.Fatalf("saw=%#v", saw)
	}
}

func TestHooksBridge_DeclaredEvents(t *testing.T) {
	b := cursor.NewHooksBridge()
	if b.AdapterID() != adapters.Cursor {
		t.Fatal(b.AdapterID())
	}
	got := b.DeclaredEvents()
	if len(got) != len(cursor.DeclaredCanonicalEvents) {
		t.Fatalf("declared=%v", got)
	}
	for _, et := range got {
		if et == hooks.EventSessionResume || et == hooks.EventTaskComplete {
			t.Fatalf("must not declare %s", et)
		}
	}
}

func TestTranslateToCanonical_OccurrenceIDsDistinct(t *testing.T) {
	now := time.Now().UTC()
	payload := cursor.NativePayload{
		HookEventName:  cursor.NativePreToolUse,
		ConversationID: "same-conv",
		GenerationID:   "same-gen",
		ToolName:       "Shell",
		ToolUseID:      "same-tool",
	}
	seen := map[string]struct{}{}
	for i := 0; i < 20; i++ {
		req, ok, err := cursor.TranslateToCanonical(payload, now)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if _, dup := seen[req.Event.ID]; dup {
			t.Fatalf("occurrence id collision: %s", req.Event.ID)
		}
		seen[req.Event.ID] = struct{}{}
		if len(req.Event.ID) == 0 || len(req.Event.ID) > 128 {
			t.Fatalf("id bounds: %q", req.Event.ID)
		}
		if req.Event.Payload.SessionID != "same-conv" || req.Event.Payload.ToolCallID != "same-tool" {
			t.Fatalf("correlation fields lost: %#v", req.Event.Payload)
		}
		if strings.Contains(req.Event.ID, "same-conv") || strings.Contains(req.Event.ID, "same-tool") {
			t.Fatalf("occurrence id must not embed native correlation ids: %s", req.Event.ID)
		}
	}

	a, _, _ := cursor.TranslateToCanonical(cursor.NativePayload{
		HookEventName: cursor.NativeSessionStart, SessionID: "s1",
	}, now)
	b, _, _ := cursor.TranslateToCanonical(cursor.NativePayload{
		HookEventName: cursor.NativePreCompact, SessionID: "s1", Trigger: "auto",
	}, now)
	if a.Event.ID == b.Event.ID {
		t.Fatal("different events must not share occurrence id")
	}
}
