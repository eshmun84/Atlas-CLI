package hookproj

import (
	"sync"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
)

var (
	mu                sync.Mutex
	defaultProjectors map[adapters.ID]Projector
)

// RegisterDefaultProjector registers a hooks projector.
func RegisterDefaultProjector(p Projector) {
	if p == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if defaultProjectors == nil {
		defaultProjectors = map[adapters.ID]Projector{}
	}
	defaultProjectors[p.ID()] = p
}

// DefaultProjectors returns a copy of registered projectors.
func DefaultProjectors() map[adapters.ID]Projector {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[adapters.ID]Projector, len(defaultProjectors))
	for k, v := range defaultProjectors {
		out[k] = v
	}
	return out
}
