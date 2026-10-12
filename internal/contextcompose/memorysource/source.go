package memorysource

import (
	"strings"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
)

// Input drives read-only Memory candidate retrieval for composition.
//
// Callers supply a composition-oriented request; this adapter never accepts
// arbitrary database paths. Project identity is resolved only through Env
// (workspace root + Atlas Home), matching memoryretrieve.
type Input struct {
	Task        string
	Query       string // used as retrieval task when Task is empty
	BudgetChars int    // composition global budget; 0 → contextcompose.DefaultBudgetChars
	MemoryCap   int    // optional explicit Memory source cap; 0 → MemoryBudgetPercent of global
	MaxItems    int    // passed to Memory as Limit; 0 → Memory default clamp
	Env         memoryretrieve.Env
}

// Result holds bounded Memory candidates and Composer-compatible status.
//
// RetrievalStatus preserves the memoryretrieve vocabulary. SourceStatus is the
// translation into contextcompose.SourceStatus without fabricating success.
// RetrievalCap is the budget passed to Memory Context (separate from Composer
// itemCost accounting).
type Result struct {
	Candidates      []contextcompose.Candidate
	SourceStatus    contextcompose.SourceStatus
	ProjectID       string
	RetrievalStatus string
	RetrievalCap    int
	RetrievalUsed   int // Memory used_chars (different formula than Composer)
	Message         string
}

// Retrieve invokes memoryretrieve.Service.Context and maps items to Candidates.
// It never writes files, migrates schemas, or opens Memory outside the
// read-only retrieval facade.
func Retrieve(in Input) Result {
	global, retrievalCap, early, ok := resolveBudgets(in)
	if !ok {
		return early
	}
	_ = global

	task := strings.TrimSpace(in.Task)
	if task == "" {
		task = strings.TrimSpace(in.Query)
	}
	if task == "" {
		return degraded("", memoryretrieve.StatusInvalidQuery, "task or query is required", retrievalCap)
	}
	if !utf8.ValidString(task) {
		return degraded("", memoryretrieve.StatusInvalidQuery, "task/query must be valid UTF-8", retrievalCap)
	}

	svc := memoryretrieve.Service{Env: in.Env}
	resp := svc.Context(memoryretrieve.ContextInput{
		Task:   task,
		Limit:  in.MaxItems,
		Budget: retrievalCap,
	})

	return mapResponse(resp, retrievalCap)
}

// resolveBudgets computes the Composer global budget and the Memory retrieval
// cap. The two contracts stay separate: Memory enforces its own used_chars
// formula; Compose recalculates Candidate/Item costs later.
func resolveBudgets(in Input) (global, retrievalCap int, early Result, ok bool) {
	global = in.BudgetChars
	switch {
	case global < 0:
		return 0, 0, degraded("", "", "budget_chars must be non-negative", 0), false
	case global == 0:
		global = contextcompose.DefaultBudgetChars
	case global > contextcompose.MaxBudgetChars:
		return 0, 0, degraded("", "", "budget_chars exceeds maximum", 0), false
	}

	retrievalCap = in.MemoryCap
	switch {
	case retrievalCap < 0:
		return 0, 0, degraded("", "", "memory source cap must be non-negative", 0), false
	case retrievalCap == 0:
		retrievalCap = global * contextcompose.MemoryBudgetPercent / 100
	}
	if retrievalCap > global {
		retrievalCap = global
	}
	if retrievalCap > memoryretrieve.MaxBudgetChars {
		retrievalCap = memoryretrieve.MaxBudgetChars
	}
	if retrievalCap > contextcompose.MaxBudgetChars {
		retrievalCap = contextcompose.MaxBudgetChars
	}
	// Memory clampBudget turns 0 into DefaultBudgetChars (4000). Always pass a
	// positive explicit cap so composition share is preserved.
	if retrievalCap <= 0 {
		retrievalCap = 1
	}
	return global, retrievalCap, Result{}, true
}

