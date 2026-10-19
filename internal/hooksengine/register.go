package hooksengine

import (
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// DefaultRegistry returns the Atlas-owned action registry for the engine.
//
// Phase B registers Context Economy and Code Intelligence maintenance Actions.
// session.start admits Home maintenance via a detached worker (see hookactions).
// Phase B.8 adds a Cursor-only beforeSubmitPrompt fallback evaluated in Engine
// (not via Action subscription to task.start). Memory / Governance / Testing
// automatic Actions are still not registered.
//
// Callers (tests) may supply a custom Registry via Engine.Registry.
func DefaultRegistry() hooks.Registry {
	reg, err := hooks.NewRegistry(hookactions.Registrations())
	if err != nil {
		// Registrations are static and must validate; fail closed to empty.
		return hooks.Registry{}
	}
	return reg
}
