package composeapp

import (
	"strings"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/cesource"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/memorysource"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
)

const (
	CommandCompose = "compose"
)

// Input is a bounded composition orchestration request.
type Input struct {
	Task        string
	Query       string
	BudgetChars int // 0 → contextcompose.DefaultBudgetChars
	MemoryCap   int // 0 → MemoryBudgetPercent of global; aligned to SourceCaps
	MaxItems    int // 0 → contextcompose.DefaultMaxItems
	// Sources optionally restricts requested sources. Empty → memory + context_economy.
	Sources []string
	Env     memoryretrieve.Env
}

// Result is the JSON-oriented composition envelope for internal transport.
type Result struct {
	contextcompose.Response
	Command   string `json:"command"`
	ProjectID string `json:"project_id,omitempty"`
}

// Compose retrieves Memory and CE candidates, then runs contextcompose.Compose.
func Compose(in Input) Result {
	task := strings.TrimSpace(in.Task)
	query := strings.TrimSpace(in.Query)
	if task == "" && query == "" {
		return invalid("task or query is required")
	}
	if !utf8.ValidString(in.Task) || !utf8.ValidString(in.Query) {
		return invalid("task/query must be valid UTF-8")
	}
	if utf8.RuneCountInString(task) > contextcompose.MaxTaskRunes {
		return invalid("task exceeds maximum length")
	}
	if utf8.RuneCountInString(query) > contextcompose.MaxQueryRunes {
		return invalid("query exceeds maximum length")
	}

	budget := in.BudgetChars
	switch {
	case budget < 0:
		return invalid("budget_chars must be non-negative")
	case budget == 0:
		budget = contextcompose.DefaultBudgetChars
	case budget > contextcompose.MaxBudgetChars:
		return invalid("budget_chars exceeds maximum")
	}

	maxItems := in.MaxItems
	switch {
	case maxItems < 0:
		return invalid("max_items must be non-negative")
	case maxItems == 0:
		maxItems = contextcompose.DefaultMaxItems
	case maxItems > contextcompose.MaxCandidates:
		return invalid("max_items exceeds maximum")
	}

	memoryCap := in.MemoryCap
	switch {
	case memoryCap < 0:
		return invalid("memory source cap must be non-negative")
	case memoryCap == 0:
		memoryCap = budget * contextcompose.MemoryBudgetPercent / 100
	}
	if memoryCap > budget {
		memoryCap = budget
	}

	sources := normalizeSources(in.Sources)
	wantMemory := containsSource(sources, contextcompose.SourceMemory)
	wantCE := containsSource(sources, contextcompose.SourceContextEconomy)

	var (
		memCands, ceCands []contextcompose.Candidate
		statuses          []contextcompose.SourceStatus
		projectID         string
	)

	if wantMemory {
		mem := memorysource.Retrieve(memorysource.Input{
			Task:        task,
			Query:       query,
			BudgetChars: budget,
			MemoryCap:   memoryCap,
			MaxItems:    maxItems,
			Env:         in.Env,
		})
		if mem.ProjectID != "" {
			projectID = mem.ProjectID
		}
		memCands = mem.Candidates
		statuses = append(statuses, mem.SourceStatus)
	}

	if wantCE {
		ce := cesource.Retrieve(cesource.Input{
			Task:     task,
			Query:    query,
			MaxItems: contextcompose.MaxCEPaths,
			Env:      in.Env,
		})
		if projectID == "" && ce.ProjectID != "" {
			projectID = ce.ProjectID
		}
		ceCands = ce.Candidates
		statuses = append(statuses, ce.SourceStatus)
	}

	cands, statuses := mergeCandidatesFair(memCands, ceCands, statuses, contextcompose.MaxCandidates)

	req := contextcompose.Request{
		Task:           task,
		Query:          query,
		BudgetChars:    budget,
		Sources:        sources,
		SourceCaps:     map[string]int{contextcompose.SourceMemory: memoryCap},
		MaxItems:       maxItems,
		Candidates:     cands,
		SourceStatuses: statuses,
	}
	resp := contextcompose.Compose(req)
	return Result{
		Response:  resp,
		Command:   CommandCompose,
		ProjectID: projectID,
	}
}

