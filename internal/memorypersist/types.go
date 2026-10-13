package memorypersist

import (
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist/policy"
)

const (
	SchemaVersion  = 1
	CommandPropose = "propose"

	CapabilitySlice = "memory-persist-36c"

	StatusAccepted             = "accepted"
	StatusDuplicate            = "duplicate"
	StatusUpdated              = "updated"
	StatusRejected             = "rejected"
	StatusConfirmationRequired = "confirmation_required"
	StatusDisabled             = "disabled"
	StatusInvalidInput         = "invalid_input"
	StatusInvalidUsage         = "invalid_usage"
	StatusNotInitialized       = "not_initialized"
	StatusIncompatibleSchema   = "incompatible_schema"
	StatusUnsafeFilesystem     = "unsafe_filesystem"
	StatusInternalError        = "internal_error"
	StatusUnavailable          = "unavailable"
)

// Candidate is the structured transport payload for automatic persistence.
// Agents submit this via atlas memory propose --json (stdin JSON).
type Candidate struct {
	Type         string              `json:"type"`
	Title        string              `json:"title"`
	Content      string              `json:"content"`
	Scope        string              `json:"scope,omitempty"`
	TopicKey     string              `json:"topic_key,omitempty"`
	Provenance   string              `json:"provenance,omitempty"`
	Confidence   policy.Confidence   `json:"confidence,omitempty"`
	Explicitness policy.Explicitness `json:"explicitness,omitempty"`
	Origin       policy.Origin       `json:"origin"`
	SourceEvent  string              `json:"source_event,omitempty"`
	// confirm is intentionally absent: the propose transport is untrusted.
	// Inferred candidates always return confirmation_required in V1; a trusted
	// interactive confirmation surface is deferred.
}

// Response is the versioned JSON envelope for propose.
type Response struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	Status        string `json:"status"`
	ProjectID     string `json:"project_id,omitempty"`

	Decision *policy.Decision `json:"decision,omitempty"`
	Action   string           `json:"action,omitempty"` // created | duplicate | updated | none

	Observation *ObservationDTO `json:"observation,omitempty"`
	Error       *ErrorBody      `json:"error,omitempty"`
}

// ErrorBody is a non-sensitive structured error for agents.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ObservationDTO mirrors the retrieval DTO shape for accepted writes.
type ObservationDTO struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
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

func toDTO(o memory.Observation) ObservationDTO {
	return ObservationDTO{
		ID:             o.ID,
		ProjectID:      o.ProjectID,
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
