package mcp

import (
	"sync"
)

var (
	defaultMu         sync.Mutex
	defaultProjectors map[AdapterID]Projector
)

// RegisterDefaultProjector registers a projector used by Apply/Status/Doctor helpers.
func RegisterDefaultProjector(p Projector) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultProjectors == nil {
		defaultProjectors = map[AdapterID]Projector{}
	}
	defaultProjectors[p.ID()] = p
}

// DefaultProjectors returns a copy of registered projectors.
func DefaultProjectors() map[AdapterID]Projector {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	out := make(map[AdapterID]Projector, len(defaultProjectors))
	for k, v := range defaultProjectors {
		out[k] = v
	}
	return out
}