// mergeCandidatesFair bounds the combined candidate set to max without
// append-order bias. When both sources overflow, slots are split evenly
// (remainder to Memory, which has higher compose priority). Dropped sources
// are annotated on SourceStatus rather than silently truncated.
func mergeCandidatesFair(
	mem, ce []contextcompose.Candidate,
	statuses []contextcompose.SourceStatus,
	max int,
) ([]contextcompose.Candidate, []contextcompose.SourceStatus) {
	if max <= 0 {
		return nil, statuses
	}
	if len(mem)+len(ce) <= max {
		out := make([]contextcompose.Candidate, 0, len(mem)+len(ce))
		out = append(out, mem...)
		out = append(out, ce...)
		return out, statuses
	}

	memN, ceN := len(mem), len(ce)
	memTake, ceTake := memN, ceN
	if memN+ceN > max {
		if memN == 0 {
			ceTake = max
		} else if ceN == 0 {
			memTake = max
		} else {
			memTake = max / 2
			ceTake = max - memTake
			if memN < memTake {
				ceTake = max - memN
				memTake = memN
			}
			if ceN < ceTake {
				memTake = max - ceN
				ceTake = ceN
			}
			if memTake > memN {
				memTake = memN
			}
			if ceTake > ceN {
				ceTake = ceN
			}
		}
	}

	out := make([]contextcompose.Candidate, 0, memTake+ceTake)
	out = append(out, mem[:memTake]...)
	out = append(out, ce[:ceTake]...)

	statuses = annotateTruncation(statuses, contextcompose.SourceMemory, memN, memTake)
	statuses = annotateTruncation(statuses, contextcompose.SourceContextEconomy, ceN, ceTake)
	return out, statuses
}

func annotateTruncation(statuses []contextcompose.SourceStatus, source string, have, kept int) []contextcompose.SourceStatus {
	if have <= kept {
		return statuses
	}
	msg := "candidate set truncated to preserve source diversity under composer bound"
	for i := range statuses {
		if statuses[i].Source != source {
			continue
		}
		if statuses[i].Message == "" {
			statuses[i].Message = msg
		} else if !strings.Contains(statuses[i].Message, "truncated") {
			statuses[i].Message = statuses[i].Message + "; " + msg
		}
		return statuses
	}
	return append(statuses, contextcompose.SourceStatus{
		Source:  source,
		Status:  contextcompose.SourceStatusSupplied,
		Message: msg,
	})
}

func normalizeSources(in []string) []string {
	if len(in) == 0 {
		return []string{contextcompose.SourceMemory, contextcompose.SourceContextEconomy}
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{contextcompose.SourceMemory, contextcompose.SourceContextEconomy}
	}
	return out
}

func containsSource(sources []string, want string) bool {
	for _, s := range sources {
		if s == want {
			return true
		}
	}
	return false
}

func invalid(message string) Result {
	return Result{
		Command: CommandCompose,
		Response: contextcompose.Response{
			SchemaVersion: contextcompose.SchemaVersion,
			Status:        contextcompose.StatusInvalidRequest,
			Count:         0,
			Error: &contextcompose.ErrorBody{
				Code:    contextcompose.StatusInvalidRequest,
				Message: message,
			},
		},
	}
}

// ShouldExitNonZero reports whether the process should exit non-zero after JSON.
func ShouldExitNonZero(res Result) bool {
	switch res.Status {
	case contextcompose.StatusSuccess, contextcompose.StatusEmpty:
		return false
	case contextcompose.StatusPartial:
		// Partial with packed items is usable; empty partial is a hard failure.
		return res.Count == 0
	default:
		return true
	}
}

// ExitError signals a non-zero process exit after JSON was written to stdout.
type ExitError struct {
	Status string
}

func (e ExitError) Error() string {
	if e.Status == "" {
		return "context compose: request failed"
	}
	return "context compose: " + e.Status
}
