package codeintel_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestInspectStorageLeaf_SymlinkGraphBlocksRefresh(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	id, err := home.ProjectID(root, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := codeintel.GraphDBPath(homeDir, id, codeintel.ProviderCodeGraph)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	ext := filepath.Join(outside, "graph.db")
	mustWrite(t, ext, "EXTERNAL\n")
	if err := os.Symlink(ext, dbPath); err != nil {
		t.Fatal(err)
	}

	var ran bool
	svc := codeintel.NewService(&refreshStub{
		cap: codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0"},
		refresh: func(req codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
			ran = true
			return codeintel.RefreshResult{Mode: req.Mode, NodesTotal: 1, FilesTotal: 1}, nil
		},
	})
	_, err = svc.Refresh(context.Background(), codeintel.RefreshOptions{
		Root: root, ProjectName: "leaf", Now: func() time.Time { return time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("expected unsafe graph leaf error")
	}
	if ran {
		t.Fatal("provider must not be invoked for unsafe graph.db")
	}
	got, err := os.ReadFile(ext)
	if err != nil || string(got) != "EXTERNAL\n" {
		t.Fatalf("external target must be unchanged: %q err=%v", got, err)
	}
}

func TestLoadMetadata_SymlinkRefused(t *testing.T) {
	homeDir := t.TempDir()
	outside := t.TempDir()
	ext := filepath.Join(outside, "metadata.json")
	mustWrite(t, ext, `{"schemaVersion":1,"provider":"codegraph","projectID":"p","sourceFingerprint":"abc"}`+"\n")
	metaPath := filepath.Join(homeDir, "projects", "p", "codegraph", "metadata.json")
	dbPath := filepath.Join(homeDir, "projects", "p", "codegraph", "graph.db")
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, dbPath, "db\n")
	if err := os.Symlink(ext, metaPath); err != nil {
		t.Fatal(err)
	}
	_, present, err := codeintel.LoadMetadata(homeDir, metaPath)
	if err == nil || present {
		t.Fatalf("expected symlink metadata error, present=%v err=%v", present, err)
	}
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	snap := codeintel.EnrichSnapshot(codeintel.Project{Root: root, ID: "p", HomePath: homeDir}, codeintel.ProjectStatus{
		Capability:   codeintel.Capability{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateAvailable, Version: "3.17.0"},
		GraphPresent: true,
		GraphDBPath:  dbPath,
		MetadataPath: metaPath,
	})
	if snap.Freshness != codeintel.FreshnessError || snap.Freshness == codeintel.FreshnessReady {
		t.Fatalf("want FreshnessError not Ready: %#v", snap)
	}
}

func TestInspectStorageLeaf_RegularOK(t *testing.T) {
	homeDir := t.TempDir()
	path := filepath.Join(homeDir, "projects", "p", "codegraph", "graph.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, "db\n")
	leaf, err := codeintel.InspectStorageLeaf(homeDir, path)
	if err != nil || !leaf.Present {
		t.Fatalf("leaf=%#v err=%v", leaf, err)
	}
}
