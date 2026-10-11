package hooks

import (
	"context"
	"errors"
	"fmt"
)

// DispatchReport is the deterministic public outcome of one event dispatch.
//
// Err is a classified terminal error (sentinel-wrapped). It never carries
// arbitrary Action error strings, secrets, or filesystem paths.
type DispatchReport struct {
	EventID   string
	EventType EventType
	Results   []Result
	Aborted   bool
	Err       error
}

// Dispatcher executes registered Actions for canonical events.
type Dispatcher struct {
	Registry   Registry
	Capability Capability
}

// Dispatch validates capability + event, then runs subscribed actions in
// deterministic action-ID order with per-action timeouts.
//
// Semantics:
//   - Non-blocking actions never abort the chain.
//   - Blocking + FailAbort aborts on action error or failure outcomes
//     (failed/timeout/cancelled), including unknown outcomes normalized to failed.
//   - Blocking with DenyContinuation (OK) or OutcomeDenied aborts with
//     ErrContinueDenied.
//   - Unsupported/disabled adapter events fail closed before any action runs.
//   - Result.ActionID is always the registered identity; Run-supplied IDs ignored.
//   - Result.Message is always a fixed public string; Run messages never propagate.
//
// Cancellation guarantees and limitations:
//   - Dispatch cancels the per-action context on timeout or parent cancellation.
//   - Well-behaved Actions should return promptly when ctx is done.
//   - Cancellation does NOT forcibly terminate goroutines. An Action that ignores
//     ctx may continue running after Dispatch has recorded timeout/cancelled and
//     moved on (or returned).
//   - Timed-out / cancelled Actions never block subsequent actions in this
//     Dispatch: the dispatcher does not wait for Run to return after deadline.
//   - Late completions from abandoned Runs are discarded and cannot mutate this
//     DispatchReport or Registry metadata.
//   - This foundation cannot prevent an abandoned Action from performing its own
//     external side effects (I/O outside Atlas). Process-level isolation would
//     require a different execution architecture and is explicitly out of scope.
func (d Dispatcher) Dispatch(ctx context.Context, ev Event) DispatchReport {
	report := DispatchReport{
		EventID:   ev.ID,
		EventType: ev.Type,
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := RequireSupported(d.Capability, ev); err != nil {
		report.Err = publicDispatchErr(err)
		return report
	}

	regs := d.Registry.ForEvent(ev.Type)
	report.Results = make([]Result, 0, len(regs))

	for _, reg := range regs {
		if err := ctx.Err(); err != nil {
			report.Results = append(report.Results, Result{
				ActionID: reg.Action.ID(),
				Outcome:  OutcomeCancelled,
				Message:  publicOutcomeMessage(OutcomeCancelled),
			})
			report.Aborted = true
			report.Err = ErrActionCancelled
			return report
		}

		res, runErr := runAction(ctx, reg, ev)
		report.Results = append(report.Results, res)

		if abort, terminal := shouldAbort(reg, res, runErr); abort {
			report.Aborted = true
			report.Err = terminal
			return report
		}
	}
	return report
}

func runAction(ctx context.Context, reg Registration, ev Event) (Result, error) {
	id := reg.Action.ID()
	timeout := reg.timeout()
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type outcome struct {
		res Result
		err error
	}
	// Buffered so an abandoned Run can finish without blocking forever if the
	// dispatcher has already moved on.
	ch := make(chan outcome, 1)
	go func() {
		res, err := reg.Action.Run(actx, ev)
		// Buffer size 1: if Dispatch already timed out/cancelled, this send
		// completes without blocking and the value is discarded with the channel.
		ch <- outcome{res: res, err: err}
	}()

	select {
	case out := <-ch:
		res := normalizeResult(id, reg, out.res, out.err)
		if out.err != nil {
			return res, classifyRunError(out.err)
		}
		return res, nil
	case <-actx.Done():
		// Do not wait for Run. Context is cancelled via defer; the goroutine may
		// still be running if the Action ignores cancellation.
		err := actx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			return Result{
				ActionID: id,
				Outcome:  OutcomeTimeout,
				Message:  publicOutcomeMessage(OutcomeTimeout),
			}, ErrActionTimeout
		}
		return Result{
			ActionID: id,
			Outcome:  OutcomeCancelled,
			Message:  publicOutcomeMessage(OutcomeCancelled),
		}, ErrActionCancelled
	}
}

func normalizeResult(id string, reg Registration, res Result, runErr error) Result {
	// Registered identity always wins — never trust Action.Run ActionID.
	out := Result{ActionID: id}

	if runErr != nil {
		switch {
		case errors.Is(runErr, context.DeadlineExceeded), errors.Is(runErr, ErrActionTimeout):
			out.Outcome = OutcomeTimeout
		case errors.Is(runErr, context.Canceled), errors.Is(runErr, ErrActionCancelled):
			out.Outcome = OutcomeCancelled
		default:
			out.Outcome = OutcomeFailed
		}
		out.DenyContinuation = false
		out.Message = publicOutcomeMessage(out.Outcome)
		return out
	}

	out.Outcome = canonicalOutcome(res.Outcome)
	out.DenyContinuation = false
	if reg.Action.Mode() == ModeBlocking {
		switch out.Outcome {
		case OutcomeDenied:
			// Canonical deny — always aborts blocking dispatch.
			out.DenyContinuation = true
		case OutcomeOK:
			out.DenyContinuation = res.DenyContinuation
		default:
			// failed/timeout/cancelled/skipped/unknown→failed cannot use
			// DenyContinuation to impersonate deny semantics.
			out.DenyContinuation = false
		}
	}
	out.Message = publicOutcomeMessage(out.Outcome)
	return out
}

