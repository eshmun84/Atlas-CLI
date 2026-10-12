package hooks_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
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

type stubBridge struct {
	id       adapters.ID
	declared []hooks.EventType
}

func (b stubBridge) AdapterID() adapters.ID            { return b.id }
func (b stubBridge) DeclaredEvents() []hooks.EventType { return b.declared }

func validSessionEvent() hooks.Event {
	return hooks.Event{
		ID:         "evt-1",
		Type:       hooks.EventSessionStart,
		OccurredAt: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
		Adapter:    adapters.Cursor,
		Payload: hooks.Payload{
			ProjectID: "demo",
			SessionID: "sess-1",
		},
	}
}

func validTaskEvent(summaryState hooks.TaskSummaryState, summary string) hooks.Event {
	return hooks.Event{
		ID:         "task-evt-1",
		Type:       hooks.EventTaskStart,
		OccurredAt: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
		Adapter:    adapters.Cursor,
		Payload: hooks.Payload{
			ProjectID:        "demo",
			TaskID:           "task-1",
			TaskSummaryState: summaryState,
			TaskSummary:      summary,
		},
	}
}

func TestKnownEventCatalogStable(t *testing.T) {
	a := hooks.KnownEventTypes()
	b := hooks.KnownEventTypes()
	if len(a) != 7 {
		t.Fatalf("catalog size=%d want 7", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic catalog: %v vs %v", a, b)
		}
	}
}

func TestValidateEvent_RejectsUnknownAndBoundaries(t *testing.T) {
	ev := validSessionEvent()
	ev.Type = hooks.EventType("memory.write")
	err := hooks.ValidateEvent(ev)
	if err == nil || !errors.Is(err, hooks.ErrInvalidEvent) {
		t.Fatalf("unknown type: got %v", err)
	}
	if !errors.Is(err, hooks.ErrUnknownEventType) {
		t.Fatalf("want ErrUnknownEventType wrap: %v", err)
	}

	ev = validSessionEvent()
	ev.ID = "../escape"
	if err := hooks.ValidateEvent(ev); err == nil || !errors.Is(err, hooks.ErrInvalidEvent) {
		t.Fatalf("path id: got %v", err)
	}

	ev = validSessionEvent()
	ev.Payload.SessionID = ""
	if err := hooks.ValidateEvent(ev); err == nil {
		t.Fatal("expected session_id required")
	}

	tool := hooks.Event{
		ID:         "evt-tool",
		Type:       hooks.EventBeforeToolExecution,
		OccurredAt: time.Now().UTC(),
		Payload:    hooks.Payload{ToolName: ""},
	}
	if err := hooks.ValidateEvent(tool); err == nil {
		t.Fatal("expected tool_name required")
	}
}

func TestValidateEvent_TaskSummaryContract(t *testing.T) {
	if err := hooks.ValidateEvent(validTaskEvent(hooks.TaskSummaryAbsent, "")); err != nil {
		t.Fatalf("absent ok: %v", err)
	}
	if err := hooks.ValidateEvent(validTaskEvent(hooks.TaskSummaryPresent, "Implement hooks hardening")); err != nil {
		t.Fatalf("present ok: %v", err)
	}

	// Missing explicit state — must not fabricate presence.
	if err := hooks.ValidateEvent(validTaskEvent(hooks.TaskSummaryUnspecified, "")); err == nil {
		t.Fatal("unspecified state must fail")
	}
	// Present requires text.
	if err := hooks.ValidateEvent(validTaskEvent(hooks.TaskSummaryPresent, "")); err == nil {
		t.Fatal("present without summary must fail")
	}
	// Absent forbids text.
	if err := hooks.ValidateEvent(validTaskEvent(hooks.TaskSummaryAbsent, "nope")); err == nil {
		t.Fatal("absent with summary must fail")
	}
	// task_id remains mandatory.
	ev := validTaskEvent(hooks.TaskSummaryAbsent, "")
	ev.Payload.TaskID = ""
	if err := hooks.ValidateEvent(ev); err == nil {
		t.Fatal("task_id required")
	}
	// Control characters / oversize rejected.
	if err := hooks.ValidateEvent(validTaskEvent(hooks.TaskSummaryPresent, "bad\x00null")); err == nil {
		t.Fatal("control char must fail")
	}
	long := strings.Repeat("x", 300)
	if err := hooks.ValidateEvent(validTaskEvent(hooks.TaskSummaryPresent, long)); err == nil {
		t.Fatal("oversize must fail")
	}
	// Non-task events cannot carry summary.
	sess := validSessionEvent()
	sess.Payload.TaskSummaryState = hooks.TaskSummaryAbsent
	if err := hooks.ValidateEvent(sess); err == nil {
		t.Fatal("session event must reject task_summary_state")
	}
}

