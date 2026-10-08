package codeintel

import "context"

// Provider is the Atlas-owned Code Intelligence backend contract for Slice 31.
// Only Probe and Status are required now. Build/Update/query methods arrive
// when Atlas adds an explicit mutation path and query surfaces.
type Provider interface {
	ID() ProviderID
	Probe(ctx context.Context) (Capability, error)
	Status(ctx context.Context, project Project) (ProjectStatus, error)
}
