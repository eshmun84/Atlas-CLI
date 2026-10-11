package hooksengine

import "github.com/eshmun84/Atlas-CLI/internal/hooks"

// DefaultRegistry returns the Atlas-owned action registry for the engine.
//
// Slice 36B.2 ships an empty registry on purpose: no Memory, Governance, or
// Testing actions are registered yet, and no placeholder production actions
// are invented to manufacture successful side effects. Callers (tests) may
// supply a custom Registry via Engine.Registry.
func DefaultRegistry() hooks.Registry {
	reg, err := hooks.NewRegistry(nil)
	if err != nil {
		// NewRegistry(nil) cannot fail with the current foundation.
		return hooks.Registry{}
	}
	return reg
}
