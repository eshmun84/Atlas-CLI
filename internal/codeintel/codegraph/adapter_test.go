package codegraph_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
)

type fakeRunner struct {
	calls [][]string
	fn    func(ctx context.Context, executable string, args []string) (codegraph.RunResult, error)
}

func (f *fakeRunner) Run(ctx context.Context, executable string, args []string) (codegraph.RunResult, error) {
	cp := append([]string{executable}, args...)
	f.calls = append(f.calls, cp)
	if f.fn != nil {
		return f.fn(ctx, executable, args)
	}
	return codegraph.RunResult{}, nil
}

func TestProbe_ExecutableMissing(t *testing.T) {
	t.Parallel()
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Runner:   &fakeRunner{},
	}
	cap, err := a.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if cap.State != codeintel.StateUnavailable {
		t.Fatalf("state=%s want unavailable", cap.State)
	}
}

func TestProbe_UsesVersionFlag(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{fn: func(context.Context, string, []string) (codegraph.RunResult, error) {
		return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
	}}
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "/usr/bin/codegraph", nil },
		Runner:   runner,
	}
	cap, err := a.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if cap.State != codeintel.StateAvailable || cap.Version != "3.17.0" {
		t.Fatalf("%#v", cap)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls=%d want exactly one --version process", len(runner.calls))
	}
	args := runner.calls[0][1:]
	if len(args) != 1 || args[0] != "--version" {
		t.Fatalf("want [--version], got %#v", args)
	}
	if len(cap.Capabilities) != 1 || cap.Capabilities[0] != codegraph.ProbedCapabilityVersion {
		t.Fatalf("Capabilities=%v want only runtime-probed %q", cap.Capabilities, codegraph.ProbedCapabilityVersion)
	}
	for _, banned := range []string{"build", "stats", "query", "db-flag", "json-flag", "--db", "--json"} {
		for _, got := range cap.Capabilities {
			if got == banned {
				t.Fatalf("Probe must not announce unverified capability %q", banned)
			}
		}
	}
	for _, call := range runner.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, " version") || strings.Contains(joined, "status") {
			t.Fatalf("must not use inventados version/status subcommands: %s", joined)
		}
		if strings.Contains(joined, "sh -c") || strings.Contains(joined, "npm") || strings.Contains(joined, "brew") {
			t.Fatalf("shell/package-manager invocation: %s", joined)
		}
		// -v is --verbose upstream; Probe must use --version.
		if len(call) > 1 && call[1] == "-v" {
			t.Fatal("must not use -v (verbose) for version probing")
		}
	}
}

func TestProbe_IncompatibleMajor(t *testing.T) {
	t.Parallel()
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(context.Context, string, []string) (codegraph.RunResult, error) {
			return codegraph.RunResult{Stdout: []byte("2.9.0")}, nil
		}},
	}
	cap, err := a.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if cap.State != codeintel.StateIncompatible {
		t.Fatalf("state=%s want incompatible", cap.State)
	}
}

func TestProbe_Timeout(t *testing.T) {
	t.Parallel()
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(context.Context, string, []string) (codegraph.RunResult, error) {
			return codegraph.RunResult{}, codegraph.ErrTimeout
		}},
	}
	cap, err := a.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if cap.State != codeintel.StateError {
		t.Fatalf("state=%s want error", cap.State)
	}
}

func TestProbe_ProcessFailure(t *testing.T) {
	t.Parallel()
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(context.Context, string, []string) (codegraph.RunResult, error) {
			return codegraph.RunResult{Stderr: []byte("boom"), ExitCode: 2}, codegraph.ErrProcessFailed
		}},
	}
	cap, err := a.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if cap.State != codeintel.StateError || !strings.Contains(cap.Message, "boom") {
		t.Fatalf("%#v", cap)
	}
}

func TestProbe_InvalidVersionOutput(t *testing.T) {
	t.Parallel()
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(context.Context, string, []string) (codegraph.RunResult, error) {
			return codegraph.RunResult{Stdout: []byte("not-a-version")}, nil
		}},
	}
	cap, err := a.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if cap.State != codeintel.StateError {
		t.Fatalf("state=%s", cap.State)
	}
}

