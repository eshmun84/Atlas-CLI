package hooksengine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
	"gopkg.in/yaml.v3"
)

type stubAction struct {
	id     string
	events []hooks.EventType
	mode   hooks.ExecutionMode
	run    func(ctx context.Context, ev hooks.Event) (hooks.Result, error)
}

func (a stubAction) ID() string                { return a.id }
func (a stubAction) Events() []hooks.EventType { return a.events }
func (a stubAction) Mode() hooks.ExecutionMode { return a.mode }
func (a stubAction) Run(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
	if a.run != nil {
		return a.run(ctx, ev)
	}
	return hooks.Result{Outcome: hooks.OutcomeOK}, nil
}

func TestDispatch_ProductionCursorHooksEnabledMaintenanceRegistry(t *testing.T) {
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), nil)
	req := validSessionRequest(projectID)
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("status=%q want success with Cursor Hooks+bridge: %#v", resp.Status, resp)
	}
	// seedEngine has config but no state.yaml → admission fail-opens to skipped.
	if resp.Aborted {
		t.Fatalf("maintenance dispatch must not abort: %#v", resp)
	}
	if resp.Count != 2 {
		t.Fatalf("want 2 maintenance actions, got %#v", resp)
	}
	for _, r := range resp.Results {
		if r.Outcome != string(hooks.OutcomeSkipped) {
			t.Fatalf("uninitialized-for-maintain project: want skipped, got %#v", r)
		}
	}
}

func TestDispatch_ProductionOpenCodeWithoutBridgeUnsupported(t *testing.T) {
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), nil)
	req := validSessionRequest(projectID)
	req.Adapter = string(adapters.OpenCode)
	req.Event.Payload.SessionDirectory = eng.Env.Root
	resp := eng.Dispatch(context.Background(), req)
	// ClassHooks is enabled, but package tests do not register the CLI bridge.
	if resp.Status != hooksengine.StatusUnsupportedEvent {
		t.Fatalf("status=%q want unsupported_event for opencode without bridge: %#v", resp.Status, resp)
	}
}

func TestDispatch_ValidWithOverrideMaintenanceRegistry(t *testing.T) {
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), &cap)
	resp := eng.Dispatch(context.Background(), validSessionRequest(projectID))
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("status=%q %#v", resp.Status, resp)
	}
	if resp.Aborted || resp.Count != 2 {
		t.Fatalf("maintenance registry dispatch: %#v", resp)
	}
	if resp.ProjectID != projectID || resp.Adapter != string(adapters.Cursor) {
		t.Fatalf("identity %#v", resp)
	}
}

func TestDispatch_InvalidAdapter(t *testing.T) {
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), nil)
	req := validSessionRequest(projectID)
	req.Adapter = "claude"
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusInvalidAdapter {
		t.Fatalf("status=%q", resp.Status)
	}
}

func TestDispatch_InvalidEventAndPayload(t *testing.T) {
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), &cap)

	req := validSessionRequest(projectID)
	req.Event.Type = "memory.write"
	if resp := eng.Dispatch(context.Background(), req); resp.Status != hooksengine.StatusInvalidEvent {
		t.Fatalf("unknown type status=%q", resp.Status)
	}

	req = validSessionRequest(projectID)
	req.Event.Payload.SessionID = ""
	if resp := eng.Dispatch(context.Background(), req); resp.Status != hooksengine.StatusInvalidEvent {
		t.Fatalf("missing session status=%q", resp.Status)
	}

	req = validSessionRequest(projectID)
	req.Event.OccurredAt = "not-a-time"
	if resp := eng.Dispatch(context.Background(), req); resp.Status != hooksengine.StatusInvalidEvent {
		t.Fatalf("bad time status=%q", resp.Status)
	}
}

func TestDispatch_UnsupportedEventWhenCapable(t *testing.T) {
	cap := capable(adapters.Cursor, hooks.EventSessionStart) // task.start not declared
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), &cap)
	req := hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID:         "task-1",
			Type:       string(hooks.EventTaskStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID:        projectID,
				TaskID:           "t1",
				TaskSummaryState: string(hooks.TaskSummaryAbsent),
			},
		},
	}
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusUnsupportedEvent {
		t.Fatalf("status=%q", resp.Status)
	}
}

func TestDispatch_ProjectMismatchAndIsolation(t *testing.T) {
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), &cap)
	req := validSessionRequest(projectID)
	req.Event.Payload.ProjectID = "other-project-ffffffff"
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusProjectMismatch {
		t.Fatalf("status=%q", resp.Status)
	}
}

