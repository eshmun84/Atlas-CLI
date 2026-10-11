package hooksengine

import (
	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// ClassifyForTest exposes error classification for unit tests.
func ClassifyForTest(err error) (status, message string) {
	return classify(err)
}

// CapabilityForTest exposes production negotiation (BridgeLookup / LookupBridge),
// ignoring CapabilityOverride on a copy of the engine.
func (e Engine) CapabilityForTest(id adapters.ID) hooks.Capability {
	e.CapabilityOverride = nil
	return e.capabilityFor(id)
}

// NegotiateAdapterForTest exposes bridge negotiation rules for unit tests.
func NegotiateAdapterForTest(id adapters.ID, bridge hooks.EventBridge) hooks.Capability {
	return negotiateAdapter(id, bridge)
}
