package sdd

// InspectRequest is the read-only input for Spec Engine inspection.
type InspectRequest struct {
	Root      string
	ProjectID string
}

// Engine is the Spec Engine abstraction.
//
// Implementations live outside this package (for example under
// internal/specengine). Core SDD must not import provider adapters.
// Mutation/execution is intentionally out of scope — prefer read-only.
type Engine interface {
	// ID returns the opaque engine identifier this adapter owns.
	ID() EngineID

	// Detect reports whether this engine is present at the project root.
	Detect(req InspectRequest) (Presence, error)

	// ListChanges discovers active and archived changes (fail-closed).
	ListChanges(req InspectRequest) (active, archived []ChangeRef, issues []Issue, err error)

	// InspectChange inspects one change by ID when resolvable.
	InspectChange(req InspectRequest, changeID string) (ChangeRef, error)
}
