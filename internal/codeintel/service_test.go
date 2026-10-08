package codeintel_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
)

type stubProvider struct {
	id   codeintel.ProviderID
	cap  codeintel.Capability
	stat codeintel.ProjectStatus
}

func (s stubProvider) ID() codeintel.ProviderID { return s.id }
func (s stubProvider) Probe(context.Context) (codeintel.Capability, error) {
	return s.cap, nil
}
func (s stubProvider) Status(_ context.Context, project codeintel.Project) (codeintel.ProjectStatus, error) {
	st := s.stat
	st.ProjectID = project.ID
	return st, nil
}

func TestService_UnknownProvider(t *testing.T) {
	t.Parallel()
	svc := codeintel.NewService(stubProvider{
		id:  codeintel.ProviderCodeGraph,
		cap: codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable},
	})
	_, err := svc.Probe(context.Background(), "other")
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
}

func TestService_DefaultDelegation(t *testing.T) {
	t.Parallel()
	svc := codeintel.NewService(stubProvider{
		id: codeintel.ProviderCodeGraph,
		cap: codeintel.Capability{
			Provider: codeintel.ProviderCodeGraph,
			State:    codeintel.StateAvailable,
			Version:  "3.17.0",
		},
		stat: codeintel.ProjectStatus{
			Capability: codeintel.Capability{
				Provider: codeintel.ProviderCodeGraph,
				State:    codeintel.StateAvailable,
				Version:  "3.17.0",
			},
			GraphPresent: true,
		},
	})
	cap, err := svc.DefaultProbe(context.Background())
	if err != nil || cap.State != codeintel.StateAvailable {
		t.Fatalf("probe=%#v err=%v", cap, err)
	}
	st, err := svc.DefaultStatus(context.Background(), codeintel.Project{ID: "p1"})
	if err != nil || st.State != codeintel.StateAvailable || st.ProjectID != "p1" || !st.GraphPresent {
		t.Fatalf("status=%#v err=%v", st, err)
	}
	snap := svc.Snapshot(context.Background(), codeintel.Project{ID: "p1", Root: t.TempDir()})
	if snap.State != codeintel.StateAvailable || snap.Provider != codeintel.ProviderCodeGraph || !snap.GraphPresent {
		t.Fatalf("snapshot=%#v", snap)
	}
}

func TestStoragePaths_ProjectScoped(t *testing.T) {
	t.Parallel()
	home := "/tmp/atlas-home"
	a := codeintel.ProviderStorageDir(home, "alpha-1111111111111111", codeintel.ProviderCodeGraph)
	b := codeintel.ProviderStorageDir(home, "beta-2222222222222222", codeintel.ProviderCodeGraph)
	if a == b {
		t.Fatal("expected distinct project storage")
	}
	if filepath.Base(a) != "codegraph" || filepath.Base(b) != "codegraph" {
		t.Fatalf("provider dir names: %q %q", a, b)
	}
	if codeintel.GraphDBPath(home, "alpha-1111111111111111", codeintel.ProviderCodeGraph) != filepath.Join(a, "graph.db") {
		t.Fatal("graph path mismatch")
	}
	if codeintel.MetadataPath(home, "alpha-1111111111111111", codeintel.ProviderCodeGraph) != filepath.Join(a, "metadata.json") {
		t.Fatal("metadata path mismatch")
	}
}

func TestStateConstants(t *testing.T) {
	t.Parallel()
	for _, st := range []codeintel.State{
		codeintel.StateMissing,
		codeintel.StateAvailable,
		codeintel.StateReady,
		codeintel.StateStale,
		codeintel.StateIncompatible,
		codeintel.StateUnavailable,
		codeintel.StateError,
	} {
		if st == "" {
			t.Fatal("empty state")
		}
	}
}