func TestDispatch_NotInitialized(t *testing.T) {
	root := t.TempDir()
	eng := hooksengine.Engine{
		Registry: hooksengine.DefaultRegistry(),
		Env:      hooksengine.Env{Root: root, Home: t.TempDir()},
	}
	resp := eng.Dispatch(context.Background(), validSessionRequest("x"))
	if resp.Status != hooksengine.StatusNotInitialized {
		t.Fatalf("status=%q", resp.Status)
	}
}

func TestDispatch_UnsafeSymlinkConfig(t *testing.T) {
	root := t.TempDir()
	realCfgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(realCfgDir, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig("sym")
	data, _ := yaml.Marshal(&cfg)
	if err := os.WriteFile(filepath.Join(realCfgDir, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	// Symlink .atlas into project root — config.LoadAt must fail closed.
	if err := os.Symlink(filepath.Join(realCfgDir, ".atlas"), filepath.Join(root, ".atlas")); err != nil {
		t.Fatal(err)
	}
	eng := hooksengine.Engine{
		Registry: hooksengine.DefaultRegistry(),
		Env:      hooksengine.Env{Root: root, Home: t.TempDir()},
	}
	resp := eng.Dispatch(context.Background(), validSessionRequest("x"))
	if resp.Status != hooksengine.StatusUnsafeFilesystem {
		t.Fatalf("status=%q want unsafe_filesystem", resp.Status)
	}
}

func TestDecode_OversizedUnknownFieldsMalformed(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), hooksengine.MaxRequestBytes+1)
	if _, err := hooksengine.DecodeRequestBytes(huge); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("oversized: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"schema_version":1,"adapter":"cursor","event":{},"extra":true}`)); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("unknown field: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"schema_version":1,`)); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("malformed: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte("{\"schema_version\":1,\"adapter\":\"cursor\"}\n{}")); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("trailing: %v", err)
	}
}

func TestDecode_StrictSchemaVersion(t *testing.T) {
	validEvent := `,"adapter":"cursor","event":{"id":"e1","type":"session.start","occurred_at":"2026-10-11T00:00:00Z","payload":{"session_id":"s1"}}}`

	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"adapter":"cursor","event":{}}`)); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("missing schema_version: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"schema_version":0` + validEvent)); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("schema_version 0: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"schema_version":1` + validEvent)); err != nil {
		t.Fatalf("schema_version 1: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"schema_version":2` + validEvent)); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("schema_version 2: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"schema_version":"1"` + validEvent)); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("schema_version string: %v", err)
	}
	if _, err := hooksengine.DecodeRequestBytes([]byte(`{"schema_version":true` + validEvent)); !errors.Is(err, hooksengine.ErrInvalidRequest) {
		t.Fatalf("schema_version bool: %v", err)
	}
}

func TestDispatch_RejectsSchemaVersionZero(t *testing.T) {
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), &cap)
	req := validSessionRequest(projectID)
	req.SchemaVersion = 0
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusInvalidRequest {
		t.Fatalf("status=%q", resp.Status)
	}
}

func TestDispatch_DeterministicActionOrder(t *testing.T) {
	var order []string
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{id: "z-last", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking, run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
			order = append(order, "z-last")
			return hooks.Result{}, nil
		}}},
		{Action: stubAction{id: "a-first", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking, run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
			order = append(order, "a-first")
			return hooks.Result{}, nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	eng, projectID := seedEngine(t, reg, &cap)
	resp := eng.Dispatch(context.Background(), validSessionRequest(projectID))
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("%#v", resp)
	}
	if len(order) != 2 || order[0] != "a-first" || order[1] != "z-last" {
		t.Fatalf("order=%v", order)
	}
	if resp.Results[0].ActionID != "a-first" || resp.Results[1].ActionID != "z-last" {
		t.Fatalf("results=%#v", resp.Results)
	}
}

