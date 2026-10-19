package hookactions

import (
	"context"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// MaybeAdmitCursorSessionFallback is the Phase B.8 Cursor-only maintenance
// evaluation point for beforeSubmitPrompt → task.start.
//
// Called from hooksengine after Action dispatch so it never alters governance,
// denials, security rules, or adapter JSON. Fail-open: missing/invalid session
// identity skips admission without inventing an ID.
//
// Does not subscribe maintenance Actions to task.start and does not change
// OpenCode or other adapters.
func MaybeAdmitCursorSessionFallback(ctx context.Context, adapter adapters.ID, ev hooks.Event) {
	if adapter != adapters.Cursor {
		return
	}
	if ev.Type != hooks.EventTaskStart {
		return
	}
	if !TriggersAreEnabled() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Bound admission so the hook process never waits on worker work.
	admitCtx, cancel := context.WithTimeout(ctx, AdmissionTimeout)
	defer cancel()
	_, _ = AdmitCursorPromptFallback(admitCtx, ev, nil)
}

// AdmitCursorPromptFallback admits Home maintenance for a Cursor task.start
// when the session has not yet been successfully admitted (or recovered after
// failure). Shared admission path with session.start — not a second trigger
// semantics for task.start Actions.
func AdmitCursorPromptFallback(ctx context.Context, ev hooks.Event, resolve ResolveFunc) (hooks.Result, error) {
	if ev.Type != hooks.EventTaskStart {
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	if !ValidSessionID(ev.Payload.SessionID) {
		// Fail-open: no identity → no unlimited maintenance.
		return hooks.Result{Outcome: hooks.OutcomeSkipped}, nil
	}
	return admitSessionMaintenance(ctx, ev, resolve, admitSourceCursorFallback)
}
