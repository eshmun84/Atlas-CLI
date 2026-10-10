package memoryretrieve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
)

// Service is the read-only Memory retrieval facade used by the CLI.
type Service struct {
	Env Env
}

// SearchInput is a CLI search request.
type SearchInput struct {
	Query string
	Limit int
}

// GetInput is a CLI get request.
type GetInput struct {
	ID string
}

// ContextInput is a progressive retrieval request.
type ContextInput struct {
	Task   string
	Limit  int
	Budget int
}

func (s Service) openReadOnly() (*memory.Store, projectRef, Response, bool) {
	ref, err := s.Env.resolveProject()
	if err != nil {
		status, msg := classify(err)
		return nil, projectRef{}, errorResponse("", status, msg), false
	}
	ok, err := memory.Exists(ref.Home, ref.ProjectID)
	if err != nil {
		status, msg := classify(err)
		resp := errorResponse("", status, msg)
		resp.ProjectID = ref.ProjectID
		return nil, ref, resp, false
	}
	if !ok {
		resp := errorResponse("", StatusMissingMemoryDB, "memory database not found")
		resp.ProjectID = ref.ProjectID
		return nil, ref, resp, false
	}
	store, err := memory.OpenReadOnly(ref.Home, ref.ProjectID)
	if err != nil {
		status, msg := classify(err)
		resp := errorResponse("", status, msg)
		resp.ProjectID = ref.ProjectID
		return nil, ref, resp, false
	}
	return store, ref, Response{}, true
}

// Search runs FTS5 search and returns compact hits (not full content).
func (s Service) Search(in SearchInput) Response {
	store, ref, early, ok := s.openReadOnly()
	if !ok {
		early.Command = CommandSearch
		return early
	}
	defer store.Close()

	query := strings.TrimSpace(in.Query)
	limit := clampSearchLimit(in.Limit)
	if query == "" {
		resp := errorResponse(CommandSearch, StatusInvalidQuery, "search query is required")
		resp.ProjectID = ref.ProjectID
		resp.Limit = limit
		return resp
	}

	obs, err := store.Search(memory.SearchQuery{Query: query, Limit: limit})
	if err != nil {
		status, msg := classify(err)
		resp := errorResponse(CommandSearch, status, msg)
		resp.ProjectID = ref.ProjectID
		resp.Query = query
		resp.Limit = limit
		return resp
	}

	hits := make([]SearchHit, 0, len(obs))
	for _, o := range obs {
		hits = append(hits, SearchHit{
			ID:        o.ID,
			Type:      string(o.Type),
			Scope:     string(o.Scope),
			Title:     o.Title,
			TopicKey:  o.TopicKey,
			UpdatedAt: formatTime(o.UpdatedAt),
			Preview:   previewText(o.Content),
		})
	}
	// Preserve FTS rank order from Store.Search (rank, updated_at DESC, id ASC).

	status := StatusSuccess
	if len(hits) == 0 {
		status = StatusNoResults
	}
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       CommandSearch,
		Status:        status,
		ProjectID:     ref.ProjectID,
		Query:         query,
		Limit:         limit,
		Count:         len(hits),
		Results:       hits,
	}
}

// Get returns one full observation for the current project.
func (s Service) Get(in GetInput) Response {
	store, ref, early, ok := s.openReadOnly()
	if !ok {
		early.Command = CommandGet
		return early
	}
	defer store.Close()

	id := strings.TrimSpace(in.ID)
	if id == "" {
		resp := errorResponse(CommandGet, StatusInvalidUsage, "observation id is required")
		resp.ProjectID = ref.ProjectID
		return resp
	}

	obs, err := store.GetObservation(id)
	if err != nil {
		status, msg := classify(err)
		resp := errorResponse(CommandGet, status, msg)
		resp.ProjectID = ref.ProjectID
		return resp
	}
	dto := toDTO(obs)
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       CommandGet,
		Status:        StatusSuccess,
		ProjectID:     ref.ProjectID,
		Count:         1,
		Observation:   &dto,
	}
}

