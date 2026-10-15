package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"gopkg.in/yaml.v3"
)

func TestResolve_ContextComposeRoutes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args        []string
		task        string
		json        bool
		budget      int
		limit       int
		wantUnknown bool
	}{
		{[]string{"context", "compose", "do work", "--json"}, "do work", true, 0, 0, false},
		{[]string{"context", "compose", "--json", "task", "--budget", "1000", "--limit", "4"}, "task", true, 1000, 4, false},
		{[]string{"context", "compose", "task"}, "task", false, 0, 0, false},
		{[]string{"context"}, "", false, 0, 0, true},
		{[]string{"context", "update"}, "", false, 0, 0, true},
		{[]string{"context", "compose", "--json"}, "", true, 0, 0, false},
	}
	for _, tc := range cases {
		action := cli.Resolve(tc.args)
		if action.Mode != cli.ModeContextCompose {
			t.Fatalf("%v: mode=%v", tc.args, action.Mode)
		}
		if action.ContextCompose.Task != tc.task || action.ContextCompose.JSON != tc.json {
			t.Fatalf("%v: cmd=%#v", tc.args, action.ContextCompose)
		}
		if action.ContextCompose.Budget != tc.budget || action.ContextCompose.Limit != tc.limit {
			t.Fatalf("%v: budget/limit %#v", tc.args, action.ContextCompose)
		}
		if tc.wantUnknown && action.UnknownCommand == "" {
			t.Fatalf("%v: expected unknown", tc.args)
		}
	}
}

func TestResolve_ContextComposeSources(t *testing.T) {
	t.Parallel()

	action := cli.Resolve([]string{"context", "compose", "task", "--json", "--sources", "memory,codegraph"})
	if !action.ContextCompose.SourcesSet {
		t.Fatal("SourcesSet")
	}
	if got := strings.Join(action.ContextCompose.Sources, ","); got != "memory,codegraph" {
		t.Fatalf("sources=%v", action.ContextCompose.Sources)
	}

	action = cli.Resolve([]string{"context", "compose", "task", "--json", "--sources=memory,context_economy"})
	if got := strings.Join(action.ContextCompose.Sources, ","); got != "memory,context_economy" {
		t.Fatalf("sources=%v", action.ContextCompose.Sources)
	}

	// Duplicates deduped deterministically (first-seen order).
	action = cli.Resolve([]string{"context", "compose", "task", "--json", "--sources", "memory,codegraph,memory,codegraph"})
	if got := strings.Join(action.ContextCompose.Sources, ","); got != "memory,codegraph" {
		t.Fatalf("dedup sources=%v", action.ContextCompose.Sources)
	}

	// Omitted --sources preserves unset.
	action = cli.Resolve([]string{"context", "compose", "task", "--json"})
	if action.ContextCompose.SourcesSet || len(action.ContextCompose.Sources) != 0 {
		t.Fatalf("omitted sources must be unset: %#v", action.ContextCompose)
	}
}

