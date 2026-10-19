package hookactions_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/hookactions"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

type refreshStub struct {
	cap      codeintel.Capability
	refresh  func(codeintel.RefreshRequest) (codeintel.RefreshResult, error)
	calls    int
	failOnce bool
}

func (s *refreshStub) ID() codeintel.ProviderID { return codeintel.ProviderCodeGraph }
func (s *refreshStub) Probe(context.Context) (codeintel.Capability, error) {
	return s.cap, nil
}
func (s *refreshStub) Status(_ context.Context, project codeintel.Project) (codeintel.ProjectStatus, error) {
	st := codeintel.ProjectStatus{
		Capability:   s.cap,
		ProjectID:    project.ID,
		GraphDBPath:  codeintel.GraphDBPath(project.HomePath, project.ID, codeintel.ProviderCodeGraph),
		MetadataPath: codeintel.MetadataPath(project.HomePath, project.ID, codeintel.ProviderCodeGraph),
	}
	st.GraphPresent = fileExists(st.GraphDBPath)
	st.MetadataPresent = fileExists(st.MetadataPath)
	return st, nil
}
func (s *refreshStub) Refresh(_ context.Context, req codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
	s.calls++
	if s.refresh != nil {
		return s.refresh(req)
	}
	if s.failOnce {
		s.failOnce = false
		return codeintel.RefreshResult{}, errors.New("refresh failed")
	}
	if err := os.MkdirAll(filepath.Dir(req.DBPath), 0o755); err != nil {
		return codeintel.RefreshResult{}, err
	}
	if err := os.WriteFile(req.DBPath, []byte("db"), 0o644); err != nil {
		return codeintel.RefreshResult{}, err
	}
	return codeintel.RefreshResult{Mode: req.Mode, NodesTotal: 2, FilesTotal: 1}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func availableCap() codeintel.Capability {
	return codeintel.Capability{
		Provider: codeintel.ProviderCodeGraph,
		State:    codeintel.StateAvailable,
		Version:  "3.17.0",
	}
}

func TestCodeIntel_ProviderUnavailable(t *testing.T) {
	ref := seedInitializedProject(t, "ci-unavail")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{cap: codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateUnavailable}}
	act := &hookactions.CodeIntelAction{
		Resolve: fixedResolve(ref),
		Service: codeintel.NewService(stub),
	}
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err != nil || !out.Skipped || out.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("want skip, got %#v err=%v", out, err)
	}
	if stub.calls != 0 {
		t.Fatal("unavailable must not refresh")
	}
}

func TestCodeIntel_ProviderIncompatible(t *testing.T) {
	ref := seedInitializedProject(t, "ci-incompat")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{cap: codeintel.Capability{
		Provider: codeintel.ProviderCodeGraph,
		State:    codeintel.StateIncompatible,
		Message:  "major 2 unsupported",
	}}
	act := &hookactions.CodeIntelAction{Resolve: fixedResolve(ref), Service: codeintel.NewService(stub)}
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err != nil || !out.Skipped || out.Outcome != hooks.OutcomeSkipped {
		t.Fatalf("want skip, got %#v err=%v", out, err)
	}
	if out.Message == "" {
		t.Fatal("incompatible should surface diagnostic message")
	}
}

func TestCodeIntel_GraphMissingInitial(t *testing.T) {
	ref := seedInitializedProject(t, "ci-missing")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{cap: availableCap()}
	act := &hookactions.CodeIntelAction{Resolve: fixedResolve(ref), Service: codeintel.NewService(stub)}
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err != nil || out.Outcome != hooks.OutcomeOK || out.Noop {
		t.Fatalf("want initial refresh, got %#v err=%v", out, err)
	}
	if out.Mode != codeintel.RefreshModeInitial {
		t.Fatalf("mode=%q", out.Mode)
	}
	if !fileExists(out.GraphDB) {
		t.Fatal("graph.db missing under Home")
	}
	if _, err := os.Stat(filepath.Join(ref.Root, ".codegraph")); !os.IsNotExist(err) {
		t.Fatal("workspace must not gain .codegraph")
	}
}

