package codegraphsource_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/codegraphsource"
)

func TestRetrieve_FirstQueryDoesNotMutateLiveStorage(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	cgDir := codeintel.ProviderStorageDir(homePath, projectID, codeintel.ProviderCodeGraph)
	before := snapshotTree(t, cgDir)

	var queriedDB string
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			for i := 0; i+1 < len(args); i++ {
				if args[i] == "--db" {
					queriedDB = args[i+1]
				}
			}
			// Simulate CodeGraph/SQLite creating WAL/SHM beside the DB it opens.
			if queriedDB != "" {
				_ = os.WriteFile(queriedDB+"-wal", []byte{}, 0o600)
				_ = os.WriteFile(queriedDB+"-shm", make([]byte, 32), 0o600)
			}
			return codegraph.RunResult{Stdout: []byte(validQueryJSON("helper"))}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
	after := snapshotTree(t, cgDir)
	if before != after {
		t.Fatalf("live CodeGraph storage mutated:\nbefore=%s\nafter=%s", before, after)
	}
	liveDB := codeintel.GraphDBPath(homePath, projectID, codeintel.ProviderCodeGraph)
	if queriedDB == "" || queriedDB == liveDB {
		t.Fatalf("expected query against isolated snapshot, got %q (live=%q)", queriedDB, liveDB)
	}
	if !strings.Contains(queriedDB, "atlas-cg-query-") {
		t.Fatalf("snapshot path unexpected: %q", queriedDB)
	}
	// Snapshot temp dir must be cleaned up after Retrieve returns.
	if _, err := os.Lstat(filepath.Dir(queriedDB)); !os.IsNotExist(err) {
		t.Fatalf("snapshot dir not cleaned: %v", err)
	}
}

func TestRetrieve_SnapshotCleanupOnQueryFailure(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	var queriedDB string
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			for i := 0; i+1 < len(args); i++ {
				if args[i] == "--db" {
					queriedDB = args[i+1]
					_ = os.WriteFile(queriedDB+"-wal", []byte("x"), 0o600)
				}
			}
			return codegraph.RunResult{ExitCode: 2}, codegraph.ErrProcessFailed
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if queriedDB == "" {
		t.Fatal("expected snapshot query path")
	}
	if _, err := os.Lstat(filepath.Dir(queriedDB)); !os.IsNotExist(err) {
		t.Fatalf("snapshot dir not cleaned after failure: %v", err)
	}
	cgDir := codeintel.ProviderStorageDir(homePath, projectID, codeintel.ProviderCodeGraph)
	entries, err := os.ReadDir(cgDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
			t.Fatalf("live storage gained sidecar %s", name)
		}
	}
}
