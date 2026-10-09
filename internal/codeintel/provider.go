package codeintel

import "context"

// Provider is the Atlas-owned Code Intelligence backend contract.
type Provider interface {
	ID() ProviderID
	Probe(ctx context.Context) (Capability, error)
	Status(ctx context.Context, project Project) (ProjectStatus, error)
	// Refresh executes a provider build against Atlas-owned storage.
	// Mode must be RefreshModeIncremental or RefreshModeFull.
	Refresh(ctx context.Context, req RefreshRequest) (RefreshResult, error)
}

// RefreshRequest is the provider-neutral refresh input.
type RefreshRequest struct {
	Root     string
	HomePath string
	DBPath   string
	Mode     RefreshMode
}

// RefreshResult is the provider-neutral outcome of one Refresh call.
type RefreshResult struct {
	Mode       RefreshMode
	Message    string
	NodesTotal int
	FilesTotal int
}
