package hooks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Action is a consumer-owned hook handler.
//
// Concurrency and immutability:
//   - After NewRegistry, ID(), Events(), and Mode() must be effectively immutable.
//     The registry snapshots those values at registration time.
//   - Run may execute concurrently with other Actions and with a prior Run that
//     outlived its timeout (abandoned goroutine). Implementations MUST be safe
//     for concurrent Run calls and MUST NOT rely on Dispatch serializing all work
//     after a timeout.
//   - Actions must be deterministic for a given Event, avoid mandatory network,
//     and must not perform automatic Git operations.
type Action interface {
	ID() string
	Events() []EventType
	Mode() ExecutionMode
	Run(ctx context.Context, ev Event) (Result, error)
}

// Registration binds an Action to dispatch policy.
// NewRegistry copies FailurePolicy/Timeout and snapshots Action metadata;
// mutating the caller's Registration slice afterward does not affect the Registry.
type Registration struct {
	Action Action
	// FailurePolicy defaults to FailContinue when empty.
	FailurePolicy FailurePolicy
	// Timeout defaults to DefaultActionTimeout when zero.
	Timeout time.Duration
}

func (r Registration) policy() FailurePolicy {
	if r.FailurePolicy == "" {
		return FailContinue
	}
	return r.FailurePolicy
}

func (r Registration) timeout() time.Duration {
	if r.Timeout == 0 {
		return DefaultActionTimeout
	}
	return r.Timeout
}

// snapshotAction freezes ID/Events/Mode at registration time so later mutation
// of caller-owned event slices (or unstable Action metadata) cannot reshape
// dispatch. Run still delegates to the inner Action.
type snapshotAction struct {
	id     string
	events []EventType
	mode   ExecutionMode
	inner  Action
}

func (s snapshotAction) ID() string { return s.id }
func (s snapshotAction) Events() []EventType {
	out := make([]EventType, len(s.events))
	copy(out, s.events)
	return out
}
func (s snapshotAction) Mode() ExecutionMode { return s.mode }
func (s snapshotAction) Run(ctx context.Context, ev Event) (Result, error) {
	return s.inner.Run(ctx, ev)
}

// Registry is an immutable, deterministically ordered set of registrations.
// Methods return defensive copies of registration metadata. The Action Run
// target is shared by reference (see Action concurrency contract).
type Registry struct {
	byID    map[string]Registration
	ordered []Registration
}

// NewRegistry validates and stores registrations.
// Order is sorted by action ID for deterministic dispatch.
// Duplicate action IDs are rejected (ownership conflict at registration time).
func NewRegistry(regs []Registration) (Registry, error) {
	byID := make(map[string]Registration, len(regs))
	for _, reg := range regs {
		if err := ValidateRegistration(reg); err != nil {
			return Registry{}, err
		}
		id := strings.TrimSpace(reg.Action.ID())
		if _, exists := byID[id]; exists {
			return Registry{}, fmt.Errorf("%w: %s", ErrDuplicateAction, id)
		}
		policy := reg.FailurePolicy
		if policy == "" {
			policy = FailContinue
		}
		events := reg.Action.Events()
		ownedEvents := make([]EventType, len(events))
		copy(ownedEvents, events)
		frozen := Registration{
			Action: snapshotAction{
				id:     id,
				events: ownedEvents,
				mode:   reg.Action.Mode(),
				inner:  reg.Action,
			},
			FailurePolicy: policy,
			Timeout:       reg.Timeout,
		}
		byID[id] = frozen
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ordered := make([]Registration, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, byID[id])
	}
	return Registry{byID: byID, ordered: ordered}, nil
}

// Len returns the number of registered actions.
func (r Registry) Len() int { return len(r.ordered) }

// All returns registrations in deterministic action-ID order.
// The returned slice and each Events() snapshot are owned by the caller.
func (r Registry) All() []Registration {
	out := make([]Registration, len(r.ordered))
	for i, reg := range r.ordered {
		out[i] = copyRegistration(reg)
	}
	return out
}

// ForEvent returns registrations subscribed to t, in deterministic order.
func (r Registry) ForEvent(t EventType) []Registration {
	out := make([]Registration, 0, len(r.ordered))
	for _, reg := range r.ordered {
		for _, et := range reg.Action.Events() {
			if et == t {
				out = append(out, copyRegistration(reg))
				break
			}
		}
	}
	return out
}

func copyRegistration(reg Registration) Registration {
	snap, ok := reg.Action.(snapshotAction)
	if !ok {
		return Registration{
			Action:        reg.Action,
			FailurePolicy: reg.FailurePolicy,
			Timeout:       reg.Timeout,
		}
	}
	events := make([]EventType, len(snap.events))
	copy(events, snap.events)
	return Registration{
		Action: snapshotAction{
			id:     snap.id,
			events: events,
			mode:   snap.mode,
			inner:  snap.inner,
		},
		FailurePolicy: reg.FailurePolicy,
		Timeout:       reg.Timeout,
	}
}
