package skills

import (
	"sync"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
)

// Projector is the adapter-owned skill projection contract.
// Implementations live under internal/adapters/<provider>/; Core never branches
// on provider names and must not host provider-specific packages.
type Projector interface {
	ID() adapters.ID
	SupportsSkills() bool
	// SkillsRootRel is the project-relative root for skill packages.
	SkillsRootRel() string
	// PackageRootRel returns the projection directory for one skill id.
	PackageRootRel(skillID string) string
}

var (
	defaultMu         sync.Mutex
	defaultProjectors map[adapters.ID]Projector
)

// RegisterDefaultProjector registers a skills projector.
func RegisterDefaultProjector(p Projector) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultProjectors == nil {
		defaultProjectors = map[adapters.ID]Projector{}
	}
	defaultProjectors[p.ID()] = p
}

// DefaultProjectors returns a copy of registered projectors.
func DefaultProjectors() map[adapters.ID]Projector {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	out := make(map[adapters.ID]Projector, len(defaultProjectors))
	for k, v := range defaultProjectors {
		out[k] = v
	}
	return out
}