func mapResponse(resp memoryretrieve.Response, retrievalCap int) Result {
	cap := resp.BudgetChars
	if cap <= 0 {
		cap = retrievalCap
	}
	out := Result{
		ProjectID:       resp.ProjectID,
		RetrievalStatus: resp.Status,
		RetrievalCap:    cap,
		RetrievalUsed:   resp.UsedChars,
	}
	if resp.Error != nil && resp.Error.Message != "" {
		out.Message = resp.Error.Message
	}

	switch resp.Status {
	case memoryretrieve.StatusSuccess:
		out.Candidates = toCandidates(resp.Items)
		out.SourceStatus = contextcompose.SourceStatus{
			Source:  contextcompose.SourceMemory,
			Status:  contextcompose.SourceStatusSupplied,
			Message: "memory retrieval returned observations",
		}
		if len(out.Candidates) == 0 {
			// Defensive: success with zero items should not imply supply.
			out.SourceStatus = contextcompose.SourceStatus{
				Source:  contextcompose.SourceMemory,
				Status:  contextcompose.SourceStatusNoCandidates,
				Message: "memory retrieval returned no observations",
			}
		}
	case memoryretrieve.StatusNoResults:
		out.SourceStatus = contextcompose.SourceStatus{
			Source:  contextcompose.SourceMemory,
			Status:  contextcompose.SourceStatusNoCandidates,
			Message: messageOr(out.Message, "memory retrieval returned no observations"),
		}
	case memoryretrieve.StatusMissingMemoryDB,
		memoryretrieve.StatusNotInitialized:
		out.SourceStatus = contextcompose.SourceStatus{
			Source:  contextcompose.SourceMemory,
			Status:  contextcompose.SourceStatusUnavailable,
			Message: messageOr(out.Message, resp.Status),
		}
	case memoryretrieve.StatusUnsafeFilesystem,
		memoryretrieve.StatusIncompatibleSchema,
		memoryretrieve.StatusInvalidQuery,
		memoryretrieve.StatusInvalidUsage,
		memoryretrieve.StatusNotFound,
		memoryretrieve.StatusInternalError:
		out.SourceStatus = contextcompose.SourceStatus{
			Source:  contextcompose.SourceMemory,
			Status:  contextcompose.SourceStatusFailed,
			Message: messageOr(out.Message, resp.Status),
		}
	default:
		// Unknown retrieval status — fail closed, never fabricate success.
		out.SourceStatus = contextcompose.SourceStatus{
			Source:  contextcompose.SourceMemory,
			Status:  contextcompose.SourceStatusFailed,
			Message: messageOr(out.Message, "memory retrieval failed"),
		}
	}

	if out.Message == "" {
		out.Message = out.SourceStatus.Message
	}
	return out
}

func toCandidates(items []memoryretrieve.ContextItem) []contextcompose.Candidate {
	if len(items) == 0 {
		return nil
	}
	out := make([]contextcompose.Candidate, 0, len(items))
	for _, it := range items {
		out = append(out, toCandidate(it))
	}
	return out
}

// toCandidate maps a Memory ContextItem onto a Composer Candidate.
// Memory remains the authority for content and scores; no rescoring.
func toCandidate(it memoryretrieve.ContextItem) contextcompose.Candidate {
	return contextcompose.Candidate{
		Source:     contextcompose.SourceMemory,
		Kind:       strings.TrimSpace(it.Type),
		ID:         strings.TrimSpace(it.ID),
		Title:      strings.TrimSpace(it.Title),
		Excerpt:    strings.TrimSpace(it.Excerpt),
		Ref:        strings.TrimSpace(it.Ref.Get),
		Score:      it.Score,
		Provenance: provenanceOf(it),
	}
}

func provenanceOf(it memoryretrieve.ContextItem) string {
	scope := strings.TrimSpace(it.Scope)
	topic := strings.TrimSpace(it.TopicKey)
	switch {
	case scope != "" && topic != "":
		return scope + ":" + topic
	case topic != "":
		return topic
	default:
		return scope
	}
}

func degraded(projectID, retrievalStatus, message string, retrievalCap int) Result {
	status := contextcompose.SourceStatusFailed
	switch retrievalStatus {
	case memoryretrieve.StatusMissingMemoryDB, memoryretrieve.StatusNotInitialized:
		status = contextcompose.SourceStatusUnavailable
	case memoryretrieve.StatusNoResults:
		status = contextcompose.SourceStatusNoCandidates
	case "":
		status = contextcompose.SourceStatusFailed
	}
	msg := message
	if msg == "" {
		msg = retrievalStatus
	}
	return Result{
		ProjectID:       projectID,
		RetrievalStatus: retrievalStatus,
		RetrievalCap:    retrievalCap,
		Message:         msg,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceMemory,
			Status:  status,
			Message: msg,
		},
	}
}

func messageOr(msg, fallback string) string {
	if strings.TrimSpace(msg) != "" {
		return msg
	}
	return fallback
}
