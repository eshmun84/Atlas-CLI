package codegraphsource_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/codegraphsource"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"gopkg.in/yaml.v3"
)

type fakeRunner struct {
	fn func(ctx context.Context, executable string, args []string) (codegraph.RunResult, error)
}

func (f *fakeRunner) Run(ctx context.Context, executable string, args []string) (codegraph.RunResult, error) {
	if f.fn != nil {
		return f.fn(ctx, executable, args)
	}
	return codegraph.RunResult{}, nil
}

func TestRetrieve_MissingBinary(t *testing.T) {
	t.Parallel()
	env, _, _ := seedProject(t)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper callers",
		Env:      env,
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Runner:   &fakeRunner{},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusUnavailable {
		t.Fatalf("status=%q want unavailable: %#v", res.SourceStatus.Status, res)
	}
}

func TestRetrieve_IncompatibleVersion(t *testing.T) {
	t.Parallel()
	env, _, _ := seedProject(t)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper callers",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if len(args) > 0 && args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("2.9.0\n")}, nil
			}
			t.Fatalf("unexpected args %v", args)
			return codegraph.RunResult{}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("status=%q want failed: %#v", res.SourceStatus.Status, res)
	}
	msg := strings.ToLower(res.Message + " " + res.SourceStatus.Message)
	if !strings.Contains(msg, "incompatible") && !strings.Contains(msg, "outside") && !strings.Contains(msg, "major") {
		t.Fatalf("expected incompatible version message: %#v", res)
	}
}

func TestRetrieve_MissingGraph(t *testing.T) {
	t.Parallel()
	env, _, _ := seedProject(t)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper callers",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if len(args) > 0 && args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			t.Fatalf("query must not run without graph: %v", args)
			return codegraph.RunResult{}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusUnavailable {
		t.Fatalf("status=%q want unavailable: %#v", res.SourceStatus.Status, res)
	}
}

func TestRetrieve_ValidQuery(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	writeMetadata(t, homePath, projectID, env.Root, false)

	var queryArgs []string
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "explain helper function callers",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if len(args) > 0 && args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			queryArgs = append([]string(nil), args...)
			return codegraph.RunResult{Stdout: []byte(validQueryJSON("helper"))}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("status=%q: %#v", res.SourceStatus.Status, res)
	}
	if res.QuerySymbol != "helper" {
		t.Fatalf("QuerySymbol=%q", res.QuerySymbol)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("candidates=%d", len(res.Candidates))
	}
	c := res.Candidates[0]
	if c.Source != contextcompose.SourceCodeGraph || c.Path != "src/main.go" || c.Provenance != "codegraph" {
		t.Fatalf("candidate %#v", c)
	}
	if strings.HasPrefix(c.Path, "/") || strings.Contains(c.Path, "projects/") {
		t.Fatalf("must not expose absolute/home paths: %#v", c)
	}
	liveDB := codeintel.GraphDBPath(homePath, projectID, codeintel.ProviderCodeGraph)
	if queryArgs[0] != "query" || !contains(queryArgs, "--json") {
		t.Fatalf("query args=%v", queryArgs)
	}
	dbArg := ""
	for i := 0; i+1 < len(queryArgs); i++ {
		if queryArgs[i] == "--db" {
			dbArg = queryArgs[i+1]
		}
	}
	if dbArg == "" || dbArg == liveDB || !strings.Contains(dbArg, "atlas-cg-query-") {
		t.Fatalf("expected isolated snapshot --db, got %q (live=%q) args=%v", dbArg, liveDB, queryArgs)
	}
	if !containsPair(queryArgs, "--limit", "8") {
		t.Fatalf("expected --limit 8 in %v", queryArgs)
	}
}

func TestRetrieve_NoResults(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(`{"name":"helper","results":[]}`)}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusNoCandidates {
		t.Fatalf("status=%q: %#v", res.SourceStatus.Status, res)
	}
}

func TestRetrieve_MalformedJSON(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(`{"results":`)}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("status=%q: %#v", res.SourceStatus.Status, res)
	}
}

