package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"gopkg.in/yaml.v3"
)

func TestResolve_MemoryRoutes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args        []string
		mode        cli.Mode
		name        string
		query       string
		json        bool
		limit       int
		budget      int
		wantUnknown bool
	}{
		{[]string{"memory", "search", "CodeGraph", "--json"}, cli.ModeMemory, "search", "CodeGraph", true, 0, 0, false},
		{[]string{"memory", "search", "--json", "q", "--limit", "5"}, cli.ModeMemory, "search", "q", true, 5, 0, false},
		{[]string{"memory", "get", "obs_1", "--json"}, cli.ModeMemory, "get", "obs_1", true, 0, 0, false},
		{[]string{"memory", "context", "task text", "--json", "--budget", "1000", "--limit", "3"}, cli.ModeMemory, "context", "task text", true, 3, 1000, false},
		{[]string{"memory"}, cli.ModeMemory, "", "", false, 0, 0, true},
		{[]string{"memory", "search", "q"}, cli.ModeMemory, "search", "q", false, 0, 0, false},
		{[]string{"memory", "nope", "--json"}, cli.ModeMemory, "", "", false, 0, 0, true},
	}
	for _, tc := range cases {
		action := cli.Resolve(tc.args)
		if action.Mode != tc.mode {
			t.Fatalf("%v: mode=%v", tc.args, action.Mode)
		}
		if action.Memory.Name != tc.name || action.Memory.Query != tc.query || action.Memory.JSON != tc.json {
			t.Fatalf("%v: memory=%#v", tc.args, action.Memory)
		}
		if action.Memory.Limit != tc.limit || action.Memory.Budget != tc.budget {
			t.Fatalf("%v: limit/budget %#v", tc.args, action.Memory)
		}
		if tc.wantUnknown && action.UnknownCommand == "" {
			t.Fatalf("%v: expected unknown", tc.args)
		}
	}
}

func TestExecute_MemoryDoesNotLaunchTUI(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI must not launch for memory commands")
		return nil
	}
	defer func() { cli.RunTUI = prevTUI }()

	root, homePath := seedCLIProject(t)
	prevSvc := cli.MemoryService
	cli.MemoryService = func() memoryretrieve.Service {
		return memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	}
	defer func() { cli.MemoryService = prevSvc }()

	var stdout, stderr bytes.Buffer
	err := cli.Execute(&stdout, &stderr, []string{"memory", "search", "CodeGraph", "--json"})
	if err != nil {
		t.Fatalf("err=%v stdout=%s", err, stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q", stderr.String())
	}
	var resp memoryretrieve.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v (%s)", err, stdout.String())
	}
	if resp.Status != memoryretrieve.StatusSuccess || resp.Command != "search" {
		t.Fatalf("%#v", resp)
	}
	if resp.Count != 1 || len(resp.Results) != 1 {
		t.Fatalf("%#v", resp)
	}

	// get
	stdout.Reset()
	id := resp.Results[0].ID
	if err := cli.Execute(&stdout, &stderr, []string{"memory", "get", id, "--json"}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Observation == nil || resp.Observation.Content == "" {
		t.Fatalf("%#v", resp)
	}

	// context
	stdout.Reset()
	if err := cli.Execute(&stdout, &stderr, []string{"memory", "context", "CodeGraph intelligence", "--json"}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memoryretrieve.StatusSuccess || resp.Count < 1 {
		t.Fatalf("%#v", resp)
	}
}

func TestExecute_MemoryRequiresJSONAndExitCodes(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("tui"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	var stdout bytes.Buffer
	err := cli.Execute(&stdout, nil, []string{"memory", "search", "q"})
	if err == nil {
		t.Fatal("expected error without --json")
	}
	var resp memoryretrieve.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memoryretrieve.StatusInvalidUsage {
		t.Fatalf("%#v", resp)
	}

	root, homePath := seedCLIProject(t)
	prevSvc := cli.MemoryService
	cli.MemoryService = func() memoryretrieve.Service {
		return memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	}
	defer func() { cli.MemoryService = prevSvc }()

	stdout.Reset()
	err = cli.Execute(&stdout, nil, []string{"memory", "search", `"bad`, "--json"})
	if err == nil {
		t.Fatal("expected invalid query exit")
	}
	if !strings.Contains(err.Error(), memoryretrieve.StatusInvalidQuery) {
		t.Fatalf("err=%v", err)
	}

	stdout.Reset()
	err = cli.Execute(&stdout, nil, []string{"memory", "search", "nomatchzzz", "--json"})
	if err != nil {
		t.Fatalf("no_results should be zero exit: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memoryretrieve.StatusNoResults {
		t.Fatalf("%#v", resp)
	}
}

func TestExecute_MemoryMissingDBReadOnly(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("tui"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root := t.TempDir()
	homePath := t.TempDir()
	writeCLIConfig(t, root, "empty-mem")
	before := snapshotTree(t, homePath)

	prevSvc := cli.MemoryService
	cli.MemoryService = func() memoryretrieve.Service {
		return memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	}
	defer func() { cli.MemoryService = prevSvc }()

	var stdout bytes.Buffer
	err := cli.Execute(&stdout, nil, []string{"memory", "search", "x", "--json"})
	if err == nil {
		t.Fatal("expected missing db error")
	}
	after := snapshotTree(t, homePath)
	if before != after {
		t.Fatalf("home mutated\nbefore=%s\nafter=%s", before, after)
	}
	var resp memoryretrieve.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memoryretrieve.StatusMissingMemoryDB {
		t.Fatalf("%#v", resp)
	}
}

func seedCLIProject(t *testing.T) (root, homePath string) {
	t.Helper()
	root = t.TempDir()
	homePath = t.TempDir()
	writeCLIConfig(t, root, "cli-mem")
	projectID, err := home.ProjectID(root, "cli-mem")
	if err != nil {
		t.Fatal(err)
	}
	store, err := memory.Open(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveObservation(memory.Observation{
		Type: memory.TypeFact, Title: "CodeGraph", Content: "CodeGraph is the Code Intelligence provider",
	}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	return root, homePath
}

func writeCLIConfig(t *testing.T, root, name string) {
	t.Helper()
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
		if !info.IsDir() {
			b.WriteString(info.Mode().String())
			b.WriteByte('|')
			b.WriteString(strings.ReplaceAll(string(mustRead(t, path)), "\n", "\\n"))
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