func TestDispatch_BlockingDenyAndNonBlockingFailure(t *testing.T) {
	var later atomic.Bool
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{
			id: "a-deny", events: []hooks.EventType{hooks.EventBeforeToolExecution}, mode: hooks.ModeBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				return hooks.Result{Outcome: hooks.OutcomeDenied, Message: "secret=/Users/me/token"}, nil
			},
		}},
		{Action: stubAction{
			id: "z-later", events: []hooks.EventType{hooks.EventBeforeToolExecution}, mode: hooks.ModeNonBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				later.Store(true)
				return hooks.Result{}, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cap := capable(adapters.Cursor, hooks.EventBeforeToolExecution)
	eng, projectID := seedEngine(t, reg, &cap)
	req := hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: "tool-1", Type: string(hooks.EventBeforeToolExecution),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload:    hooksengine.PayloadInput{ProjectID: projectID, ToolName: "Shell"},
		},
	}
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusDenied || !resp.Aborted {
		t.Fatalf("%#v", resp)
	}
	if later.Load() {
		t.Fatal("later must not run")
	}
	if strings.Contains(resp.Results[0].Message, "/Users/") || strings.Contains(resp.Results[0].Message, "secret") {
		t.Fatalf("leaked message=%q", resp.Results[0].Message)
	}
	if resp.Results[0].Message != "action denied continuation" {
		t.Fatalf("message=%q", resp.Results[0].Message)
	}
}

func TestDispatch_Timeout(t *testing.T) {
	reg, err := hooks.NewRegistry([]hooks.Registration{{
		Action: stubAction{
			id: "slow", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				select {
				case <-ctx.Done():
					return hooks.Result{}, ctx.Err()
				case <-time.After(2 * time.Second):
					return hooks.Result{}, nil
				}
			},
		},
		FailurePolicy: hooks.FailAbort,
		Timeout:       20 * time.Millisecond,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	eng, projectID := seedEngine(t, reg, &cap)
	resp := eng.Dispatch(context.Background(), validSessionRequest(projectID))
	if resp.Status != hooksengine.StatusAborted || !resp.Aborted {
		t.Fatalf("%#v", resp)
	}
	if resp.Results[0].Outcome != string(hooks.OutcomeTimeout) {
		t.Fatalf("results=%#v", resp.Results)
	}
}

func TestDispatch_Cancellation(t *testing.T) {
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{id: "a", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking}},
	})
	if err != nil {
		t.Fatal(err)
	}
	eng, projectID := seedEngine(t, reg, &cap)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp := eng.Dispatch(ctx, validSessionRequest(projectID))
	if resp.Status != hooksengine.StatusCancelled {
		t.Fatalf("%#v", resp)
	}
}

func TestDispatch_NoRegisteredActions(t *testing.T) {
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	empty, err := hooks.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	eng, projectID := seedEngine(t, empty, &cap)
	resp := eng.Dispatch(context.Background(), validSessionRequest(projectID))
	if resp.Status != hooksengine.StatusSuccess || resp.Count != 0 {
		t.Fatalf("%#v", resp)
	}
}

func TestDispatch_PublicErrorSanitization(t *testing.T) {
	sensitive := errors.New(`open /Users/me/.atlas/projects/x/memory.db password=super`)
	reg, err := hooks.NewRegistry([]hooks.Registration{{
		Action: stubAction{
			id: "fail", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				return hooks.Result{Message: sensitive.Error()}, sensitive
			},
		},
		FailurePolicy: hooks.FailAbort,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cap := capable(adapters.Cursor, hooks.EventSessionStart)
	eng, projectID := seedEngine(t, reg, &cap)
	resp := eng.Dispatch(context.Background(), validSessionRequest(projectID))
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), "/Users/") || strings.Contains(string(raw), "password") || strings.Contains(string(raw), "memory.db") {
		t.Fatalf("leaked: %s", raw)
	}
	if resp.Results[0].Message != "action failed" {
		t.Fatalf("message=%q", resp.Results[0].Message)
	}
}

func TestDispatch_RejectedInputNoFilesystemMutation(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "hooks-demo")
	beforeRoot := snapshotTree(t, root)
	beforeHome := snapshotTree(t, homePath)

	eng := hooksengine.Engine{
		Registry: hooksengine.DefaultRegistry(),
		Env:      hooksengine.Env{Root: root, Home: homePath},
	}
	req := validSessionRequest("wrong-id")
	req.Adapter = ""
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusInvalidAdapter {
		t.Fatalf("status=%q", resp.Status)
	}
	if snapshotTree(t, root) != beforeRoot {
		t.Fatal("project root mutated on rejected input")
	}
	if snapshotTree(t, homePath) != beforeHome {
		t.Fatal("ATLAS_HOME mutated on rejected input")
	}
}

