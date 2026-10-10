// Package memory implements Atlas-native durable Memory Core persistence
// (SQLite + FTS5) under Atlas Home. It stores distilled knowledge — not chat
// transcripts, full source trees, or regenerable state.
//
// Slice 35: persistence and model only. Runtime retrieval belongs to Slice 36.
// This package does not import Spec Engine adapters or tool-bridge layers.
package memory

import "time"

// ObservationType is the closed V1 taxonomy.
type ObservationType string

const (
	TypeDecision     ObservationType = "decision"
	TypeArchitecture ObservationType = "architecture"
	TypeConstraint   ObservationType = "constraint"
	TypeDiscovery    ObservationType = "discovery"
	TypeBugfix       ObservationType = "bugfix"
	TypePattern      ObservationType = "pattern"
	TypeConfig       ObservationType = "config"
	TypePreference   ObservationType = "preference"
	TypeFact         ObservationType = "fact"
)

// Scope is the closed V1 visibility domain.
//
// V1 supports only ScopeProject because storage is project-scoped
// ($ATLAS_HOME/projects/<project-id>/memory/memory.db). Cross-project
// "global" memory is deferred and must not be implied by this API.
type Scope string

const (
	ScopeProject Scope = "project"
)

// SaveAction reports how SaveObservation mutated the store.
type SaveAction string

const (
	SaveCreated   SaveAction = "created"
	SaveDuplicate SaveAction = "duplicate"
	SaveUpdated   SaveAction = "updated"
)

// Observation is the durable Memory unit.
type Observation struct {
	ID             string
	ProjectID      string
	SessionID      string // optional
	Type           ObservationType
	Scope          Scope
	Title          string
	Content        string
	TopicKey       string // optional evolutionary key
	SourceKind     string // optional opaque source class (e.g. future "sdd-change")
	SourceRef      string // optional opaque source reference
	ContentHash    string
	DuplicateCount int
	RevisionCount  int
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Session is a Memory association window — not a chat transcript.
type Session struct {
	ID        string
	ProjectID string
	StartedAt time.Time
	EndedAt   *time.Time
	Summary   string // optional; Slice 35 does not generate summaries
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SaveResult describes the outcome of SaveObservation.
type SaveResult struct {
	Action      SaveAction
	Observation Observation
}

// ListFilter selects observations for ListObservations.
type ListFilter struct {
	ProjectID string
	Scope     Scope           // empty = any
	Type      ObservationType // empty = any
	TopicKey  string          // empty = any; exact match when set
	Limit     int             // 0 = default
	Offset    int
}

// SearchQuery is a full-text search request.
type SearchQuery struct {
	ProjectID string
	Query     string
	Limit     int // 0 = default
}
