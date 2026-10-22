package hookactions_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
)

// Native postToolUse is admitted through the production Cursor bridge and the
// maintenance registry. Cursor stdout is not the admission signal.
func TestCursorPostToolUse_NativeAdmissionAndBump(t *testing.T) {
	hookactions.SetTriggersEnabled(true)
	const (
		callID = "call-ed199ee1-8765-4eb0-b631-dd5f84abb6b2-34"
		fcID   = "fc_5d211f72-be2c-9ab3-827c-ef80c05626f1_0"
	)
	cases := []struct {
		name     string
		tool     string
		useID    string
		wantID   string
		wantBump bool
	}{
		{
			name:     "write compound",
			tool:     "Write",
			useID:    callID + "\n" + fcID,
			wantID:   callID,
			wantBump: true,
		},
		{
			name:     "write simple",
			tool:     "Write",
			useID:    "call-16c26c7f-6b30-4afe-9077-205c1206c56c-25",
			wantID:   "call-16c26c7f-6b30-4afe-9077-205c1206c56c-25",
			wantBump: true,
		},
		{
			name:     "write empty",
			tool:     "Write",
			useID:    "",
			wantID:   "",
			wantBump: true,
		},
		{
			name:     "write malformed",
			tool:     "Write",
			useID:    "../not-an-id",
			wantID:   "",
			wantBump: true,
		},
		{
			name:     "read",
			tool:     "Read",
			useID:    callID + "\n" + fcID,
			wantID:   callID,
			wantBump: false,
		},
		{
			name:     "shell",
			tool:     "Shell",
			useID:    callID + "\n" + fcID,
			wantID:   callID,
			wantBump: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spawns := c2cInstallSchedulerNoop(t)
			ref := seedInitializedProject(t, "cursor-id-"+strings.ReplaceAll(tc.name, " ", "-"))
			body, err := json.Marshal(map[string]any{
				"hook_event_name": adaptercursor.NativePostToolUse,
				"tool_name":       tc.tool,
				"tool_use_id":     tc.useID,
				"session_id":      "1c893f7d-1208-4387-8ed6-a447046ff96c",
				"tool_input": map[string]string{
					"file_path": "internal/ledger/ledger.go",
					"content":   "package ledger",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			native, err := adaptercursor.DecodeNative(strings.NewReader(string(body)))
			if err != nil {
				t.Fatal(err)
			}
			canon, ok, err := adaptercursor.TranslateToCanonical(native, time.Date(2026, 10, 21, 23, 0, 0, 0, time.UTC))
			if err != nil || !ok {
				t.Fatalf("translate ok=%v err=%v", ok, err)
			}
			if canon.Event.Payload.ToolCallID != tc.wantID {
				t.Fatalf("tool_call_id=%q want %q", canon.Event.Payload.ToolCallID, tc.wantID)
			}
			if err := hooks.ValidateEvent(canon.Event); err != nil {
				t.Fatal(err)
			}

			eng := hooksengine.Engine{
				Registry: hooksengine.DefaultRegistry(),
				Env:      hooksengine.Env{Root: ref.Root, Home: ref.Home},
				BridgeLookup: func(id adapters.ID) (hooks.EventBridge, bool) {
					if id != adapters.Cursor {
						return nil, false
					}
					return adaptercursor.NewHooksBridge(), true
				},
			}
			resp := eng.Dispatch(context.Background(), hooksengine.Request{
				SchemaVersion: canon.SchemaVersion,
				Adapter:       string(canon.Adapter),
				Event: hooksengine.EventInput{
					ID:         canon.Event.ID,
					Type:       string(canon.Event.Type),
					OccurredAt: canon.Event.OccurredAt.UTC().Format(time.RFC3339Nano),
					Payload: hooksengine.PayloadInput{
						SessionID:  canon.Event.Payload.SessionID,
						ToolName:   canon.Event.Payload.ToolName,
						ToolCallID: canon.Event.Payload.ToolCallID,
						Status:     canon.Event.Payload.Status,
					},
				},
			})
			if resp.Status == hooksengine.StatusInvalidEvent {
				t.Fatalf("event rejected before admission: %#v", resp)
			}
			if resp.Status != hooksengine.StatusSuccess || resp.Aborted {
				t.Fatalf("resp=%#v", resp)
			}
			if resp.Count != 1 || resp.Results[0].ActionID != hookactions.ActionIDPostCode {
				t.Fatalf("results=%#v", resp.Results)
			}

			pendingPath := hookactions.MaintainPendingPath(ref.Home, ref.ProjectID)
			doc, err := hookactions.LoadMaintainPending(ref.Home, ref.ProjectID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantBump {
				if resp.Results[0].Outcome != string(hooks.OutcomeOK) {
					t.Fatalf("outcome=%s", resp.Results[0].Outcome)
				}
				if doc.PendingGeneration != 1 || doc.ProcessedGeneration != 0 || doc.PendingGeneration <= doc.ProcessedGeneration {
					t.Fatalf("pending=%d processed=%d", doc.PendingGeneration, doc.ProcessedGeneration)
				}
				if spawns.Load() != 1 {
					t.Fatalf("scheduler spawns=%d", spawns.Load())
				}
				return
			}
			if resp.Results[0].Outcome != string(hooks.OutcomeSkipped) {
				t.Fatalf("outcome=%s", resp.Results[0].Outcome)
			}
			if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
				t.Fatalf("non-mutating tool created pending: err=%v doc=%+v", err, doc)
			}
			if doc.PendingGeneration != 0 || doc.ProcessedGeneration != 0 {
				t.Fatalf("pending=%d processed=%d", doc.PendingGeneration, doc.ProcessedGeneration)
			}
			if spawns.Load() != 0 {
				t.Fatalf("scheduler spawns=%d", spawns.Load())
			}
		})
	}
}
