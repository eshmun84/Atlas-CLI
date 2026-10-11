package hooksengine

import (
	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// LookupBridge returns the Atlas-registered EventBridge for an adapter.
//
// Missing bridges fail closed: Negotiate receives an empty declaration set
// (never the full canonical catalog). Adapters register via RegisterBridge
// from config init without Core branching on provider names.
func LookupBridge(id adapters.ID) (hooks.EventBridge, bool) {
	bridge, ok := bridgeTable[id]
	if !ok || bridge == nil {
		return nil, false
	}
	return bridge, true
}

// RegisterBridge installs a production EventBridge for an adapter.
// Nil bridges and empty adapter IDs are ignored. Last registration wins.
func RegisterBridge(bridge hooks.EventBridge) {
	if bridge == nil {
		return
	}
	id := bridge.AdapterID()
	if id == "" {
		return
	}
	if bridgeTable == nil {
		bridgeTable = map[adapters.ID]hooks.EventBridge{}
	}
	bridgeTable[id] = bridge
}

// bridgeTable is the closed production registry of adapter EventBridges.
// Populated by adapter registration (config init). Core never invents declarations.
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
