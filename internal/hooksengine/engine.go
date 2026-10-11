package hooksengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// Engine is the thin application execution layer over hooks.Dispatcher.
type Engine struct {
	Registry hooks.Registry
	Env      Env

	// BridgeLookup resolves adapter EventBridge declarations. nil → LookupBridge.
	// Production has no registered bridges yet (fail closed / empty declarations).
	BridgeLookup func(adapters.ID) (hooks.EventBridge, bool)

	// CapabilityOverride, when non-nil, replaces production negotiation entirely.
	// Production CLI leaves this nil. Tests may inject a capable surface without
	// mutating adapters.Capabilities or registering production bridges.
	CapabilityOverride *hooks.Capability
}

// New returns an engine with the Atlas default (empty) action registry.
func New() Engine {
	return Engine{Registry: DefaultRegistry()}
}

// Dispatch validates an untrusted request, resolves project identity, negotiates
// capability, and executes hooks.Dispatcher. It never selects actions from input.
func (e Engine) Dispatch(ctx context.Context, req Request) Response {
	if ctx == nil {
		ctx = context.Background()
	}
	trimRequest(&req)

	ref, err := e.Env.resolveProject()
	if err != nil {
		status, msg := classify(err)
		return errorResponse(status, msg, "", "", "", "")
	}

	ev, adapterID, err := e.buildEvent(req, ref.ProjectID)
	if err != nil {
		status, msg := classify(err)
		resp := errorResponse(status, msg, ref.ProjectID, string(adapterID), req.Event.ID, req.Event.Type)
		return resp
	}

	cap := e.capabilityFor(adapterID)
	dispatcher := hooks.Dispatcher{
		Registry:   e.Registry,
		Capability: cap,
	}
	report := dispatcher.Dispatch(ctx, ev)
	return responseFromReport(ref.ProjectID, adapterID, report)
}

func (e Engine) capabilityFor(id adapters.ID) hooks.Capability {
	if e.CapabilityOverride != nil {
		return *e.CapabilityOverride
	}
	lookup := e.BridgeLookup
	if lookup == nil {
		lookup = LookupBridge
	}
	bridge, ok := lookup(id)
	if !ok {
		bridge = nil
	}
	return negotiateAdapter(id, bridge)
}

func (e Engine) buildEvent(req Request, resolvedProjectID string) (hooks.Event, adapters.ID, error) {
	if err := validateSchemaVersion(req.SchemaVersion); err != nil {
		return hooks.Event{}, "", err
	}
	adapterID, err := parseAdapter(req.Adapter)
	if err != nil {
		return hooks.Event{}, "", err
	}

	occurred, err := time.Parse(time.RFC3339, req.Event.OccurredAt)
	if err != nil {
		// Also accept RFC3339Nano.
		occurred, err = time.Parse(time.RFC3339Nano, req.Event.OccurredAt)
		if err != nil {
			return hooks.Event{}, adapterID, fmt.Errorf("%w: occurred_at must be RFC3339", hooks.ErrInvalidEvent)
		}
	}

	payloadProject := strings.TrimSpace(req.Event.Payload.ProjectID)
	if payloadProject != "" && payloadProject != resolvedProjectID {
		return hooks.Event{}, adapterID, ErrProjectMismatch
	}
	if payloadProject == "" {
		payloadProject = resolvedProjectID
	}

	ev := hooks.Event{
		ID:         req.Event.ID,
		Type:       hooks.EventType(req.Event.Type),
		OccurredAt: occurred.UTC(),
		Adapter:    adapterID,
		Payload: hooks.Payload{
			ProjectID:        payloadProject,
			SessionID:        req.Event.Payload.SessionID,
			TaskID:           req.Event.Payload.TaskID,
			ToolName:         req.Event.Payload.ToolName,
			ToolCallID:       req.Event.Payload.ToolCallID,
			Status:           req.Event.Payload.Status,
			Reason:           req.Event.Payload.Reason,
			TaskSummary:      req.Event.Payload.TaskSummary,
			TaskSummaryState: hooks.TaskSummaryState(req.Event.Payload.TaskSummaryState),
		},
	}
	if err := hooks.ValidateEvent(ev); err != nil {
		return hooks.Event{}, adapterID, err
	}
	return ev, adapterID, nil
}

func parseAdapter(raw string) (adapters.ID, error) {
	id := adapters.ID(strings.TrimSpace(raw))
	if id == "" {
		return "", fmt.Errorf("%w: adapter is required", ErrInvalidAdapter)
	}
	if _, ok := adapters.For(id); !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidAdapter, id)
	}
	return id, nil
}

func responseFromReport(projectID string, adapter adapters.ID, report hooks.DispatchReport) Response {
	results := make([]ResultDTO, 0, len(report.Results))
	for _, r := range report.Results {
		results = append(results, resultFromHook(r))
	}

	resp := Response{
		SchemaVersion: SchemaVersion,
		Command:       CommandDispatch,
		ProjectID:     projectID,
		Adapter:       string(adapter),
		EventID:       report.EventID,
		EventType:     string(report.EventType),
		Aborted:       report.Aborted,
		Count:         len(results),
		Results:       results,
	}

	if report.Err != nil {
		status, msg := classify(report.Err)
		resp.Status = status
		resp.Error = &ErrorBody{Code: status, Message: msg}
		return resp
	}
	if report.Aborted {
		resp.Status = StatusAborted
		resp.Error = &ErrorBody{Code: StatusAborted, Message: "dispatch aborted"}
		return resp
	}
	resp.Status = StatusSuccess
	return resp
}

func errorResponse(status, message, projectID, adapter, eventID, eventType string) Response {
	if message == "" {
		message = status
	}
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       CommandDispatch,
		Status:        status,
		ProjectID:     projectID,
		Adapter:       adapter,
		EventID:       eventID,
		EventType:     eventType,
		Count:         0,
		Error: &ErrorBody{
			Code:    status,
			Message: message,
		},
	}
}