func TestRetrieve_OversizedStdout(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte("{}"), Truncated: true}, codegraph.ErrOutputExceeded
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("status=%q: %#v", res.SourceStatus.Status, res)
	}
	if !strings.Contains(res.Message, "exceeded") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_Timeout(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{}, codegraph.ErrTimeout
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("status=%q: %#v", res.SourceStatus.Status, res)
	}
	if !strings.Contains(res.Message, "timed out") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_NonZeroExit(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{ExitCode: 2, Stderr: []byte("boom")}, codegraph.ErrProcessFailed
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("status=%q: %#v", res.SourceStatus.Status, res)
	}
}

func TestRetrieve_PathTraversalRejected(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(`{"name":"helper","results":[{"name":"helper","kind":"function","file":"../secret.go","line":1}]}`)}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusNoCandidates {
		t.Fatalf("status=%q want no_candidates after rejecting unsafe path: %#v", res.SourceStatus.Status, res)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("candidates=%v", res.Candidates)
	}
}

func TestRetrieve_AbsolutePathRejected(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(`{"name":"helper","results":[{"name":"helper","kind":"function","file":"/etc/passwd","line":1}]}`)}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusNoCandidates || len(res.Candidates) != 0 {
		t.Fatalf("absolute path must be rejected: %#v", res)
	}
}

func TestRetrieve_OversizedStringsBounded(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	longName := strings.Repeat("A", 500)
	payload := fmt.Sprintf(`{"name":"helper","results":[{"name":%q,"kind":"function","file":"src/main.go","line":1,"role":%q,"callers":[]}]}`,
		longName, strings.Repeat("r", 200))
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(payload)}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
	c := res.Candidates[0]
	if len([]rune(c.Title)) > 128 {
		t.Fatalf("title not bounded: %d", len([]rune(c.Title)))
	}
	if c.Path != "src/main.go" {
		t.Fatalf("path=%q", c.Path)
	}
}

func TestRetrieve_OversizedPathRejectedNotTruncated(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	// Truncation would drop the "/../secret.go" suffix and leave a seemingly
	// safe relative path — acceptance must reject the full string instead.
	oversized := strings.Repeat("x", contextcompose.MaxPathRunes) + "/../secret.go"
	if utf8.RuneCountInString(oversized) <= contextcompose.MaxPathRunes {
		t.Fatal("fixture must exceed MaxPathRunes")
	}
	payload := fmt.Sprintf(`{"name":"helper","results":[{"name":"helper","kind":"function","file":%q,"line":1}]}`, oversized)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(payload)}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusNoCandidates {
		t.Fatalf("oversized path must be rejected entirely: %#v", res)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("candidates=%#v", res.Candidates)
	}
}

func TestRetrieve_TooManyResultsCapped(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	results := make([]map[string]any, 0, 20)
	for i := 0; i < 20; i++ {
		results = append(results, map[string]any{
			"name": fmt.Sprintf("Fn%d", i),
			"kind": "function",
			"file": fmt.Sprintf("src/f%d.go", i),
			"line": i + 1,
		})
	}
	body, _ := json.Marshal(map[string]any{"name": "Fn0", "results": results})
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect Fn0",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: body}, nil
		}},
	})
	if len(res.Candidates) != contextcompose.MaxCodeGraphResults {
		t.Fatalf("got %d want %d", len(res.Candidates), contextcompose.MaxCodeGraphResults)
	}
}

