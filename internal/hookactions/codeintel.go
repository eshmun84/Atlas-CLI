package hookactions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// Stable Action identity for Code Intelligence maintenance.
const ActionIDCodeIntel = "codeintel.refresh.maintain"

// CodeIntelTimeout bounds one Code Intelligence maintenance Run.
// Aligns with provider build budgets (explicit refresh may take minutes).
const CodeIntelTimeout = 2 * time.Minute

// CodeIntelAction refreshes the Code Intelligence graph under Atlas Home when
// missing or stale. Reuses codeintel.Service.Refresh. Never installs CodeGraph,
// never projects MCP, never writes graph.db into the product repository.
type CodeIntelAction struct {
	// Resolve supplies workspace identity. nil → DefaultResolve.
	Resolve ResolveFunc
	// Service is the Code Intelligence facade. nil → default CodeGraph service.
	Service *codeintel.Service
	// Now overrides the clock for Refresh. nil → time.Now.
	Now func() time.Time
}

var _ hooks.Action = (*CodeIntelAction)(nil)

func (a *CodeIntelAction) ID() string { return ActionIDCodeIntel }

// Events lists production session subscriptions (session.start only).
// Cursor beforeSubmitPrompt fallback is evaluated in hooksengine (Phase B.8),
// not via task.start subscription. session.resume remains unmapped.
func (a *CodeIntelAction) Events() []hooks.EventType {
	return []hooks.EventType{
		hooks.EventSessionStart,
	}
}

func (a *CodeIntelAction) Mode() hooks.ExecutionMode {
	return hooks.ModeNonBlocking
}

// Run admits session maintenance (shared with Context Economy Action).
func (a *CodeIntelAction) Run(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
	return AdmitSessionMaintenance(ctx, ev, a.Resolve)
}

// CodeIntelResult is the structured outcome of one maintenance attempt.
type CodeIntelResult struct {
	Outcome     hooks.Outcome
	Noop        bool
	Skipped     bool
	State       codeintel.State
	Freshness   codeintel.Freshness
	Mode        codeintel.RefreshMode
	ProjectID   string
	HomePath    string
	GraphDB     string
	Message     string
	Containment codeintel.ContainmentReport
}

// MaintainCodeIntel is the package-level entry for direct invocation.
// Ignores TriggersEnabled.
func MaintainCodeIntel(ctx context.Context, root, homePath string) (CodeIntelResult, error) {
	ref, err := ResolveAt(root, homePath)
	if err != nil {
		return CodeIntelResult{Outcome: hooks.OutcomeFailed}, err
	}
	return (&CodeIntelAction{}).Maintain(ctx, ref)
}

// Maintain probes freshness and refreshes when needed.
// Unavailable / incompatible → OutcomeSkipped (no install, no failure of Atlas).
// Fresh → no-op. Missing → initial. Stale → incremental. Ignores TriggersEnabled.
func (a *CodeIntelAction) Maintain(ctx context.Context, ref ProjectRef) (CodeIntelResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CodeIntelResult{Outcome: hooks.OutcomeCancelled, ProjectID: ref.ProjectID, HomePath: ref.Home}, err
	}

	out := CodeIntelResult{
		ProjectID: ref.ProjectID,
		HomePath:  ref.Home,
	}
	svc := a.service()
	project := codeintel.Project{
		Root:     ref.Root,
		ID:       ref.ProjectID,
		HomePath: ref.Home,
	}

	snap := svc.Snapshot(ctx, project)
	out.State = snap.State
	out.Freshness = snap.Freshness
	out.GraphDB = snap.GraphDBPath
	out.Message = snap.Message

	switch snap.State {
	case codeintel.StateUnavailable, codeintel.StateMissing:
		out.Outcome = hooks.OutcomeSkipped
		out.Skipped = true
		out.Message = "code intelligence provider unavailable"
		return out, nil
	case codeintel.StateIncompatible:
		out.Outcome = hooks.OutcomeSkipped
		out.Skipped = true
		if strings.TrimSpace(out.Message) == "" {
			out.Message = "code intelligence provider incompatible"
		}
		return out, nil
	case codeintel.StateError:
		out.Outcome = hooks.OutcomeFailed
		if strings.TrimSpace(out.Message) == "" {
			out.Message = "code intelligence provider error"
		}
		return out, fmt.Errorf("hookactions: %s", out.Message)
	}

	switch snap.Freshness {
	case codeintel.FreshnessReady:
		out.Outcome = hooks.OutcomeOK
		out.Noop = true
		out.Mode = codeintel.RefreshModeNoop
		out.Message = "code intelligence graph already fresh"
		return out, nil
	case codeintel.FreshnessError:
		out.Outcome = hooks.OutcomeFailed
		out.Message = "code intelligence freshness error"
		if snap.FreshnessReason != "" {
			out.Message = snap.FreshnessReason
		}
		return out, fmt.Errorf("hookactions: %s", out.Message)
	case codeintel.FreshnessMissing, codeintel.FreshnessStale:
		// proceed
	default:
		// Unknown freshness with available provider — attempt refresh planning via Service.
	}

	if err := ctx.Err(); err != nil {
		out.Outcome = hooks.OutcomeCancelled
		return out, err
	}

	refresh, err := svc.Refresh(ctx, codeintel.RefreshOptions{
		Root:        ref.Root,
		ProjectName: ref.Name,
		ProjectID:   ref.ProjectID,
		HomePath:    ref.Home,
		ForceFull:   false,
		Now:         a.Now,
	})
	out.Containment = refresh.Containment
	out.GraphDB = refresh.GraphDBPath
	out.Mode = refresh.Mode
	out.State = refresh.State
	out.Freshness = refresh.Freshness
	out.Message = refresh.Message

	if refresh.Blocked {
		switch refresh.State {
		case codeintel.StateUnavailable, codeintel.StateMissing:
			out.Outcome = hooks.OutcomeSkipped
			out.Skipped = true
			return out, nil
		case codeintel.StateIncompatible:
			out.Outcome = hooks.OutcomeSkipped
			out.Skipped = true
			return out, nil
		default:
			out.Outcome = hooks.OutcomeFailed
			if err != nil {
				return out, err
			}
			msg := refresh.Message
			if msg == "" && len(refresh.Blockers) > 0 {
				msg = strings.Join(refresh.Blockers, "; ")
			}
			return out, fmt.Errorf("hookactions: codeintel blocked: %s", msg)
		}
	}
	if err != nil {
		out.Outcome = hooks.OutcomeFailed
		return out, err
	}
	out.Noop = refresh.Noop
	out.Outcome = hooks.OutcomeOK
	return out, nil
}

func (a *CodeIntelAction) service() *codeintel.Service {
	if a != nil && a.Service != nil {
		return a.Service
	}
	return codeintel.NewService(codegraph.New())
}