func TestExecute_ContextComposeSourcesValidationAndPassthrough(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI must not launch")
		return nil
	}
	defer func() { cli.RunTUI = prevTUI }()

	t.Run("unknown_source_rejected", func(t *testing.T) {
		var stdout bytes.Buffer
		err := cli.Execute(&stdout, &bytes.Buffer{}, []string{
			"context", "compose", "task", "--json", "--sources", "memory,nope",
		})
		if err == nil {
			t.Fatal("expected failure")
		}
		var res composeapp.Result
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if res.Status != contextcompose.StatusInvalidRequest {
			t.Fatalf("%#v", res)
		}
	})

	t.Run("empty_sources_rejected", func(t *testing.T) {
		var stdout bytes.Buffer
		err := cli.Execute(&stdout, &bytes.Buffer{}, []string{
			"context", "compose", "task", "--json", "--sources", "",
		})
		if err == nil {
			t.Fatal("expected failure for empty --sources")
		}
		var res composeapp.Result
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if res.Status != contextcompose.StatusInvalidRequest {
			t.Fatalf("%#v", res)
		}
	})

	t.Run("passthrough_exact_sources", func(t *testing.T) {
		var got []string
		prev := cli.ComposeService
		cli.ComposeService = func(in composeapp.Input) composeapp.Result {
			got = append([]string{}, in.Sources...)
			return composeapp.Result{
				Command: composeapp.CommandCompose,
				Response: contextcompose.Response{
					SchemaVersion: contextcompose.SchemaVersion,
					Status:        contextcompose.StatusSuccess,
					BudgetChars:   contextcompose.DefaultBudgetChars,
				},
			}
		}
		defer func() { cli.ComposeService = prev }()

		var stdout bytes.Buffer
		if err := cli.Execute(&stdout, &bytes.Buffer{}, []string{
			"context", "compose", "task", "--json", "--sources", "memory,codegraph",
		}); err != nil {
			t.Fatalf("err=%v stdout=%s", err, stdout.String())
		}
		if strings.Join(got, ",") != "memory,codegraph" {
			t.Fatalf("ComposeService sources=%v", got)
		}
	})

	t.Run("omitted_sources_nil_to_service", func(t *testing.T) {
		var called bool
		var got []string
		prev := cli.ComposeService
		cli.ComposeService = func(in composeapp.Input) composeapp.Result {
			called = true
			got = in.Sources
			return composeapp.Result{
				Command: composeapp.CommandCompose,
				Response: contextcompose.Response{
					SchemaVersion: contextcompose.SchemaVersion,
					Status:        contextcompose.StatusSuccess,
					BudgetChars:   contextcompose.DefaultBudgetChars,
				},
			}
		}
		defer func() { cli.ComposeService = prev }()

		var stdout bytes.Buffer
		if err := cli.Execute(&stdout, &bytes.Buffer{}, []string{"context", "compose", "task", "--json"}); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("ComposeService not called")
		}
		if got != nil {
			t.Fatalf("omitted --sources must pass nil Sources, got %v", got)
		}
	})
}

func TestExecute_ContextComposeJSONSchema(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI must not launch for context compose")
		return nil
	}
	defer func() { cli.RunTUI = prevTUI }()

	root, homePath := seedComposeCLIProject(t)
	prev := cli.ComposeService
	cli.ComposeService = func(in composeapp.Input) composeapp.Result {
		in.Env = memoryretrieve.Env{Root: root, Home: homePath}
		return composeapp.Compose(in)
	}
	defer func() { cli.ComposeService = prev }()

	var stdout, stderr bytes.Buffer
	err := cli.Execute(&stdout, &stderr, []string{"context", "compose", "orient atlas project", "--json"})
	if err != nil {
		t.Fatalf("err=%v stdout=%s", err, stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q", stderr.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("json: %v (%s)", err, stdout.String())
	}
	for _, key := range []string{"schema_version", "command", "status", "budget_chars", "used_chars", "count"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing key %q in %s", key, stdout.String())
		}
	}
	if int(raw["schema_version"].(float64)) != contextcompose.SchemaVersion {
		t.Fatalf("schema=%v", raw["schema_version"])
	}
	if raw["command"] != composeapp.CommandCompose {
		t.Fatalf("command=%v", raw["command"])
	}
	if strings.Contains(stdout.String(), homePath) {
		t.Fatal("JSON must not leak Atlas Home path")
	}
}

func TestExecute_ContextComposeStructuredFailures(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI must not launch")
		return nil
	}
	defer func() { cli.RunTUI = prevTUI }()

	var stdout bytes.Buffer
	err := cli.Execute(&stdout, &bytes.Buffer{}, []string{"context", "compose", "task"})
	if err == nil {
		t.Fatal("expected non-zero without --json")
	}
	var res composeapp.Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != contextcompose.StatusInvalidRequest || res.Error == nil {
		t.Fatalf("%#v", res)
	}

	stdout.Reset()
	err = cli.Execute(&stdout, &bytes.Buffer{}, []string{"context", "compose", "--json"})
	if err == nil {
		t.Fatal("expected failure for missing task")
	}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("%#v", res)
	}
}