func TestNegotiate_OpenCodeAndCursorHooksEnabled(t *testing.T) {
	if !adapters.Supports(adapters.Cursor, adapters.ClassHooks) {
		t.Fatal("cursor must support hooks")
	}
	if !adapters.Supports(adapters.OpenCode, adapters.ClassHooks) {
		t.Fatal("opencode must support hooks")
	}
	cap := hooks.Negotiate(adapters.OpenCode, []hooks.EventType{hooks.EventSessionStart})
	if !cap.HooksCapability || !cap.Supports(hooks.EventSessionStart) {
		t.Fatalf("opencode capability missing: %#v", cap)
	}
	cap = hooks.Negotiate(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart})
	if !cap.HooksCapability || !cap.Supports(hooks.EventSessionStart) {
		t.Fatalf("cursor capability missing: %#v", cap)
	}
}

func TestBuildCapabilityViaBridge_WhenCapable(t *testing.T) {
	cap := hooks.NegotiateBridge(nil)
	if cap.HooksCapability {
		t.Fatal("nil bridge must be disabled")
	}
	cap = hooks.BuildCapabilityForTest(adapters.Cursor, true, []hooks.EventType{
		hooks.EventSessionStart,
		hooks.EventBeforeToolExecution,
		hooks.EventType("not.a.real.event"),
	})
	got := cap.SupportedEvents()
	if len(got) != 2 || got[0] != hooks.EventSessionStart || got[1] != hooks.EventBeforeToolExecution {
		t.Fatalf("supported=%v", got)
	}
}

func hooksCapabilityForTest(id adapters.ID, declared []hooks.EventType) hooks.Capability {
	return hooks.BuildCapabilityForTest(id, true, declared)
}

func TestRegistry_DuplicateActionConflict(t *testing.T) {
	a := stubAction{id: "memory.recall", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking}
	_, err := hooks.NewRegistry([]hooks.Registration{
		{Action: a},
		{Action: a, FailurePolicy: hooks.FailAbort},
	})
	if err == nil || !errors.Is(err, hooks.ErrDuplicateAction) {
		t.Fatalf("got %v", err)
	}
}

func TestRegistry_SnapshotsMetadataAgainstCallerMutation(t *testing.T) {
	events := []hooks.EventType{hooks.EventSessionStart}
	regs := []hooks.Registration{
		{
			Action:        stubAction{id: "a", events: events, mode: hooks.ModeNonBlocking},
			FailurePolicy: hooks.FailContinue,
			Timeout:       100 * time.Millisecond,
		},
	}
	reg, err := hooks.NewRegistry(regs)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate caller-owned slice and registration metadata after construction.
	events[0] = hooks.EventTaskStart
	regs[0].FailurePolicy = hooks.FailAbort
	regs[0].Timeout = time.Hour

	got := reg.ForEvent(hooks.EventSessionStart)
	if len(got) != 1 {
		t.Fatalf("session subscription lost after caller mutation: %v", got)
	}
	if len(reg.ForEvent(hooks.EventTaskStart)) != 0 {
		t.Fatal("mutated caller events slice must not reshape registry")
	}
	all := reg.All()
	if all[0].FailurePolicy != hooks.FailContinue || all[0].Timeout != 100*time.Millisecond {
		t.Fatalf("registry metadata mutated: %#v", all[0])
	}

	// Defensive copy: mutating All() Events must not affect registry.
	allEvents := all[0].Action.Events()
	allEvents[0] = hooks.EventTaskComplete
	if reg.ForEvent(hooks.EventSessionStart)[0].Action.Events()[0] != hooks.EventSessionStart {
		t.Fatal("All() Events slice must be caller-owned copy")
	}
}

func TestRegistry_DeterministicOrder(t *testing.T) {
	regs := []hooks.Registration{
		{Action: stubAction{id: "z-last", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking}},
		{Action: stubAction{id: "a-first", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking}},
		{Action: stubAction{id: "m-mid", events: []hooks.EventType{hooks.EventTaskStart}, mode: hooks.ModeBlocking}},
	}
	reg, err := hooks.NewRegistry(regs)
	if err != nil {
		t.Fatal(err)
	}
	all := reg.All()
	if all[0].Action.ID() != "a-first" || all[1].Action.ID() != "m-mid" || all[2].Action.ID() != "z-last" {
		t.Fatalf("order=%v", []string{all[0].Action.ID(), all[1].Action.ID(), all[2].Action.ID()})
	}
}