func TestDefaultRegistryMaintenancePhaseA(t *testing.T) {
	reg := hooksengine.DefaultRegistry()
	if reg.Len() != 3 {
		t.Fatalf("DefaultRegistry len=%d want 3 maintenance Actions", reg.Len())
	}
	// Memory automatic injection Actions remain deferred.
	for _, regn := range reg.All() {
		id := regn.Action.ID()
		if strings.HasPrefix(id, "memory.") {
			t.Fatalf("production registry must not register Memory Actions, got %s", id)
		}
	}
	ids := map[string]bool{}
	for _, regn := range reg.All() {
		ids[regn.Action.ID()] = true
	}
	if !ids["context.economy.maintain"] || !ids["codeintel.refresh.maintain"] || !ids["context.postcode.maintain"] {
		t.Fatalf("missing maintenance ids: %v", ids)
	}
}

type stubBridge struct {
	id       adapters.ID
	declared []hooks.EventType
}

func (b stubBridge) AdapterID() adapters.ID            { return b.id }
func (b stubBridge) DeclaredEvents() []hooks.EventType { return b.declared }

func TestCapability_MissingBridgeFailClosed(t *testing.T) {
	eng := hooksengine.Engine{Registry: hooksengine.DefaultRegistry()}
	// Production LookupBridge starts empty (cli registers Cursor at process init).
	for _, id := range adapters.KnownIDs() {
		if _, ok := hooksengine.LookupBridge(id); ok {
			t.Fatalf("unexpected production bridge for %s in package tests", id)
		}
		cap := eng.CapabilityForTest(id)
		switch id {
		case adapters.Cursor:
			if !cap.HooksCapability {
				t.Fatal("cursor ClassHooks must be enabled")
			}
			if len(cap.SupportedEvents()) != 0 {
				t.Fatalf("cursor without bridge must advertise no events: %v", cap.SupportedEvents())
			}
		case adapters.OpenCode:
			if !cap.HooksCapability {
				t.Fatal("opencode ClassHooks must be enabled")
			}
			if len(cap.SupportedEvents()) != 0 {
				t.Fatalf("opencode without bridge must advertise no events: %v", cap.SupportedEvents())
			}
		}
	}
}

func TestCapability_PartialBridgeDeclarations(t *testing.T) {
	bridge := stubBridge{
		id:       adapters.Cursor,
		declared: []hooks.EventType{hooks.EventSessionStart, hooks.EventBeforeToolExecution},
	}
	// Cursor ClassHooks is true: only declared events are supported.
	cap := hooksengine.NegotiateAdapterForTest(adapters.Cursor, bridge)
	if !cap.HooksCapability || len(cap.SupportedEvents()) != 2 {
		t.Fatalf("cursor must honor bridge declarations: %#v", cap)
	}
	if !cap.Supports(hooks.EventSessionStart) || !cap.Supports(hooks.EventBeforeToolExecution) {
		t.Fatalf("declared events missing: %#v", cap)
	}
	if cap.Supports(hooks.EventTaskComplete) {
		t.Fatal("undeclared events must stay unsupported")
	}

	// OpenCode ClassHooks is true: declared events are honored.
	ocBridge := stubBridge{id: adapters.OpenCode, declared: []hooks.EventType{hooks.EventSessionStart}}
	cap = hooksengine.NegotiateAdapterForTest(adapters.OpenCode, ocBridge)
	if !cap.HooksCapability || !cap.Supports(hooks.EventSessionStart) {
		t.Fatalf("opencode must honor bridge declarations: %#v", cap)
	}

	// Mismatched bridge identity fails closed to empty declarations.
	bad := stubBridge{id: adapters.OpenCode, declared: hooks.KnownEventTypes()}
	cap = hooksengine.NegotiateAdapterForTest(adapters.Cursor, bad)
	if len(cap.SupportedEvents()) != 0 {
		t.Fatalf("mismatched bridge advertised %v", cap.SupportedEvents())
	}
	cap = hooksengine.NegotiateAdapterForTest(adapters.Cursor, nil)
	if len(cap.SupportedEvents()) != 0 {
		t.Fatalf("nil bridge advertised %v", cap.SupportedEvents())
	}

	// When Hooks is enabled (test-only override surface), only bridge-declared
	// events are supported — never the full canonical catalog by default.
	partial := capable(adapters.Cursor, hooks.EventSessionStart, hooks.EventBeforeToolExecution)
	eng, projectID := seedEngine(t, hooksengine.DefaultRegistry(), &partial)
	eng.BridgeLookup = func(id adapters.ID) (hooks.EventBridge, bool) {
		if id == adapters.Cursor {
			return bridge, true
		}
		return nil, false
	}
	// Override still wins for dispatch; unsupported undeclared event fails closed.
	req := hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID: "task-1", Type: string(hooks.EventTaskStart),
			OccurredAt: time.Now().UTC().Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: projectID, TaskID: "t1",
				TaskSummaryState: string(hooks.TaskSummaryAbsent),
			},
		},
	}
	resp := eng.Dispatch(context.Background(), req)
	if resp.Status != hooksengine.StatusUnsupportedEvent {
		t.Fatalf("undeclared event status=%q", resp.Status)
	}
	// Declared event succeeds (Phase A maintenance Actions skip when triggers disabled).
	resp = eng.Dispatch(context.Background(), validSessionRequest(projectID))
	if resp.Status != hooksengine.StatusSuccess {
		t.Fatalf("declared event status=%q", resp.Status)
	}
}