// canonicalOutcome maps empty → OK and unknown → failed.
func canonicalOutcome(o Outcome) Outcome {
	if o == "" {
		return OutcomeOK
	}
	if isCanonicalOutcome(o) {
		return o
	}
	return OutcomeFailed
}

func isCanonicalOutcome(o Outcome) bool {
	switch o {
	case OutcomeOK, OutcomeSkipped, OutcomeFailed, OutcomeTimeout, OutcomeCancelled, OutcomeDenied:
		return true
	default:
		return false
	}
}

// publicOutcomeMessage returns fixed predefined public strings only.
// Action.Run messages are never accepted or echoed.
func publicOutcomeMessage(outcome Outcome) string {
	switch outcome {
	case OutcomeOK:
		return "action ok"
	case OutcomeDenied:
		return "action denied continuation"
	case OutcomeTimeout:
		return "action timed out"
	case OutcomeCancelled:
		return "action cancelled"
	case OutcomeSkipped:
		return "action skipped"
	case OutcomeFailed:
		return "action failed"
	default:
		return "action failed"
	}
}

func shouldAbort(reg Registration, res Result, runErr error) (bool, error) {
	if reg.Action.Mode() == ModeNonBlocking {
		return false, nil
	}
	if res.DenyContinuation || res.Outcome == OutcomeDenied {
		return true, fmt.Errorf("%w: action %s", ErrContinueDenied, reg.Action.ID())
	}
	if reg.policy() == FailAbort && (runErr != nil || isAbortableFailure(res.Outcome)) {
		return true, publicAbortErr(reg.Action.ID(), runErr)
	}
	return false, nil
}

func isAbortableFailure(o Outcome) bool {
	switch o {
	case OutcomeFailed, OutcomeTimeout, OutcomeCancelled:
		return true
	default:
		return false
	}
}

func publicAbortErr(actionID string, runErr error) error {
	if runErr == nil {
		return fmt.Errorf("%w: action %s", ErrDispatchAborted, actionID)
	}
	classified := classifyRunError(runErr)
	switch {
	case errors.Is(classified, ErrActionTimeout):
		return fmt.Errorf("%w: action %s: %w", ErrDispatchAborted, actionID, ErrActionTimeout)
	case errors.Is(classified, ErrActionCancelled):
		return fmt.Errorf("%w: action %s: %w", ErrDispatchAborted, actionID, ErrActionCancelled)
	default:
		// Do not wrap arbitrary Action errors into the public report.
		return fmt.Errorf("%w: action %s", ErrDispatchAborted, actionID)
	}
}

func publicDispatchErr(err error) error {
	switch {
	case errors.Is(err, ErrHooksDisabled):
		return ErrHooksDisabled
	case errors.Is(err, ErrUnsupportedEvent):
		return ErrUnsupportedEvent
	case errors.Is(err, ErrUnknownEventType):
		return ErrUnknownEventType
	case errors.Is(err, ErrInvalidEvent):
		return ErrInvalidEvent
	default:
		return ErrInvalidEvent
	}
}

func classifyRunError(err error) error {
	switch {
	case errors.Is(err, ErrActionTimeout), errors.Is(err, context.DeadlineExceeded):
		return ErrActionTimeout
	case errors.Is(err, ErrActionCancelled), errors.Is(err, context.Canceled):
		return ErrActionCancelled
	default:
		return err
	}
}

// Classify returns the most specific public sentinel for err, or err unchanged
// when it is already a hooks sentinel. Arbitrary Action errors are mapped to
// ErrDispatchAborted so callers do not propagate internal text via Classify.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrUnknownEventType):
		return ErrUnknownEventType
	case errors.Is(err, ErrUnsupportedEvent):
		return ErrUnsupportedEvent
	case errors.Is(err, ErrInvalidEvent):
		return ErrInvalidEvent
	case errors.Is(err, ErrInvalidAction):
		return ErrInvalidAction
	case errors.Is(err, ErrDuplicateAction):
		return ErrDuplicateAction
	case errors.Is(err, ErrHooksDisabled):
		return ErrHooksDisabled
	case errors.Is(err, ErrActionTimeout):
		return ErrActionTimeout
	case errors.Is(err, ErrActionCancelled):
		return ErrActionCancelled
	case errors.Is(err, ErrDispatchAborted):
		return ErrDispatchAborted
	case errors.Is(err, ErrOwnershipConflict):
		return ErrOwnershipConflict
	case errors.Is(err, ErrContinueDenied):
		return ErrContinueDenied
	default:
		return ErrDispatchAborted
	}
}