func TestDispatch_FailClosedWhenHooksDisabled(t *testing.T) {
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{id: "a", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Unknown adapter IDs never advertise ClassHooks.
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooks.Negotiate(adapters.ID("claude"), hooks.KnownEventTypes()),
	}
	ev := validSessionEvent()
	ev.Adapter = adapters.ID("claude")
	rep := d.Dispatch(context.Background(), ev)
	if rep.Err == nil || !errors.Is(rep.Err, hooks.ErrHooksDisabled) {
		t.Fatalf("got %#v err=%v", rep, rep.Err)
	}
}

func TestDispatch_DeterministicOrderAndContinueOnNonBlockingFailure(t *testing.T) {
	var order []string
	regs := []hooks.Registration{
		{Action: stubAction{
			id: "b-second", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				order = append(order, "b-second")
				return hooks.Result{}, fmt.Errorf("secret=/Users/me/.atlas/token boom")
			},
		}},
		{Action: stubAction{
			id: "a-first", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				order = append(order, "a-first")
				return hooks.Result{Outcome: hooks.OutcomeOK}, nil
			},
		}},
	}
	reg, err := hooks.NewRegistry(regs)
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}
	rep := d.Dispatch(context.Background(), validSessionEvent())
	if rep.Aborted || rep.Err != nil {
		t.Fatalf("non-blocking must not abort: %#v", rep)
	}
	if len(order) != 2 || order[0] != "a-first" || order[1] != "b-second" {
		t.Fatalf("order=%v", order)
	}
	if rep.Results[1].Outcome != hooks.OutcomeFailed || rep.Results[1].Message != "action failed" {
		t.Fatalf("public failure message leaked: %#v", rep.Results[1])
	}
	if strings.Contains(rep.Results[1].Message, "/Users/") || strings.Contains(rep.Results[1].Message, "token") {
		t.Fatal("sensitive error text exposed in Result.Message")
	}
}

func TestDispatch_PublicErrorsHideInternalText(t *testing.T) {
	sensitive := errors.New(`open /Users/me/.atlas/projects/x/memory/memory.db: password=super-secret`)
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{
			Action: stubAction{
				id: "fail", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeBlocking,
				run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
					return hooks.Result{Message: sensitive.Error()}, sensitive
				},
			},
			FailurePolicy: hooks.FailAbort,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}
	rep := d.Dispatch(context.Background(), validSessionEvent())
	if !rep.Aborted || !errors.Is(rep.Err, hooks.ErrDispatchAborted) {
		t.Fatalf("want abort: %#v", rep)
	}
	if errors.Is(rep.Err, sensitive) {
		t.Fatal("public Err must not unwrap to internal action error")
	}
	errText := rep.Err.Error()
	if strings.Contains(errText, "/Users/") || strings.Contains(errText, "password") || strings.Contains(errText, "memory.db") {
		t.Fatalf("public Err leaked internals: %q", errText)
	}
	if rep.Results[0].Message != "action failed" {
		t.Fatalf("Result.Message=%q", rep.Results[0].Message)
	}
	if hooks.Classify(sensitive) != hooks.ErrDispatchAborted {
		t.Fatalf("Classify(arbitrary)=%v", hooks.Classify(sensitive))
	}
}

func TestDispatch_BlockingAbortAndDeny(t *testing.T) {
	var ran int32
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{
			id: "deny", events: []hooks.EventType{hooks.EventBeforeToolExecution}, mode: hooks.ModeBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				return hooks.Result{Outcome: hooks.OutcomeDenied, Message: "blocked"}, nil
			},
		}},
		{Action: stubAction{
			id: "later", events: []hooks.EventType{hooks.EventBeforeToolExecution}, mode: hooks.ModeNonBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				atomic.AddInt32(&ran, 1)
				return hooks.Result{}, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventBeforeToolExecution}),
	}
	ev := hooks.Event{
		ID:         "t1",
		Type:       hooks.EventBeforeToolExecution,
		OccurredAt: time.Now().UTC(),
		Adapter:    adapters.Cursor,
		Payload:    hooks.Payload{ToolName: "Shell"},
	}
	rep := d.Dispatch(context.Background(), ev)
	if !rep.Aborted || !errors.Is(rep.Err, hooks.ErrContinueDenied) {
		t.Fatalf("want deny abort: %#v", rep)
	}
	if rep.Results[0].Message != "action denied continuation" {
		t.Fatalf("deny message=%q", rep.Results[0].Message)
	}
	if atomic.LoadInt32(&ran) != 0 {
		t.Fatal("later action must not run after deny")
	}
}

