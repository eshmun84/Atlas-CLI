package codeintel_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

type refreshStub struct {
	cap      codeintel.Capability
	stat     codeintel.ProjectStatus
	refresh  func(codeintel.RefreshRequest) (codeintel.RefreshResult, error)
	calls    int
	lastMode codeintel.RefreshMode
}

func (s *refreshStub) ID() codeintel.ProviderID { return codeintel.ProviderCodeGraph }
func (s *refreshStub) Probe(context.Context) (codeintel.Capability, error) {
	return s.cap, nil
}
func (s *refreshStub) Status(_ context.Context, project codeintel.Project) (codeintel.ProjectStatus, error) {
	st := s.stat
	st.ProjectID = project.ID
	st.GraphDBPath = codeintel.GraphDBPath(project.HomePath, project.ID, codeintel.ProviderCodeGraph)
	st.MetadataPath = codeintel.MetadataPath(project.HomePath, project.ID, codeintel.ProviderCodeGraph)
	st.GraphPresent = fileExists(st.GraphDBPath)
	st.MetadataPresent = fileExists(st.MetadataPath)
	return st, nil
}
func (s *refreshStub) Refresh(_ context.Context, req codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
	s.calls++
	s.lastMode = req.Mode
	if s.refresh != nil {
		return s.refresh(req)
	}
	if err := os.MkdirAll(filepath.Dir(req.DBPath), 0o755); err != nil {
		return codeintel.RefreshResult{}, err
	}
	if err := os.WriteFile(req.DBPath, []byte("db"), 0o644); err != nil {
		return codeintel.RefreshResult{}, err
	}
	return codeintel.RefreshResult{Mode: req.Mode, NodesTotal: 2, FilesTotal: 1, Message: "ok"}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func TestRefresh_UnavailableNoMutation(t *testing.T) {
	t.Setenv(home.EnvAtlasHome, t.TempDir())
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	stub := &refreshStub{cap: codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateUnavailable}}
	svc := codeintel.NewService(stub)
	out, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Blocked || stub.calls != 0 {
		t.Fatalf("want blocked no-op %#v calls=%d", out, stub.calls)
	}
}

func TestRefresh_InitialIncrementalFullNoop(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	stub := &refreshStub{cap: codeintel.Capability{
		Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0",
	}}
	svc := codeintel.NewService(stub)
	fixed := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	out, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo", Now: func() time.Time { return fixed }})
	if err != nil || out.Noop || out.Mode != codeintel.RefreshModeInitial {
		t.Fatalf("initial %#v err=%v", out, err)
	}
	if !fileExists(out.GraphDBPath) || !fileExists(out.MetadataPath) {
		t.Fatal("expected graph+metadata")
	}
	metaBytes, _ := os.ReadFile(out.MetadataPath)

	// No-op when fresh.
	stub.calls = 0
	out2, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo"})
	if err != nil || !out2.Noop || stub.calls != 0 {
		t.Fatalf("noop %#v err=%v calls=%d", out2, err, stub.calls)
	}

	// Stale after content change → incremental.
	mustWrite(t, filepath.Join(root, "a.go"), "package a\nfunc X(){}\n")
	out3, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo", Now: func() time.Time { return fixed.Add(time.Minute) }})
	if err != nil || out3.Mode != codeintel.RefreshModeIncremental {
		t.Fatalf("incremental %#v err=%v", out3, err)
	}

	// Force full.
	out4, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo", ForceFull: true, Now: func() time.Time { return fixed.Add(2 * time.Minute) }})
	if err != nil || out4.Mode != codeintel.RefreshModeFull || stub.lastMode != codeintel.RefreshModeFull {
		t.Fatalf("full %#v last=%s err=%v", out4, stub.lastMode, err)
	}

	// Failure preserves prior metadata.
	good := append([]byte(nil), metaBytes...)
	_ = good
	stub.refresh = func(codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
		return codeintel.RefreshResult{}, os.ErrPermission
	}
	beforeMeta, _ := os.ReadFile(out.MetadataPath)
	_, err = svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo", ForceFull: true})
	if err == nil {
		t.Fatal("expected failure")
	}
	afterMeta, _ := os.ReadFile(out.MetadataPath)
	if string(beforeMeta) != string(afterMeta) {
		t.Fatal("metadata must be preserved on failure")
	}
}

func TestEnrichSnapshot_Freshness(t *testing.T) {
	t.Setenv(home.EnvAtlasHome, t.TempDir())
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	homePath := os.Getenv(home.EnvAtlasHome)
	id, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	db := codeintel.GraphDBPath(homePath, id, codeintel.ProviderCodeGraph)
	metaPath := codeintel.MetadataPath(homePath, id, codeintel.ProviderCodeGraph)
	st := codeintel.ProjectStatus{
		Capability:   codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0"},
		GraphDBPath:  db,
		MetadataPath: metaPath,
	}
	snap := codeintel.EnrichSnapshot(codeintel.Project{Root: root, ID: id, HomePath: homePath}, st)
	if snap.Freshness != codeintel.FreshnessMissing {
		t.Fatalf("want missing got %#v", snap)
	}

	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, db, "db")
	st.GraphPresent = true
	fp, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	meta := codeintel.Metadata{
		SchemaVersion:     codeintel.MetadataSchemaVersion,
		Provider:          "codegraph",
		ProjectID:         id,
		SourceFingerprint: fp.Value,
		RefreshedAt:       "2026-10-08T12:00:00Z",
		RefreshMode:       "initial",
	}
	if err := codeintel.WriteMetadataAtomic(homePath, metaPath, meta); err != nil {
		t.Fatal(err)
	}
	st.MetadataPresent = true
	snap = codeintel.EnrichSnapshot(codeintel.Project{Root: root, ID: id, HomePath: homePath}, st)
	if snap.Freshness != codeintel.FreshnessReady {
		t.Fatalf("want ready %#v", snap)
	}
	mustWrite(t, filepath.Join(root, "a.go"), "package a\nfunc Z(){}\n")
	snap = codeintel.EnrichSnapshot(codeintel.Project{Root: root, ID: id, HomePath: homePath}, st)
	if snap.Freshness != codeintel.FreshnessStale {
		t.Fatalf("want stale %#v", snap)
	}
}

