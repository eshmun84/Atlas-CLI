package hooks

import "github.com/eshmun84/Atlas-CLI/internal/adapters"

// OwnershipEntry is the minimal future projection-ownership record for an
// Atlas-managed hook/plugin artifact. Slice 36B.1 does not materialize files;
// later slices must persist this via Home ownership documents and mutate only
// through project/fsafety containment (same pattern as skills/MCP).
type OwnershipEntry struct {
	Adapter  adapters.ID
	RootRel  string // adapter-owned projection root (provider-declared, never invented by Core)
	Digest   string
	Source   string
	EventSet []EventType // catalog events covered by this projection
}

// ConflictError builds an ownership-conflict error for projection layers.
// Foundation dispatch uses ErrDuplicateAction for in-process registration
// conflicts; filesystem ownership conflicts use this sentinel.
func ConflictError(adapter adapters.ID, rootRel string) error {
	if rootRel == "" {
		return ErrOwnershipConflict
	}
	return &OwnershipConflictError{Adapter: adapter, RootRel: rootRel}
}

// OwnershipConflictError carries adapter/path context for projection conflicts.
type OwnershipConflictError struct {
	Adapter adapters.ID
	RootRel string
}

func (e *OwnershipConflictError) Error() string {
	if e == nil {
		return ErrOwnershipConflict.Error()
	}
	if e.RootRel == "" {
		return ErrOwnershipConflict.Error() + ": " + string(e.Adapter)
	}
	return ErrOwnershipConflict.Error() + ": " + string(e.Adapter) + " " + e.RootRel
}

func (e *OwnershipConflictError) Unwrap() error { return ErrOwnershipConflict }
