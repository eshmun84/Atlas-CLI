package codeintel

import (
	"context"
	"fmt"
	"strings"
)

// Service is the Atlas-owned Code Intelligence facade over registered providers.
type Service struct {
	providers map[ProviderID]Provider
	defaultID ProviderID
}

// NewService constructs a service with the given providers.
// The first provider is the default when DefaultProbe/DefaultStatus are used.
func NewService(providers ...Provider) *Service {
	s := &Service{
		providers: make(map[ProviderID]Provider, len(providers)),
	}
	for i, p := range providers {
		if p == nil {
			continue
		}
		id := p.ID()
		s.providers[id] = p
		if i == 0 && s.defaultID == "" {
			s.defaultID = id
		}
	}
	return s
}

// DefaultID returns the default provider id.
func (s *Service) DefaultID() ProviderID {
	if s == nil {
		return ""
	}
	return s.defaultID
}

// Probe asks one provider for availability/compatibility. Read-only.
func (s *Service) Probe(ctx context.Context, id ProviderID) (Capability, error) {
	p, err := s.provider(id)
	if err != nil {
		return Capability{
			Provider: id,
			State:    StateUnavailable,
			Message:  err.Error(),
		}, err
	}
	cap, err := p.Probe(ctx)
	if cap.Provider == "" {
		cap.Provider = p.ID()
	}
	return cap, err
}

// Status asks one provider for project-scoped readiness. Read-only.
func (s *Service) Status(ctx context.Context, id ProviderID, project Project) (ProjectStatus, error) {
	p, err := s.provider(id)
	if err != nil {
		return ProjectStatus{
			Capability: Capability{
				Provider: id,
				State:    StateUnavailable,
				Message:  err.Error(),
			},
			ProjectID: project.ID,
		}, err
	}
	st, err := p.Status(ctx, project)
	if st.Provider == "" {
		st.Provider = p.ID()
	}
	if st.ProjectID == "" {
		st.ProjectID = project.ID
	}
	return st, err
}

// DefaultProbe probes the default provider.
func (s *Service) DefaultProbe(ctx context.Context) (Capability, error) {
	return s.Probe(ctx, s.DefaultID())
}

// DefaultStatus statuses the default provider for a project.
func (s *Service) DefaultStatus(ctx context.Context, project Project) (ProjectStatus, error) {
	return s.Status(ctx, s.DefaultID(), project)
}

// Snapshot builds a Status/Doctor-friendly summary for the default provider.
// Never fails Atlas: provider errors become StateError snapshots.
// Read-only: never builds, never writes metadata, never creates Atlas Home.
func (s *Service) Snapshot(ctx context.Context, project Project) Snapshot {
	if s == nil || s.DefaultID() == "" {
		return Snapshot{
			Applicable: true,
			State:      StateUnavailable,
			Message:    "no Code Intelligence provider registered",
		}
	}
	id := s.DefaultID()
	if strings.TrimSpace(project.Root) == "" && strings.TrimSpace(project.ID) == "" {
		cap, err := s.Probe(ctx, id)
		snap := Snapshot{
			Applicable: true,
			Provider:   cap.Provider,
			State:      cap.State,
			Version:    cap.Version,
			Executable: cap.Executable,
			Message:    cap.Message,
		}
		if err != nil && snap.Message == "" {
			snap.Message = err.Error()
		}
		if snap.State == "" {
			snap.State = StateError
		}
		return snap
	}
	st, err := s.Status(ctx, id, project)
	if err != nil && st.State == "" {
		return Snapshot{
			Applicable: true,
			Provider:   id,
			State:      StateError,
			Message:    err.Error(),
		}
	}
	snap := EnrichSnapshot(project, st)
	if err != nil && snap.Message == "" {
		snap.Message = err.Error()
	}
	if snap.State == "" {
		snap.State = StateError
	}
	return snap
}

func (s *Service) provider(id ProviderID) (Provider, error) {
	if s == nil || len(s.providers) == 0 {
		return nil, fmt.Errorf("codeintel: no providers registered")
	}
	id = ProviderID(strings.TrimSpace(string(id)))
	if id == "" {
		id = s.defaultID
	}
	p, ok := s.providers[id]
	if !ok || p == nil {
		return nil, fmt.Errorf("codeintel: unknown provider %q", id)
	}
	return p, nil
}