func TestDispatch_RegisteredActionIDAlwaysWins(t *testing.T) {
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{
			id: "registered-id", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				return hooks.Result{
					ActionID: "spoofed-id",
					Outcome:  hooks.OutcomeOK,
					Message:  "should-not-appear",
				}, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}
	rep := d.Dispatch(context.Background(), validSessionEvent())
	if len(rep.Results) != 1 || rep.Results[0].ActionID != "registered-id" {
		t.Fatalf("ActionID not forced to registered identity: %#v", rep.Results)
	}
	if rep.Results[0].Message != "action ok" {
		t.Fatalf("Message=%q", rep.Results[0].Message)
	}
}

func TestDispatch_UnknownOutcomeNormalizedAndFailAbort(t *testing.T) {
	var laterRan atomic.Bool
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{
			Action: stubAction{
				id: "a-weird", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeBlocking,
				run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
					return hooks.Result{
						Outcome:          hooks.Outcome("totally-unknown"),
						DenyContinuation: true, // must not become deny after normalize
						Message:          "password=super-secret path=/Users/me/.atlas",
					}, nil
				},
			},
			FailurePolicy: hooks.FailAbort,
		},
		{Action: stubAction{
			id: "z-later", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				laterRan.Store(true)
				return hooks.Result{}, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}
	rep := d.Dispatch(context.Background(), validSessionEvent())
	if !rep.Aborted || !errors.Is(rep.Err, hooks.ErrDispatchAborted) {
		t.Fatalf("unknown outcome must FailAbort: %#v", rep)
	}
	if errors.Is(rep.Err, hooks.ErrContinueDenied) {
		t.Fatal("unknown outcome must not bypass into deny semantics")
	}
	if len(rep.Results) != 1 {
		t.Fatalf("must stop after abort: %#v", rep.Results)
	}
	if rep.Results[0].ActionID != "a-weird" || rep.Results[0].Outcome != hooks.OutcomeFailed || rep.Results[0].Message != "action failed" {
		t.Fatalf("normalized result=%#v", rep.Results[0])
	}
	if rep.Results[0].DenyContinuation {
		t.Fatal("DenyContinuation must be cleared for unknown→failed")
	}
	if laterRan.Load() {
		t.Fatal("later action must not run after FailAbort")
	}
	if strings.Contains(rep.Results[0].Message, "password") || strings.Contains(rep.Err.Error(), "/Users/") {
		t.Fatal("sensitive strings leaked")
	}
}

func TestDispatch_FixedPublicMessagesOnly(t *testing.T) {
	sensitive := "token=abc open /Users/me/.atlas/memory.db"
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{
			id: "ok", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				return hooks.Result{Outcome: hooks.OutcomeOK, Message: sensitive}, nil
			},
		}},
		{Action: stubAction{
			id: "deny", events: []hooks.EventType{hooks.EventBeforeToolExecution}, mode: hooks.ModeBlocking,
			run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
				return hooks.Result{Outcome: hooks.OutcomeDenied, Message: sensitive}, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry: reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{
			hooks.EventSessionStart, hooks.EventBeforeToolExecution,
		}),
	}
	rep := d.Dispatch(context.Background(), validSessionEvent())
	if rep.Results[0].Message != "action ok" || strings.Contains(rep.Results[0].Message, "token") {
		t.Fatalf("ok message=%q", rep.Results[0].Message)
	}
	toolEv := hooks.Event{
		ID: "t1", Type: hooks.EventBeforeToolExecution, OccurredAt: time.Now().UTC(),
		Adapter: adapters.Cursor, Payload: hooks.Payload{ToolName: "Shell"},
	}
	rep = d.Dispatch(context.Background(), toolEv)
	if rep.Results[0].Message != "action denied continuation" {
		t.Fatalf("deny message=%q", rep.Results[0].Message)
	}
	if strings.Contains(rep.Results[0].Message, sensitive) || strings.Contains(fmt.Sprintf("%v", rep.Err), "token") {
		t.Fatal("sensitive action message propagated")
	}
}

