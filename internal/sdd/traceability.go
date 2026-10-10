package sdd

import (
	"fmt"
	"strings"
	"time"
)

// ClosedChangeRef is a stable, copy-free reference to an archived change.
//
// Slice 35/36 Memory may later record:
//
//	source = archived <engine> change <change-id>
//
// without copying proposal/spec/design/tasks/verify/archive contents into
// Atlas. This type is the clean hand-off; Memory itself is out of scope.
type ClosedChangeRef struct {
	ProjectID    string
	Engine       EngineID
	ChangeID     string
	Location     string // opaque adapter archived reference
	ArchivedAt   *time.Time
	Verification VerificationState
	SpecRefs     []SpecRef
}

// ClosedChangeFrom builds a ClosedChangeRef from an unequivocally archived ChangeRef.
//
// Required (fail-closed):
//   - Archived == true
//   - Lifecycle == LifecycleArchived
//   - Ambiguous == false
//   - Engine, ChangeID, and Location non-empty
//
// VerificationUnknown is valid and does not block.
func ClosedChangeFrom(ref ChangeRef) (ClosedChangeRef, error) {
	id := strings.TrimSpace(ref.ChangeID)
	loc := strings.TrimSpace(ref.Location)
	switch {
	case !ref.Archived:
		return ClosedChangeRef{}, fmt.Errorf("sdd: change %q is not archived", id)
	case ref.Lifecycle != LifecycleArchived:
		return ClosedChangeRef{}, fmt.Errorf("sdd: archived change %q has lifecycle %q (want archived)", id, ref.Lifecycle)
	case ref.Ambiguous:
		return ClosedChangeRef{}, fmt.Errorf("sdd: archived change %q is ambiguous", id)
	case ref.Engine == "":
		return ClosedChangeRef{}, fmt.Errorf("sdd: archived change missing engine")
	case id == "":
		return ClosedChangeRef{}, fmt.Errorf("sdd: archived change missing change id")
	case loc == "":
		return ClosedChangeRef{}, fmt.Errorf("sdd: archived change %q missing location", id)
	}

	out := ClosedChangeRef{
		ProjectID:    strings.TrimSpace(ref.ProjectID),
		Engine:       ref.Engine,
		ChangeID:     id,
		Location:     loc,
		ArchivedAt:   cloneTime(ref.ArchivedAt),
		Verification: ref.Verification,
		SpecRefs:     append([]SpecRef(nil), ref.SpecRefs...),
	}
	if out.Verification == "" {
		out.Verification = VerificationUnknown
	}
	return out, nil
}

// SourceLabel returns a Memory-oriented source string without embedding content.
func (c ClosedChangeRef) SourceLabel() string {
	eng := string(c.Engine)
	if eng == "" {
		eng = "unknown-engine"
	}
	return fmt.Sprintf("archived %s change %s", eng, c.ChangeID)
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}