func TestCodeIntel_GraphStaleIncremental(t *testing.T) {
	ref := seedInitializedProject(t, "ci-stale")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{cap: availableCap()}
	act := &hookactions.CodeIntelAction{Resolve: fixedResolve(ref), Service: codeintel.NewService(stub)}
	if _, err := act.Maintain(context.Background(), mustRef(t, ref)); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ref.Root, "a.go"), "package a\nfunc X(){}\n")
	stub.calls = 0
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err != nil || out.Mode != codeintel.RefreshModeIncremental {
		t.Fatalf("want incremental, got %#v err=%v", out, err)
	}
	if stub.calls != 1 {
		t.Fatalf("calls=%d", stub.calls)
	}
}

func TestCodeIntel_GraphFreshNoop(t *testing.T) {
	ref := seedInitializedProject(t, "ci-fresh")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{cap: availableCap()}
	act := &hookactions.CodeIntelAction{Resolve: fixedResolve(ref), Service: codeintel.NewService(stub)}
	if _, err := act.Maintain(context.Background(), mustRef(t, ref)); err != nil {
		t.Fatal(err)
	}
	stub.calls = 0
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err != nil || !out.Noop || out.Outcome != hooks.OutcomeOK {
		t.Fatalf("want fresh noop, got %#v err=%v", out, err)
	}
	if stub.calls != 0 {
		t.Fatal("fresh must not call provider refresh")
	}
}

func TestCodeIntel_RefreshFailure(t *testing.T) {
	ref := seedInitializedProject(t, "ci-fail")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{
		cap: availableCap(),
		refresh: func(codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
			return codeintel.RefreshResult{}, errors.New("boom")
		},
	}
	act := &hookactions.CodeIntelAction{Resolve: fixedResolve(ref), Service: codeintel.NewService(stub)}
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err == nil || out.Outcome != hooks.OutcomeFailed {
		t.Fatalf("want failure, got %#v err=%v", out, err)
	}
}

func TestCodeIntel_Timeout(t *testing.T) {
	ref := seedInitializedProject(t, "ci-timeout")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{cap: availableCap()}
	act := &hookactions.CodeIntelAction{Resolve: fixedResolve(ref), Service: codeintel.NewService(stub)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	out, err := act.Maintain(ctx, mustRef(t, ref))
	if err == nil || out.Outcome != hooks.OutcomeCancelled {
		t.Fatalf("want cancelled, got %#v err=%v", out, err)
	}
}

func TestCodeIntel_NoWorkspaceContamination(t *testing.T) {
	ref := seedInitializedProject(t, "ci-clean")
	write(t, filepath.Join(ref.Root, "a.go"), "package a\n")
	stub := &refreshStub{
		cap: availableCap(),
		refresh: func(req codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
			// Simulate provider side-effect under workspace; Service containment should clean it.
			side := filepath.Join(req.Root, ".codegraph", "changes.journal")
			if err := os.MkdirAll(filepath.Dir(side), 0o755); err != nil {
				return codeintel.RefreshResult{}, err
			}
			if err := os.WriteFile(side, []byte("j"), 0o644); err != nil {
				return codeintel.RefreshResult{}, err
			}
			if err := os.MkdirAll(filepath.Dir(req.DBPath), 0o755); err != nil {
				return codeintel.RefreshResult{}, err
			}
			if err := os.WriteFile(req.DBPath, []byte("db"), 0o644); err != nil {
				return codeintel.RefreshResult{}, err
			}
			return codeintel.RefreshResult{Mode: req.Mode, NodesTotal: 1, FilesTotal: 1}, nil
		},
	}
	act := &hookactions.CodeIntelAction{Resolve: fixedResolve(ref), Service: codeintel.NewService(stub)}
	out, err := act.Maintain(context.Background(), mustRef(t, ref))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Containment.JournalRemoved && !out.Containment.DirRemoved {
		// Containment should have removed the side-effect we planted.
		if _, err := os.Stat(filepath.Join(ref.Root, ".codegraph")); !os.IsNotExist(err) {
			t.Fatalf("workspace still contaminated; containment=%#v", out.Containment)
		}
	}
	if _, err := os.Stat(filepath.Join(ref.Root, ".codegraph")); !os.IsNotExist(err) {
		t.Fatal(".codegraph must not remain in workspace")
	}
}

func mustRef(t *testing.T, f projectFixture) hookactions.ProjectRef {
	t.Helper()
	return hookactions.ProjectRef{
		Root: f.Root, Home: f.Home, ProjectID: f.ProjectID, Name: f.Name, State: f.State,
	}
}