func TestRetrieve_StaleGraphSurfaced(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	writeMetadata(t, homePath, projectID, env.Root, true) // stale fingerprint
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(validQueryJSON("helper"))}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
	if res.Freshness != string(codeintel.FreshnessStale) {
		t.Fatalf("freshness=%q", res.Freshness)
	}
	if !strings.Contains(res.Message, "stale") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_ProjectIsolation(t *testing.T) {
	t.Parallel()
	homePath := t.TempDir()
	envA, _, idA := seedNamedProject(t, homePath, "proj-a")
	envB, _, idB := seedNamedProject(t, homePath, "proj-b")
	writeGraphDB(t, homePath, idA)
	writeGraphDB(t, homePath, idB)

	liveA := codeintel.GraphDBPath(homePath, idA, codeintel.ProviderCodeGraph)
	liveB := codeintel.GraphDBPath(homePath, idB, codeintel.ProviderCodeGraph)
	if liveA == liveB {
		t.Fatalf("live DBs must be project-scoped: %q", liveA)
	}
	beforeA := snapshotTree(t, filepath.Dir(liveA))
	beforeB := snapshotTree(t, filepath.Dir(liveB))

	var snapA, snapB string
	run := func(env memoryretrieve.Env, capture *string) {
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
						*capture = args[i+1]
						_ = os.WriteFile(args[i+1]+"-wal", []byte{}, 0o600)
					}
				}
				return codegraph.RunResult{Stdout: []byte(validQueryJSON("helper"))}, nil
			}},
		})
		if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
			t.Fatalf("%#v", res)
		}
	}
	run(envA, &snapA)
	run(envB, &snapB)
	if snapA == "" || snapB == "" || snapA == snapB {
		t.Fatalf("expected distinct query snapshots: %q vs %q", snapA, snapB)
	}
	if snapA == liveA || snapB == liveB {
		t.Fatalf("query must not open live DBs: snap=%q/%q live=%q/%q", snapA, snapB, liveA, liveB)
	}
	if snapshotTree(t, filepath.Dir(liveA)) != beforeA || snapshotTree(t, filepath.Dir(liveB)) != beforeB {
		t.Fatal("project CodeGraph storage mutated during isolated retrieve")
	}
}

func TestRetrieve_Deterministic(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	in := codegraphsource.Input{
		Task:     "explain helper callers",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			return codegraph.RunResult{Stdout: []byte(validQueryJSON("helper"))}, nil
		}},
	}
	a := codegraphsource.Retrieve(in)
	b := codegraphsource.Retrieve(in)
	aj, _ := json.Marshal(a.Candidates)
	bj, _ := json.Marshal(b.Candidates)
	if string(aj) != string(bj) {
		t.Fatalf("non-deterministic:\n%s\n%s", aj, bj)
	}
	if a.QuerySymbol != b.QuerySymbol || a.SourceStatus.Status != b.SourceStatus.Status {
		t.Fatalf("status drift %#v vs %#v", a, b)
	}
}

func TestRetrieve_ReadOnly(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	beforeHome := snapshotTree(t, homePath)
	beforeRoot := snapshotTree(t, env.Root)
	_ = codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			if args[0] == "build" {
				t.Fatal("must not build")
			}
			return codegraph.RunResult{Stdout: []byte(validQueryJSON("helper"))}, nil
		}},
	})
	if snapshotTree(t, homePath) != beforeHome || snapshotTree(t, env.Root) != beforeRoot {
		t.Fatal("retrieve mutated filesystem")
	}
}

func TestRetrieve_NoSymbolDerived(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "please explain the function callers",
		Env:      env,
		LookPath: func(string) (string, error) { return "/bin/codegraph", nil },
		Runner: &fakeRunner{fn: func(_ context.Context, _ string, args []string) (codegraph.RunResult, error) {
			if args[0] == "--version" {
				return codegraph.RunResult{Stdout: []byte("3.17.0\n")}, nil
			}
			t.Fatalf("must not query without symbol: %v", args)
			return codegraph.RunResult{}, nil
		}},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusNoCandidates {
		t.Fatalf("%#v", res)
	}
}

func TestExecRunnerIntegration_FakeBinary(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	bin := writeFakeCodegraph(t, validQueryJSON("helper"))

	before := snapshotTree(t, homePath)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		Timeout:  3 * time.Second,
		LookPath: func(string) (string, error) { return bin, nil },
		Runner: codegraph.ExecRunner{
			Timeout:   3 * time.Second,
			MaxOutput: 64 << 10,
		},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Path != "src/main.go" {
		t.Fatalf("%#v", res.Candidates)
	}
	if snapshotTree(t, homePath) != before {
		t.Fatal("integration retrieve mutated home")
	}
}