func TestWriteMetadataAtomic(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	path := filepath.Join(homeDir, "projects", "p", "codeintel", "metadata.json")
	meta := codeintel.Metadata{
		SchemaVersion: codeintel.MetadataSchemaVersion, Provider: "codegraph", ProjectID: "p", SourceFingerprint: "abc",
	}
	if err := codeintel.WriteMetadataAtomic(homeDir, path, meta); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got codeintel.Metadata
	if err := json.Unmarshal(raw, &got); err != nil || got.Provider != "codegraph" {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestWriteMetadataAtomic_OutsideHomeRefused(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "metadata.json")
	meta := codeintel.Metadata{
		SchemaVersion: codeintel.MetadataSchemaVersion, Provider: "codegraph", ProjectID: "p", SourceFingerprint: "abc",
	}
	if err := codeintel.WriteMetadataAtomic(homeDir, path, meta); err == nil {
		t.Fatal("expected escape error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("must not write outside home: %v", err)
	}
}

func TestWriteMetadataAtomic_SymlinkParentEscape(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(homeDir, "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(link, "metadata.json")
	meta := codeintel.Metadata{
		SchemaVersion: codeintel.MetadataSchemaVersion, Provider: "codegraph", ProjectID: "p", SourceFingerprint: "abc",
	}
	if err := codeintel.WriteMetadataAtomic(homeDir, path, meta); err == nil {
		t.Fatal("expected symlink containment error")
	}
	if _, err := os.Stat(filepath.Join(outside, "metadata.json")); !os.IsNotExist(err) {
		t.Fatalf("must not write through symlink escape: %v", err)
	}
}

func TestWriteMetadataAtomic_AlternateHome(t *testing.T) {
	t.Parallel()
	altHome := t.TempDir()
	path := filepath.Join(altHome, "projects", "alt", "metadata.json")
	meta := codeintel.Metadata{
		SchemaVersion: codeintel.MetadataSchemaVersion, Provider: "codegraph", ProjectID: "alt", SourceFingerprint: "xyz",
	}
	if err := codeintel.WriteMetadataAtomic(altHome, path, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestContainment_NewJournalRemoved(t *testing.T) {
	// Exercised via Refresh with journal side-effect simulation.
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	stub := &refreshStub{
		cap: codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0"},
		refresh: func(req codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
			if err := os.MkdirAll(filepath.Dir(req.DBPath), 0o755); err != nil {
				return codeintel.RefreshResult{}, err
			}
			if err := os.WriteFile(req.DBPath, []byte("db"), 0o644); err != nil {
				return codeintel.RefreshResult{}, err
			}
			mustWrite(t, filepath.Join(root, ".codegraph", "changes.journal"), "j\n")
			return codeintel.RefreshResult{Mode: req.Mode, NodesTotal: 1, FilesTotal: 1}, nil
		},
	}
	svc := codeintel.NewService(stub)
	out, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Containment.JournalRemoved || !out.Containment.DirRemoved {
		t.Fatalf("want journal+dir removed %#v", out.Containment)
	}
	if _, err := os.Stat(filepath.Join(root, ".codegraph")); !os.IsNotExist(err) {
		t.Fatal(".codegraph should be gone")
	}
}

func TestContainment_PreexistingPreserved(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	mustWrite(t, filepath.Join(root, ".codegraph", "developer-owned.txt"), "keep\n")
	mustWrite(t, filepath.Join(root, ".codegraph", "changes.journal"), "old\n")
	stub := &refreshStub{
		cap: codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0"},
		refresh: func(req codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
			if err := os.MkdirAll(filepath.Dir(req.DBPath), 0o755); err != nil {
				return codeintel.RefreshResult{}, err
			}
			if err := os.WriteFile(req.DBPath, []byte("db"), 0o644); err != nil {
				return codeintel.RefreshResult{}, err
			}
			mustWrite(t, filepath.Join(root, ".codegraph", "changes.journal"), "new\n")
			mustWrite(t, filepath.Join(root, ".codegraph", "unknown.bin"), "x")
			return codeintel.RefreshResult{Mode: req.Mode, NodesTotal: 1, FilesTotal: 1}, nil
		},
	}
	svc := codeintel.NewService(stub)
	out, err := svc.Refresh(context.Background(), codeintel.RefreshOptions{Root: root, ProjectName: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Containment.JournalRemoved {
		t.Fatal("preexisting journal must be preserved")
	}
	body, _ := os.ReadFile(filepath.Join(root, ".codegraph", "developer-owned.txt"))
	if string(body) != "keep\n" {
		t.Fatal("developer-owned must survive")
	}
	if _, err := os.Stat(filepath.Join(root, ".codegraph", "unknown.bin")); err != nil {
		t.Fatal("unknown file must not be deleted")
	}
}
