package hooksengine

import (
	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// LookupBridge returns the Atlas-registered EventBridge for an adapter.
//
// Slice 36B.2 registers no native Cursor/OpenCode bridges yet. Missing bridges
// fail closed: Negotiate receives an empty declaration set (never the full
// canonical catalog). Provider implementations will register here in later
// slices without Core branching on provider names.
func LookupBridge(id adapters.ID) (hooks.EventBridge, bool) {
	bridge, ok := bridgeTable[id]
	if !ok || bridge == nil {
		return nil, false
	}
	return bridge, true
}

// bridgeTable is the closed production registry of adapter EventBridges.
// Empty until native hook projections land. Core never invents declarations.
var bridgeTable = map[adapters.ID]hooks.EventBridge{}

// negotiateAdapter builds capability from ClassHooks + EventBridge declarations.
//
// Rules:
//   - CapabilityOverride is handled by Engine.capabilityFor (test-only).
//   - Missing / nil / identity-mismatched bridges negotiate with zero declarations.
//   - Canonical catalog membership is NOT treated as adapter support.
func negotiateAdapter(id adapters.ID, bridge hooks.EventBridge) hooks.Capability {
	if bridge == nil {
		return hooks.Negotiate(id, nil)
	}
	if bridge.AdapterID() != id {
		return hooks.Negotiate(id, nil)
	}
	return hooks.NegotiateBridge(bridge)
}
