package mcp

// Projector is the agent-neutral contract for MCP native projections.
// Implementations live in adapter packages (cursor/opencode) and must not be
// imported by the mcp domain types themselves beyond this interface.
type Projector interface {
	// ID returns the adapter identifier (cursor / opencode).
	ID() AdapterID
	// SupportsMCP reports whether this adapter can host MCP projections.
	SupportsMCP() bool
	// ConfigRelPath is the project-relative native MCP config path.
	ConfigRelPath() string
	// InspectMCPProjection reads the native config without mutating it.
	InspectMCPProjection(root string) (NativeSnapshot, error)
	// BuildMCPProjection maps a normalized definition to a native entry.
	BuildMCPProjection(root string, def Definition) (NativeEntry, error)
	// ApplyMCPProjection merges Atlas-managed entries into the native config.
	// Only keys listed in managedKeys may be created/updated/removed.
	// user-owned keys must be preserved. Malformed configs must block.
	ApplyMCPProjection(root string, entries []NativeEntry, managedKeys []string, removeKeys []string) error
	// RemoveManagedMCPProjection removes only Atlas-managed keys for this adapter.
	RemoveManagedMCPProjection(root string, managedKeys []string) error
}