// Context performs progressive FTS-backed retrieval under a character budget.
func (s Service) Context(in ContextInput) Response {
	store, ref, early, ok := s.openReadOnly()
	if !ok {
		early.Command = CommandContext
		return early
	}
	defer store.Close()

	task := strings.TrimSpace(in.Task)
	limit := clampContextLimit(in.Limit)
	budget := clampBudget(in.Budget)
	limitsNote := []string{
		"Progressive retrieval via FTS5; not semantic/embeddings search.",
		"Items are compact excerpts; use atlas memory get <id> for full content.",
		"used_chars counts UTF-8 runes in id+type+scope+title+topic_key+excerpt per item.",
		fmt.Sprintf("Budget capped at %d characters; item limit %d.", budget, limit),
	}

	if task == "" {
		resp := errorResponse(CommandContext, StatusInvalidQuery, "task description is required")
		resp.ProjectID = ref.ProjectID
		resp.BudgetChars = budget
		resp.Limit = limit
		resp.Limits = limitsNote
		return resp
	}

	tokens := ftsTokens(task)
	ftsQuery := buildFTSQuery(tokens)
	var obs []memory.Observation
	if ftsQuery != "" {
		var err error
		obs, err = store.Search(memory.SearchQuery{Query: ftsQuery, Limit: limit * 2})
		if err != nil {
			status, msg := classify(err)
			// Sanitized query should rarely fail; surface cleanly.
			resp := errorResponse(CommandContext, status, msg)
			resp.ProjectID = ref.ProjectID
			resp.Task = task
			resp.BudgetChars = budget
			resp.Limit = limit
			resp.Limits = limitsNote
			return resp
		}
	}

	ranked := make([]scoredObs, 0, len(obs))
	for i, o := range obs {
		// Prefer earlier FTS rank, then lexical overlap on title/topic.
		score := (len(obs) - i) * 10
		score += tokenOverlapScore(o.Title+" "+o.TopicKey, tokens) * 3
		score += tokenOverlapScore(o.Content, tokens)
		ranked = append(ranked, scoredObs{obs: o, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if !ranked[i].obs.UpdatedAt.Equal(ranked[j].obs.UpdatedAt) {
			return ranked[i].obs.UpdatedAt.After(ranked[j].obs.UpdatedAt)
		}
		return ranked[i].obs.ID < ranked[j].obs.ID
	})

	items, used := selectContextItems(ranked, limit, budget)
	if used > budget {
		// Invariant violation — fail closed rather than emit an over-budget payload.
		resp := errorResponse(CommandContext, StatusInternalError, "memory request failed")
		resp.ProjectID = ref.ProjectID
		resp.Task = task
		resp.BudgetChars = budget
		resp.Limit = limit
		resp.Limits = limitsNote
		return resp
	}

	status := StatusSuccess
	if len(items) == 0 {
		status = StatusNoResults
	}
	return Response{
		SchemaVersion: SchemaVersion,
		Command:       CommandContext,
		Status:        status,
		ProjectID:     ref.ProjectID,
		Task:          task,
		Limit:         limit,
		BudgetChars:   budget,
		UsedChars:     used,
		Count:         len(items),
		Items:         items,
		Limits:        limitsNote,
	}
}

func toDTO(o memory.Observation) ObservationDTO {
	return ObservationDTO{
		ID:             o.ID,
		ProjectID:      o.ProjectID,
		SessionID:      o.SessionID,
		Type:           string(o.Type),
		Scope:          string(o.Scope),
		Title:          o.Title,
		Content:        o.Content,
		TopicKey:       o.TopicKey,
		SourceKind:     o.SourceKind,
		SourceRef:      o.SourceRef,
		ContentHash:    o.ContentHash,
		DuplicateCount: o.DuplicateCount,
		RevisionCount:  o.RevisionCount,
		FirstSeenAt:    formatTime(o.FirstSeenAt),
		LastSeenAt:     formatTime(o.LastSeenAt),
		CreatedAt:      formatTime(o.CreatedAt),
		UpdatedAt:      formatTime(o.UpdatedAt),
	}
}