func TestCapability_BridgeLookupIsolatedFromOverride(t *testing.T) {
	bridge := stubBridge{id: adapters.Cursor, declared: []hooks.EventType{hooks.EventSessionStart}}
	override := capable(adapters.Cursor, hooks.EventTaskStart)
	eng := hooksengine.Engine{
		Registry:           hooksengine.DefaultRegistry(),
		CapabilityOverride: &override,
		BridgeLookup: func(id adapters.ID) (hooks.EventBridge, bool) {
			return bridge, true
		},
	}
	// Production negotiation path (no override) uses ClassHooks ∧ bridge declarations.
	prod := hooksengine.Engine{
		Registry: hooksengine.DefaultRegistry(),
		BridgeLookup: func(id adapters.ID) (hooks.EventBridge, bool) {
			return bridge, true
		},
	}.CapabilityForTest(adapters.Cursor)
	if !prod.Supports(hooks.EventSessionStart) {
		t.Fatalf("production path must support bridge-declared events when Hooks=true: %#v", prod)
	}
	if prod.Supports(hooks.EventTaskStart) {
		t.Fatalf("production path must not invent undeclared events: %#v", prod)
	}
	// Override path is test-only and does not consult bridge declarations.
	if !eng.CapabilityOverride.Supports(hooks.EventTaskStart) || eng.CapabilityOverride.Supports(hooks.EventSessionStart) {
		t.Fatalf("override isolation broken: %#v", eng.CapabilityOverride)
	}
}

func capable(id adapters.ID, events ...hooks.EventType) hooks.Capability {
	cap := hooks.Capability{
		Adapter:         id,
		HooksCapability: true,
		Events:          map[hooks.EventType]hooks.EventSupport{},
	}
	for _, et := range hooks.KnownEventTypes() {
		cap.Events[et] = hooks.SupportUnsupported
	}
	for _, et := range events {
		cap.Events[et] = hooks.SupportSupported
	}
	return cap
}

func seedEngine(t *testing.T, reg hooks.Registry, cap *hooks.Capability) (hooksengine.Engine, string) {
	t.Helper()
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "hooks-demo")
	projectID, err := home.ProjectID(root, "hooks-demo")
	if err != nil {
		t.Fatal(err)
	}
	return hooksengine.Engine{
		Registry:           reg,
		Env:                hooksengine.Env{Root: root, Home: homePath},
		CapabilityOverride: cap,
		BridgeLookup: func(id adapters.ID) (hooks.EventBridge, bool) {
			if id == adapters.Cursor {
				return cursorBridge{}, true
			}
			return nil, false
		},
	}, projectID
}

type cursorBridge struct{}

func (cursorBridge) AdapterID() adapters.ID { return adapters.Cursor }
func (cursorBridge) DeclaredEvents() []hooks.EventType {
	return []hooks.EventType{
		hooks.EventSessionStart,
		hooks.EventTaskStart,
		hooks.EventBeforeToolExecution,
		hooks.EventAfterToolExecution,
		hooks.EventBeforeContextCompaction,
	}
}

func validSessionRequest(projectID string) hooksengine.Request {
	return hooksengine.Request{
		SchemaVersion: 1,
		Adapter:       string(adapters.Cursor),
		Event: hooksengine.EventInput{
			ID:         "evt-1",
			Type:       string(hooks.EventSessionStart),
			OccurredAt: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			Payload: hooksengine.PayloadInput{
				ProjectID: projectID,
				SessionID: "sess-1",
			},
		},
	}
}

func writeProjectConfig(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig(name)
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotTree(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(&b, "%s %d %v\n", rel, info.Size(), info.Mode())
		return nil
	})
	return b.String()
}
