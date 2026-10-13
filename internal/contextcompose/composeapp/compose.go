package composeapp

import (
	"strings"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/cesource"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/codegraphsource"
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
	// Sources optionally restricts requested sources.
	// Empty → memory + context_economy + codegraph.
	Sources []string
	Env     memoryretrieve.Env
}

// Result is the JSON-oriented composition envelope for internal transport.
type Result struct {
	contextcompose.Response
	Command   string `json:"command"`
	ProjectID string `json:"project_id,omitempty"`
}

// Compose retrieves Memory, CE, and CodeGraph candidates, then runs contextcompose.Compose.
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

	codeGraphCap := budget * contextcompose.CodeGraphBudgetPercent / 100
	if codeGraphCap > budget {
		codeGraphCap = budget
	}
	if codeGraphCap <= 0 {
		codeGraphCap = 1
	}

	sources := normalizeSources(in.Sources)
	wantMemory := containsSource(sources, contextcompose.SourceMemory)
	wantCE := containsSource(sources, contextcompose.SourceContextEconomy)
	wantCG := containsSource(sources, contextcompose.SourceCodeGraph)

	var (
		groups    []candidateGroup
		statuses  []contextcompose.SourceStatus
		projectID string
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
		groups = append(groups, candidateGroup{
			source: contextcompose.SourceMemory,
			cands:  mem.Candidates,
		})
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
		groups = append(groups, candidateGroup{
			source: contextcompose.SourceContextEconomy,
			cands:  ce.Candidates,
		})
		statuses = append(statuses, ce.SourceStatus)
	}

	if wantCG {
		cg := codegraphsource.Retrieve(codegraphsource.Input{
			Task:     task,
			Query:    query,
			MaxItems: contextcompose.MaxCodeGraphResults,
			Env:      in.Env,
		})
		if projectID == "" && cg.ProjectID != "" {
			projectID = cg.ProjectID
		}
		groups = append(groups, candidateGroup{
			source: contextcompose.SourceCodeGraph,
			cands:  cg.Candidates,
		})
		statuses = append(statuses, cg.SourceStatus)
	}

	cands, statuses := mergeCandidatesFair(groups, statuses, contextcompose.MaxCandidates)

	req := contextcompose.Request{
		Task:        task,
		Query:       query,
		BudgetChars: budget,
		Sources:     sources,
		SourceCaps: map[string]int{
			contextcompose.SourceMemory:    memoryCap,
			contextcompose.SourceCodeGraph: codeGraphCap,
		},
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

type candidateGroup struct {
	source string
	cands  []contextcompose.Candidate
}

// mergeCandidatesFair bounds the combined candidate set to max without
// append-order bias. When multiple sources overflow, slots are split evenly
// across non-empty sources (remainder to earlier groups — Memory, then CE,
// then CodeGraph — matching compose priority). Spare capacity from undersized
// sources is redistributed in the same priority order until max is reached or
// all candidates are exhausted. Dropped sources are annotated on SourceStatus
// rather than silently truncated.
func mergeCandidatesFair(
	groups []candidateGroup,
	statuses []contextcompose.SourceStatus,
	max int,
) ([]contextcompose.Candidate, []contextcompose.SourceStatus) {
	if max <= 0 {
		return nil, statuses
	}

	total := 0
	active := 0
	for _, g := range groups {
		total += len(g.cands)
		if len(g.cands) > 0 {
			active++
		}
	}
	if total <= max {
		out := make([]contextcompose.Candidate, 0, total)
		for _, g := range groups {
			out = append(out, g.cands...)
		}
		return out, statuses
	}
	if active == 0 {
		return nil, statuses
	}

	takes := make([]int, len(groups))
	base := max / active
	rem := max % active
	for i, g := range groups {
		if len(g.cands) == 0 {
			continue
		}
		takes[i] = base
		if rem > 0 {
			takes[i]++
			rem--
		}
		if takes[i] > len(g.cands) {
			takes[i] = len(g.cands)
		}
	}

	// Redistribute every remaining slot until max is filled or candidates end.
	// Walk sources in declared order (Memory → CE → CodeGraph) each pass so
	// spare capacity prefers higher-priority sources while preserving the
	// initial fair split for sources that still have room.
	for {
		used := 0
		for _, n := range takes {
			used += n
		}
		if used >= max {
			break
		}
		spare := max - used
		progressed := false
		for i, g := range groups {
			if spare <= 0 {
				break
			}
			room := len(g.cands) - takes[i]
			if room <= 0 {
				continue
			}
			add := room
			if add > spare {
				add = spare
			}
			takes[i] += add
			spare -= add
			progressed = true
		}
		if !progressed {
			break
		}
	}

	out := make([]contextcompose.Candidate, 0, max)
	for i, g := range groups {
		n := takes[i]
		if n > len(g.cands) {
			n = len(g.cands)
		}
		if n > 0 {
			out = append(out, g.cands[:n]...)
		}
		statuses = annotateTruncation(statuses, g.source, len(g.cands), n)
	}
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
		return []string{
			contextcompose.SourceMemory,
			contextcompose.SourceContextEconomy,
			contextcompose.SourceCodeGraph,
		}
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
		return []string{
			contextcompose.SourceMemory,
			contextcompose.SourceContextEconomy,
			contextcompose.SourceCodeGraph,
		}
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