func TestExecRunnerIntegration_OutputBound(t *testing.T) {
	t.Parallel()
	env, homePath, projectID := seedProject(t)
	writeGraphDB(t, homePath, projectID)
	huge := `{"name":"helper","results":[{"name":"helper","kind":"function","file":"src/main.go","line":1,"excerpt":"` + strings.Repeat("x", 20000) + `"}]}`
	bin := writeFakeCodegraph(t, huge)
	res := codegraphsource.Retrieve(codegraphsource.Input{
		Task:     "inspect helper",
		Env:      env,
		LookPath: func(string) (string, error) { return bin, nil },
		Runner: codegraph.ExecRunner{
			Timeout:   3 * time.Second,
			MaxOutput: 1024,
		},
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("expected failed on oversized output: %#v", res)
	}
}

// --- helpers ---

func seedProject(t *testing.T) (memoryretrieve.Env, string, string) {
	t.Helper()
	return seedNamedProject(t, t.TempDir(), "cg-demo")
}

func seedNamedProject(t *testing.T, homePath, name string) (memoryretrieve.Env, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig(name)
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectID, err := home.ProjectID(root, name)
	if err != nil {
		t.Fatal(err)
	}
	return memoryretrieve.Env{Root: root, Home: homePath}, homePath, projectID
}

func writeGraphDB(t *testing.T, homePath, projectID string) {
	t.Helper()
	path := codeintel.GraphDBPath(homePath, projectID, codeintel.ProviderCodeGraph)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("sqlite-stub"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeMetadata(t *testing.T, homePath, projectID, root string, stale bool) {
	t.Helper()
	fp := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	if !stale {
		computed, err := codeintel.ComputeSourceFingerprint(root)
		if err != nil {
			t.Fatal(err)
		}
		fp = computed.Value
	}
	meta := codeintel.Metadata{
		SchemaVersion:     codeintel.MetadataSchemaVersion,
		Provider:          string(codeintel.ProviderCodeGraph),
		ProviderVersion:   "3.17.0",
		ProjectID:         projectID,
		GraphDBPath:       codeintel.GraphDBPath(homePath, projectID, codeintel.ProviderCodeGraph),
		RefreshedAt:       time.Now().UTC().Format(time.RFC3339),
		RefreshMode:       string(codeintel.RefreshModeIncremental),
		SourceFingerprint: fp,
		NodesTotal:        4,
		FilesTotal:        1,
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	path := codeintel.MetadataPath(homePath, projectID, codeintel.ProviderCodeGraph)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func validQueryJSON(name string) string {
	return fmt.Sprintf(`{
  "name": %q,
  "results": [
    {
      "name": %q,
      "kind": "function",
      "file": "src/main.go",
      "line": 3,
      "endLine": 3,
      "role": "utility",
      "callers": [{"name":"main","kind":"function","file":"src/main.go","line":5}],
      "callees": []
    }
  ]
}`, name, name)
}

func writeFakeCodegraph(t *testing.T, queryJSON string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "codegraph")
	// Escape for embedding in shell script via base64-ish heredoc using printf %q via Go write.
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo '3.17.0'; exit 0; fi\n" +
		"if [ \"$1\" = \"query\" ]; then\n" +
		"cat <<'EOF'\n" + queryJSON + "\nEOF\n" +
		"exit 0\n" +
		"fi\necho 'unexpected' >&2; exit 2\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Ensure executable bit sticks on all platforms we care about.
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath(bin); err != nil {
		// LookPath requires PATH; direct path is fine for our LookPath hook.
		_ = err
	}
	return bin
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsPair(args []string, key, val string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == val {
			return true
		}
	}
	return false
}

func snapshotTree(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		b.WriteString(rel)
		b.WriteByte('|')
		if info != nil && !info.IsDir() {
			data, _ := os.ReadFile(path)
			sum := sha256.Sum256(data)
			b.WriteString(fmt.Sprintf("%d:%s", info.Size(), hex.EncodeToString(sum[:8])))
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
