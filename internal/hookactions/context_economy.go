package hookactions

import (
	"context"
	"fmt"
	"strings"
	"time"

	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// Stable Action identity for Context Economy maintenance.
const ActionIDContextEconomy = "context.economy.maintain"

// ContextEconomyTimeout bounds one Context Economy maintenance Run.
const ContextEconomyTimeout = 45 * time.Second

// ContextEconomyAction updates Context Economy payloads under Atlas Home when
// missing or stale. It reuses atlascontext.ApplyUpdate and never writes large
// context into the product repository.
type ContextEconomyAction struct {
	// Resolve supplies workspace identity. nil → DefaultResolve.
	Resolve ResolveFunc
	// Now overrides the clock for ApplyUpdate. nil → time.Now.
	Now func() time.Time
	// Objective overrides the pack objective. Empty → DefaultPackObjective.
	Objective string
}

// Ensure ContextEconomyAction satisfies hooks.Action.
var _ hooks.Action = (*ContextEconomyAction)(nil)

func (a *ContextEconomyAction) ID() string { return ActionIDContextEconomy }

// Events lists production session subscriptions.
// Only session.start: Cursor sessionStart and OpenCode session.created.
// Cursor beforeSubmitPrompt fallback is evaluated in hooksengine (Phase B.8),
// not via task.start subscription. session.resume is NOT subscribed.
func (a *ContextEconomyAction) Events() []hooks.EventType {
	return []hooks.EventType{
		hooks.EventSessionStart,
	}
}

func (a *ContextEconomyAction) Mode() hooks.ExecutionMode {
	return hooks.ModeNonBlocking
}

// Run admits session maintenance (fast spawn / dedupe). Inline Maintain remains
// available via Maintain / MaintainContextEconomy for tests and workers.
func (a *ContextEconomyAction) Run(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
	return AdmitSessionMaintenance(ctx, ev, a.Resolve)
}

// ContextEconomyResult is the structured outcome of one maintenance attempt.
type ContextEconomyResult struct {
	Outcome   hooks.Outcome
	Noop      bool
	State     string
	ProjectID string
	HomePath  string
	Created   []string
	Replaced  []string
	Message   string
}

// MaintainContextEconomy is the package-level entry for direct invocation
// (tests / future controlled callers). Ignores TriggersEnabled.
func MaintainContextEconomy(ctx context.Context, root, homePath string) (ContextEconomyResult, error) {
	ref, err := ResolveAt(root, homePath)
	if err != nil {
		return ContextEconomyResult{Outcome: hooks.OutcomeFailed}, err
	}
	return (&ContextEconomyAction{}).Maintain(ctx, ref)
}

// Maintain inspects Context Economy and applies Home updates when needed.
// Fresh → no-op. Unreadable → controlled error (no rewrite). Ignores TriggersEnabled.
func (a *ContextEconomyAction) Maintain(ctx context.Context, ref ProjectRef) (ContextEconomyResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ContextEconomyResult{Outcome: hooks.OutcomeCancelled, ProjectID: ref.ProjectID, HomePath: ref.Home}, err
	}

	out := ContextEconomyResult{
		ProjectID: ref.ProjectID,
		HomePath:  ref.Home,
	}

	snap := atlascontext.InspectFromState(ref.Root, ref.State.Initialized, ref.State)
	out.State = snap.State

	switch snap.State {
	case atlascontext.StatusPresent:
		out.Outcome = hooks.OutcomeOK
		out.Noop = true
		out.Message = "context economy already fresh"
		return out, nil
	case atlascontext.StatusUnreadable:
		out.Outcome = hooks.OutcomeFailed
		out.Message = "context economy unreadable"
		return out, fmt.Errorf("hookactions: context economy unreadable")
	case atlascontext.StatusMissing, atlascontext.StatusNA, atlascontext.StatusStale:
		// proceed to ApplyUpdate
	default:
		out.Outcome = hooks.OutcomeFailed
		out.Message = "context economy state unknown"
		return out, fmt.Errorf("hookactions: context economy state %q", snap.State)
	}

	if err := ctx.Err(); err != nil {
		out.Outcome = hooks.OutcomeCancelled
		return out, err
	}

	objective := strings.TrimSpace(a.Objective)
	if objective == "" {
		objective = atlascontext.DefaultPackObjective
	}
	plan := atlascontext.BuildUpdatePlan(ref.Root, ref.State.Initialized, ref.State, objective)
	if plan.Blocked {
		out.Outcome = hooks.OutcomeFailed
		out.Message = strings.Join(plan.Blockers, "; ")
		return out, fmt.Errorf("hookactions: context economy blocked: %s", out.Message)
	}

	result, err := atlascontext.ApplyUpdate(ref.Root, plan.Signature(), ref.State, objective, a.Now)
	if err != nil {
		out.Outcome = hooks.OutcomeFailed
		out.Message = err.Error()
		return out, err
	}
	if result.Stale {
		out.Outcome = hooks.OutcomeFailed
		out.Message = atlascontext.UpdateStaleMessage
		return out, fmt.Errorf("hookactions: %s", atlascontext.UpdateStaleMessage)
	}
	out.Created = append(out.Created, result.Created...)
	out.Replaced = append(out.Replaced, result.Replaced...)
	out.Noop = result.Noop
	out.Outcome = hooks.OutcomeOK
	if result.Noop {
		out.Message = atlascontext.UpdateNoopTitle
	} else {
		out.Message = atlascontext.UpdateSuccessTitle
	}
	// Re-inspect for honest post-state (best effort).
	after := atlascontext.InspectFromState(ref.Root, true, ref.State)
	if after.State != "" {
		out.State = after.State
	}
	return out, nil
}