func TestStatus_ReadOnlyNoArtifactsAndNoStatusCommand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	homePath := t.TempDir()
	projectID := "demo-aaaaaaaaaaaaaaaa"
	runner := &fakeRunner{fn: func(context.Context, string, []string) (codegraph.RunResult, error) {
		return codegraph.RunResult{Stdout: []byte("3.17.0")}, nil
	}}
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner:   runner,
	}
	beforeHome := walkFiles(t, homePath)
	beforeRoot := walkFiles(t, root)
	st, err := a.Status(context.Background(), codeintel.Project{
		Root: root, ID: projectID, HomePath: homePath,
	})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	afterHome := walkFiles(t, homePath)
	afterRoot := walkFiles(t, root)
	if len(beforeHome) != len(afterHome) || len(beforeRoot) != len(afterRoot) {
		t.Fatal("Status mutated filesystem")
	}
	if st.GraphPresent || st.MetadataPresent {
		t.Fatal("unexpected artifacts reported present")
	}
	if st.State != codeintel.StateAvailable {
		t.Fatalf("state=%s want available (graph absence must not invent ready/stale)", st.State)
	}
	if len(st.Capabilities) != 1 || st.Capabilities[0] != codegraph.ProbedCapabilityVersion {
		t.Fatalf("Status Capabilities=%v must mirror Probe (only version)", st.Capabilities)
	}
	if _, err := os.Stat(st.GraphDBPath); !os.IsNotExist(err) {
		t.Fatalf("graph.db should not exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codegraph")); !os.IsNotExist(err) {
		t.Fatal(".codegraph must not be created in project root")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("Status must not add processes beyond Probe --version: calls=%d", len(runner.calls))
	}
	for _, call := range runner.calls {
		for _, arg := range call {
			if arg == "status" || arg == "stats" || arg == "build" || arg == "query" {
				t.Fatalf("Status must not execute graph commands in Slice 31: %#v", call)
			}
		}
	}
}

func TestStatus_GraphPresenceDoesNotInventFreshness(t *testing.T) {
	t.Parallel()
	homePath := t.TempDir()
	projectID := "demo-bbbbbbbbbbbbbbbb"
	dir := codeintel.ProviderStorageDir(homePath, projectID, codeintel.ProviderCodeGraph)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codeintel.GraphDBPath(homePath, projectID, codeintel.ProviderCodeGraph), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &codegraph.Adapter{
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(context.Context, string, []string) (codegraph.RunResult, error) {
			return codegraph.RunResult{Stdout: []byte("3.1.0")}, nil
		}},
	}
	st, err := a.Status(context.Background(), codeintel.Project{
		Root: t.TempDir(), ID: projectID, HomePath: homePath,
	})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.GraphPresent {
		t.Fatal("expected GraphPresent")
	}
	if st.State != codeintel.StateAvailable {
		t.Fatalf("state=%s want available; ready/stale require verified freshness evidence", st.State)
	}
	if !strings.Contains(st.Message, "graph.db present") {
		t.Fatalf("message=%q", st.Message)
	}
}

func TestArgs_WithDB(t *testing.T) {
	t.Parallel()
	db := "/tmp/atlas-home/projects/p1/codegraph/graph.db"
	got := codegraph.StatsArgs(db)
	want := []string{"stats", "--db", db, "--json"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("StatsArgs=%v want %v", got, want)
	}
	build := codegraph.BuildArgs("/proj", db)
	if !containsPair(build, "--db", db) || build[0] != "build" {
		t.Fatalf("BuildArgs=%v", build)
	}
	query := codegraph.QueryArgs(db, "Foo")
	if !containsPair(query, "--db", db) || query[0] != "query" {
		t.Fatalf("QueryArgs=%v", query)
	}
	if !contains(query, "--json") {
		t.Fatalf("QueryArgs missing --json: %v", query)
	}
}

func TestStatus_DistinctProjectStorage(t *testing.T) {
	t.Parallel()
	homePath := t.TempDir()
	a := codegraph.New()
	a.LookPath = func(string) (string, error) { return "", errors.New("missing") }
	a.Runner = &fakeRunner{}
	stA, _ := a.Status(context.Background(), codeintel.Project{ID: "alpha-1111111111111111", HomePath: homePath})
	stB, _ := a.Status(context.Background(), codeintel.Project{ID: "beta-2222222222222222", HomePath: homePath})
	if stA.StorageDir == "" || stA.StorageDir == stB.StorageDir {
		t.Fatalf("expected distinct storage: %q vs %q", stA.StorageDir, stB.StorageDir)
	}
	if !strings.HasSuffix(stA.GraphDBPath, filepath.Join("codegraph", "graph.db")) {
		t.Fatalf("graph path=%q", stA.GraphDBPath)
	}
}

func TestExecRunner_Timeout(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("sleep unavailable")
	}
	r := codegraph.ExecRunner{Timeout: 50 * time.Millisecond, MaxOutput: 1024}
	_, err := r.Run(context.Background(), "/bin/sleep", []string{"2"})
	if !errors.Is(err, codegraph.ErrTimeout) {
		t.Fatalf("err=%v want timeout", err)
	}
}

func containsPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func walkFiles(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if !info.IsDir() {
			out[rel] = struct{}{}
		}
		return nil
	})
	return out
}