func TestDispatch_Timeout(t *testing.T) {
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{
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
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}
	rep := d.Dispatch(context.Background(), validSessionEvent())
	if !rep.Aborted || !errors.Is(rep.Err, hooks.ErrDispatchAborted) || !errors.Is(rep.Err, hooks.ErrActionTimeout) {
		t.Fatalf("want abort+timeout: %#v", rep)
	}
	if rep.Results[0].Outcome != hooks.OutcomeTimeout || rep.Results[0].Message != "action timed out" {
		t.Fatalf("results=%#v", rep.Results)
	}
}

func TestDispatch_IgnoresContext_TimeoutDoesNotBlockSubsequent(t *testing.T) {
	var secondRan atomic.Bool
	releaseLate := make(chan struct{})

	reg, err := hooks.NewRegistry([]hooks.Registration{
		{
			Action: stubAction{
				id: "a-ignore-cancel", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking,
				run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
					// Ignore ctx entirely — simulates a misbehaved action.
					time.Sleep(80 * time.Millisecond)
					<-releaseLate
					return hooks.Result{
						Outcome: hooks.OutcomeOK,
						Message: "late-success-/Users/me/secret",
					}, nil
				},
			},
			Timeout: 15 * time.Millisecond,
		},
		{
			Action: stubAction{
				id: "b-next", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking,
				run: func(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
					secondRan.Store(true)
					return hooks.Result{Outcome: hooks.OutcomeOK}, nil
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}

	start := time.Now()
	rep := d.Dispatch(context.Background(), validSessionEvent())
	elapsed := time.Since(start)
	if elapsed > 60*time.Millisecond {
		t.Fatalf("dispatch blocked on abandoned action: elapsed=%s", elapsed)
	}
	if !secondRan.Load() {
		t.Fatal("subsequent action must run after timeout")
	}
	if len(rep.Results) != 2 {
		t.Fatalf("results=%#v", rep.Results)
	}
	if rep.Results[0].Outcome != hooks.OutcomeTimeout || rep.Results[0].ActionID != "a-ignore-cancel" {
		t.Fatalf("first result=%#v", rep.Results[0])
	}
	if rep.Results[1].Outcome != hooks.OutcomeOK || rep.Results[1].ActionID != "b-next" {
		t.Fatalf("second result=%#v", rep.Results[1])
	}
	// Snapshot public report fields, then let the abandoned action finish.
	firstOutcome := rep.Results[0].Outcome
	firstMessage := rep.Results[0].Message
	close(releaseLate)
	time.Sleep(120 * time.Millisecond)
	if rep.Results[0].Outcome != firstOutcome || rep.Results[0].Outcome != hooks.OutcomeTimeout {
		t.Fatalf("late completion mutated report: %#v", rep.Results[0])
	}
	if firstMessage != "action timed out" || strings.Contains(rep.Results[0].Message, "late-success") {
		t.Fatalf("late message leaked into report: %q", rep.Results[0].Message)
	}
}

func TestDispatch_ParentCancel(t *testing.T) {
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{id: "a", events: []hooks.EventType{hooks.EventSessionStart}, mode: hooks.ModeNonBlocking}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}
	rep := d.Dispatch(ctx, validSessionEvent())
	if !rep.Aborted || !errors.Is(rep.Err, hooks.ErrActionCancelled) {
		t.Fatalf("got %#v", rep)
	}
	if rep.Results[0].Message != "action cancelled" {
		t.Fatalf("message=%q", rep.Results[0].Message)
	}
}

func TestDispatch_UnsupportedEvent(t *testing.T) {
	reg, err := hooks.NewRegistry([]hooks.Registration{
		{Action: stubAction{id: "a", events: []hooks.EventType{hooks.EventTaskStart}, mode: hooks.ModeNonBlocking}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := hooks.Dispatcher{
		Registry:   reg,
		Capability: hooksCapabilityForTest(adapters.Cursor, []hooks.EventType{hooks.EventSessionStart}),
	}
	ev := validTaskEvent(hooks.TaskSummaryAbsent, "")
	rep := d.Dispatch(context.Background(), ev)
	if !errors.Is(rep.Err, hooks.ErrUnsupportedEvent) {
		t.Fatalf("got %v", rep.Err)
	}
}

func TestOwnershipConflictError(t *testing.T) {
	err := hooks.ConflictError(adapters.Cursor, ".cursor/hooks.json")
	if !errors.Is(err, hooks.ErrOwnershipConflict) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRegistration(t *testing.T) {
	_, err := hooks.NewRegistry([]hooks.Registration{{Action: nil}})
	if err == nil || !errors.Is(err, hooks.ErrInvalidAction) {
		t.Fatalf("got %v", err)
	}
}
