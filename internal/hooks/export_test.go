package hooks

import "github.com/eshmun84/Atlas-CLI/internal/adapters"

// BuildCapabilityForTest exposes capability construction for unit tests without
// mutating the public adapters.Capabilities table.
func BuildCapabilityForTest(id adapters.ID, hooksCapable bool, declared []EventType) Capability {
	return buildCapability(id, hooksCapable, declared)
}
