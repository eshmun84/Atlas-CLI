// Package memoryretrieve is the read-only Memory application layer for
// agent-consumable CLI retrieval (Slice 36A).
//
// Persistence stays in internal/memory. Context Economy packs/capsules are
// intentionally out of scope — this package never writes Home or project files.
//
// Context budget: used_chars is the sum of UTF-8 runes in each returned item's
// id, type, scope, title, topic_key, and excerpt. Always used_chars <= budget_chars.
package memoryretrieve

import "time"

const (
	SchemaVersion = 1

	StatusSuccess            = "success"
	StatusNoResults          = "no_results"
	StatusInvalidQuery       = "invalid_query"
	StatusInvalidUsage       = "invalid_usage"
	StatusMissingMemoryDB    = "missing_memory_db"
	StatusIncompatibleSchema = "incompatible_schema"
	StatusUnsafeFilesystem   = "unsafe_filesystem"
	StatusNotInitialized     = "not_initialized"
	StatusNotFound           = "not_found"
	StatusInternalError      = "internal_error"

	CommandSearch  = "search"
	CommandGet     = "get"
	CommandContext = "context"

	DefaultSearchLimit  = 20
	MaxSearchLimit      = 100
	DefaultContextLimit = 8
	MaxContextLimit     = 32
	DefaultBudgetChars  = 4000
	MaxBudgetChars      = 32000
	DefaultPreviewChars = 160
	DefaultExcerptChars = 280
)

// Response is the versioned JSON envelope for all memory CLI commands.
type Response struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	ProjectID     string `json:"project_id,omitempty"`

	Query string `json:"query,omitempty"`
	Task  string `json:"task,omitempty"`
	Limit int    `json:"limit,omitempty"`
	// BudgetChars is the progressive-retrieval cap in UTF-8 runes (context only).
	BudgetChars int `json:"budget_chars,omitempty"`
	// UsedChars is Σ charged runes across returned context items:
	// id + type + scope + title + topic_key + excerpt per item.
	// Invariant: UsedChars <= BudgetChars. See budget.go.
	UsedChars int `json:"used_chars,omitempty"`
	Count     int `json:"count"`

	Results     []SearchHit     `json:"results,omitempty"`
	Observation *ObservationDTO `json:"observation,omitempty"`
	Items       []ContextItem   `json:"items,omitempty"`
	Limits      []string        `json:"limits,omitempty"`
	Error       *ErrorBody      `json:"error,omitempty"`
}

// ErrorBody is a non-sensitive structured error for agents.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SearchHit is a compact search result — never the full observation body.
type SearchHit struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Scope     string `json:"scope"`
	Title     string `json:"title"`
	TopicKey  string `json:"topic_key,omitempty"`
	UpdatedAt string `json:"updated_at"`
	Preview   string `json:"preview,omitempty"`
}

// ObservationDTO is the full observation payload for get.
type ObservationDTO struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	SessionID      string `json:"session_id,omitempty"`
	Type           string `json:"type"`
	Scope          string `json:"scope"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	TopicKey       string `json:"topic_key,omitempty"`
	SourceKind     string `json:"source_kind,omitempty"`
	SourceRef      string `json:"source_ref,omitempty"`
	ContentHash    string `json:"content_hash"`
	DuplicateCount int    `json:"duplicate_count"`
	RevisionCount  int    `json:"revision_count"`
	FirstSeenAt    string `json:"first_seen_at"`
	LastSeenAt     string `json:"last_seen_at"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ContextItem is a budgeted progressive-retrieval reference.
type ContextItem struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Scope    string     `json:"scope"`
	Title    string     `json:"title"`
	TopicKey string     `json:"topic_key,omitempty"`
	Score    int        `json:"score"`
	Excerpt  string     `json:"excerpt,omitempty"`
	Ref      ContextRef `json:"ref"`
}

// ContextRef tells agents how to fetch full detail.
type ContextRef struct {
	Get string `json:"get"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
