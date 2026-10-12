package opencode_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/adapters/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

func TestDecodeNative_RejectsMalformed(t *testing.T) {
	cases := []string{
		``,
		`null`,
		`[]`,
		`{"hook_event_name":"session.created"}{"x":1}`,
		string([]byte{0xff, 0xfe}),
		`{"hook_event_name":"session.created","session_id":"s1","x":` + strings.Repeat("a", opencode.MaxNativeBytes) + `}`,
	}
	for _, raw := range cases {
		if _, err := opencode.DecodeNative(strings.NewReader(raw)); err == nil {
			t.Fatalf("expected error for %q", raw[:min(40, len(raw))])
		}
	}
}

func TestTranslateToCanonical_AllEvents(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		want   hooks.EventType
		check  func(t *testing.T, d opencode.CanonicalDispatch)
		wantOK bool
	}{
		{
			name: "session.created",
			raw:  `{"hook_event_name":"session.created","session_id":"s1"}`,
			want: hooks.EventSessionStart,
			check: func(t *testing.T, d opencode.CanonicalDispatch) {
				if d.Event.Payload.SessionID != "s1" {
					t.Fatal(d.Event.Payload)
				}
			},
			wantOK: true,
		},
		{
			name: "session.prompt",
			raw:  `{"hook_event_name":"session.prompt","session_id":"s1"}`,
			want: hooks.EventTaskStart,
			check: func(t *testing.T, d opencode.CanonicalDispatch) {
				if d.Event.Payload.TaskSummaryState != hooks.TaskSummaryAbsent {
					t.Fatal("must preserve absent task summary")
				}
				if d.Event.Payload.TaskID == "" {
					t.Fatal("task_id required")
				}
				if d.Event.Payload.TaskSummary != "" {
					t.Fatal("must not fabricate summary")
				}
			},
			wantOK: true,
		},
		{
			name: "tool.execute.before",
			raw:  `{"hook_event_name":"tool.execute.before","tool_name":"read","tool_call_id":"c1","session_id":"s1"}`,
			want: hooks.EventBeforeToolExecution,
			check: func(t *testing.T, d opencode.CanonicalDispatch) {
				if d.Event.Payload.ToolName != "read" || d.Event.Payload.ToolCallID != "c1" {
					t.Fatal(d.Event.Payload)
				}
			},
			wantOK: true,
		},
		{
			name: "tool.execute.after",
			raw:  `{"hook_event_name":"tool.execute.after","tool_name":"read","status":"error"}`,
			want: hooks.EventAfterToolExecution,
			check: func(t *testing.T, d opencode.CanonicalDispatch) {
				if d.Event.Payload.Status != "error" {
					t.Fatal(d.Event.Payload.Status)
				}
			},
			wantOK: true,
		},
		{
			name: "session.compaction",
			raw:  `{"hook_event_name":"session.compaction","session_id":"s1","reason":"compaction"}`,
			want: hooks.EventBeforeContextCompaction,
			check: func(t *testing.T, d opencode.CanonicalDispatch) {
				if d.Event.Payload.Reason != "compaction" {
					t.Fatal(d.Event.Payload.Reason)
				}
			},
			wantOK: true,
		},
		{
			name:   "unknown",
			raw:    `{"hook_event_name":"session.idle","session_id":"s1"}`,
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := opencode.DecodeNative(strings.NewReader(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			// Ensure sensitive fields never stick.
			if len(p.Prompt) != 0 || len(p.ToolInput) != 0 || len(p.ToolOutput) != 0 {
				t.Fatal("sensitive fields retained")
			}
			d, ok, terr := opencode.TranslateToCanonical(p, time.Now().UTC())
			if terr != nil {
				t.Fatal(terr)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if d.Adapter != adapters.OpenCode || d.Event.Type != tc.want {
				t.Fatalf("%#v", d)
			}
			if d.Event.ID == "" {
				t.Fatal("occurrence id required")
			}
			if tc.check != nil {
				tc.check(t, d)
			}
		})
	}
}

func TestTranslateResponse_DenyOnlyToolBefore(t *testing.T) {
	res := opencode.TranslateResponse(opencode.NativeToolExecuteBefore, opencode.StatusDenied)
	var body map[string]any
	if err := json.Unmarshal(res.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["decision"] != "deny" || body["message"] != "blocked by atlas hooks" {
		t.Fatalf("%v", body)
	}
	// Informational denial → allow, and must not emit status=denied with decision=allow.
	res2 := opencode.TranslateResponse(opencode.NativeSessionPrompt, opencode.StatusDenied)
	var body2 map[string]any
	_ = json.Unmarshal(res2.Body, &body2)
	if body2["decision"] != "allow" || body2["status"] != "success" {
		t.Fatalf("prompt must normalize informational deny: %v", body2)
	}
}

func TestRunNative_DenyAndFailOpen(t *testing.T) {
	var out bytes.Buffer
	err := opencode.RunNative(context.Background(), func(context.Context, opencode.CanonicalDispatch) string {
		return opencode.StatusDenied
	}, strings.NewReader(`{"hook_event_name":"tool.execute.before","tool_name":"bash"}`), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"decision":"deny"`) {
		t.Fatalf("%s", out.String())
	}

	out.Reset()
	err = opencode.RunNative(context.Background(), nil, strings.NewReader(`{`), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"decision":"allow"`) {
		t.Fatalf("fail-open: %s", out.String())
	}
}

func TestDecodeNative_DropsSensitiveFields(t *testing.T) {
	raw := `{"hook_event_name":"session.prompt","session_id":"s1","prompt":"SECRET","tool_input":{"x":1},"tool_output":"OUT"}`
	p, err := opencode.DecodeNative(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Prompt) != 0 || len(p.ToolInput) != 0 || len(p.ToolOutput) != 0 {
		t.Fatalf("sensitive retained: %#v", p)
	}
	d, ok, _ := opencode.TranslateToCanonical(p, time.Now().UTC())
	if !ok {
		t.Fatal("expected ok")
	}
	enc, _ := json.Marshal(d.Event.Payload)
	if strings.Contains(string(enc), "SECRET") || strings.Contains(string(enc), "OUT") {
		t.Fatalf("payload leaked secrets: %s", enc)
	}
}