func TestExecute_ContextComposeOversizedBudgetRejected(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root, homePath := seedComposeCLIProject(t)
	prev := cli.ComposeService
	cli.ComposeService = func(in composeapp.Input) composeapp.Result {
		in.Env = memoryretrieve.Env{Root: root, Home: homePath}
		return composeapp.Compose(in)
	}
	defer func() { cli.ComposeService = prev }()

	var stdout bytes.Buffer
	err := cli.Execute(&stdout, &bytes.Buffer{}, []string{
		"context", "compose", "orient", "--json",
		"--budget", strconv.Itoa(contextcompose.MaxBudgetChars + 1),
	})
	if err == nil {
		t.Fatal("expected non-zero for oversized budget")
	}
	var res composeapp.Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("%#v", res)
	}
	if res.UsedChars > res.BudgetChars && res.BudgetChars > 0 {
		t.Fatalf("used=%d budget=%d", res.UsedChars, res.BudgetChars)
	}
}

func TestExecute_ContextComposeOversizedTaskRejected(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root, homePath := seedComposeCLIProject(t)
	prev := cli.ComposeService
	cli.ComposeService = func(in composeapp.Input) composeapp.Result {
		in.Env = memoryretrieve.Env{Root: root, Home: homePath}
		return composeapp.Compose(in)
	}
	defer func() { cli.ComposeService = prev }()

	huge := strings.Repeat("t", contextcompose.MaxTaskRunes+8)
	var stdout bytes.Buffer
	err := cli.Execute(&stdout, &bytes.Buffer{}, []string{"context", "compose", huge, "--json"})
	if err == nil {
		t.Fatal("expected non-zero for oversized task")
	}
	var res composeapp.Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("%#v", res)
	}
}

func TestExecute_ContextComposeDoesNotMutate(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root, homePath := seedComposeCLIProject(t)
	beforeHome := walkSnap(t, homePath)
	beforeRoot := walkSnap(t, root)

	prev := cli.ComposeService
	cli.ComposeService = func(in composeapp.Input) composeapp.Result {
		in.Env = memoryretrieve.Env{Root: root, Home: homePath}
		return composeapp.Compose(in)
	}
	defer func() { cli.ComposeService = prev }()

	var stdout bytes.Buffer
	if err := cli.Execute(&stdout, &bytes.Buffer{}, []string{"context", "compose", "orient", "--json"}); err != nil {
		t.Fatal(err)
	}
	if walkSnap(t, homePath) != beforeHome || walkSnap(t, root) != beforeRoot {
		t.Fatal("CLI compose mutated filesystem")
	}
}

func seedComposeCLIProject(t *testing.T) (root, homePath string) {
	t.Helper()
	root = t.TempDir()
	homePath = t.TempDir()
	name := "cli-compose"
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
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectID, err := home.ProjectID(root, name)
	if err != nil {
		t.Fatal(err)
	}
	store, err := memory.Open(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveObservation(memory.Observation{
		Type: memory.TypeFact, Title: "CLI compose", Content: "orient atlas project composition",
	}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	idx, err := atlascontext.BuildIndex(root, name, time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	idx.ProjectID = projectID
	indexYAML, err := atlascontext.RenderIndexYAML(idx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		path string
		data string
	}{
		{atlascontext.IndexPath(homePath, projectID), indexYAML},
		{atlascontext.CapsulePath(homePath, projectID), atlascontext.RenderCapsule(idx)},
	} {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.path, []byte(p.data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, homePath
}

func walkSnap(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		b.WriteString(rel)
		if info != nil && !info.IsDir() {
			data, _ := os.ReadFile(path)
			b.WriteString(string(data))
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
