package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist"
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
		{[]string{"memory", "propose", "--json"}, cli.ModeMemory, "propose", "", true, 0, 0, false},
		{[]string{"memory"}, cli.ModeMemory, "", "", false, 0, 0, true},
		{[]string{"memory", "search", "q"}, cli.ModeMemory, "search", "q", false, 0, 0, false},
		{[]string{"memory", "nope", "--json"}, cli.ModeMemory, "", "", false, 0, 0, true},
		{[]string{"memory", "propose", "--json", "extra"}, cli.ModeMemory, "propose", "", true, 0, 0, true},
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

func TestExecute_MemoryProposeGoverned(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI must not launch for memory propose")
		return nil
	}
	defer func() { cli.RunTUI = prevTUI }()

	root := t.TempDir()
	homePath := t.TempDir()
	writeCLIConfig(t, root, "propose-cli")

	prevPersist := cli.MemoryPersistService
	cli.MemoryPersistService = func() memorypersist.Service {
		return memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	}
	defer func() { cli.MemoryPersistService = prevPersist }()

	prevIn := cli.MemoryProposeInput
	body := `{
		"type":"decision",
		"title":"Prefer SQLite Memory Core",
		"content":"Atlas stores durable project Memory in SQLite under Atlas Home, not in the product repository.",
		"scope":"project",
		"topic_key":"memory.storage.engine",
		"origin":"atlas_lifecycle",
		"explicitness":"explicit",
		"confidence":"high",
		"provenance":"cap=cli-test",
		"source_event":"cli.propose"
	}`
	cli.MemoryProposeInput = func() io.Reader { return strings.NewReader(body) }
	defer func() { cli.MemoryProposeInput = prevIn }()

	var stdout bytes.Buffer
	if err := cli.Execute(&stdout, nil, []string{"memory", "propose", "--json"}); err != nil {
		t.Fatalf("err=%v out=%s", err, stdout.String())
	}
	var resp memorypersist.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memorypersist.StatusAccepted || resp.Observation == nil {
		t.Fatalf("%#v", resp)
	}

	// Replay → duplicate soft exit.
	stdout.Reset()
	cli.MemoryProposeInput = func() io.Reader { return strings.NewReader(body) }
	if err := cli.Execute(&stdout, nil, []string{"memory", "propose", "--json"}); err != nil {
		t.Fatalf("duplicate should be soft: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memorypersist.StatusDuplicate {
		t.Fatalf("%#v", resp)
	}

	// Secret → rejected soft exit, retrieval empty of secrets.
	secret := `{
		"type":"fact",
		"title":"Database password note for ops",
		"content":"Production password: SuperSecretPassword123",
		"origin":"atlas_operation",
		"explicitness":"explicit",
		"confidence":"high"
	}`
	stdout.Reset()
	cli.MemoryProposeInput = func() io.Reader { return strings.NewReader(secret) }
	if err := cli.Execute(&stdout, nil, []string{"memory", "propose", "--json"}); err != nil {
		t.Fatalf("rejected should be soft: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memorypersist.StatusRejected {
		t.Fatalf("%#v", resp)
	}

	// Inferred + confirm key → invalid_input (unknown field); never writes.
	inferredConfirm := `{
		"type":"decision",
		"title":"Inferred architecture choice for caching",
		"content":"Agents inferred Redis as the preferred cache but must not self-confirm.",
		"origin":"atlas_operation",
		"explicitness":"inferred",
		"confidence":"medium",
		"confirm":true
	}`
	stdout.Reset()
	cli.MemoryProposeInput = func() io.Reader { return strings.NewReader(inferredConfirm) }
	if err := cli.Execute(&stdout, nil, []string{"memory", "propose", "--json"}); err == nil {
		t.Fatal("confirm field must be rejected as invalid_input")
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memorypersist.StatusInvalidInput {
		t.Fatalf("%#v", resp)
	}
	if strings.Contains(stdout.String(), "Redis") && strings.Contains(resp.Error.Message, "Redis") {
		t.Fatal("must not echo payload")
	}

	// Inferred without confirm → confirmation_required, no write.
	inferred := `{
		"type":"decision",
		"title":"Inferred architecture choice for caching",
		"content":"Agents inferred Redis as the preferred cache but confirmation is deferred.",
		"origin":"atlas_operation",
		"explicitness":"inferred",
		"confidence":"medium"
	}`
	stdout.Reset()
	cli.MemoryProposeInput = func() io.Reader { return strings.NewReader(inferred) }
	if err := cli.Execute(&stdout, nil, []string{"memory", "propose", "--json"}); err != nil {
		t.Fatalf("confirmation_required should be soft: %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memorypersist.StatusConfirmationRequired {
		t.Fatalf("%#v", resp)
	}

	// Self-labeled confirmed must not write (soft confirmation_required).
	confirmed := `{
		"type":"decision",
		"title":"Self labeled confirmed architecture note",
		"content":"Agents must not self-assert confirmed through the untrusted propose transport.",
		"origin":"developer_confirmation",
		"explicitness":"confirmed",
		"confidence":"high"
	}`
	stdout.Reset()
	cli.MemoryProposeInput = func() io.Reader { return strings.NewReader(confirmed) }
	if err := cli.Execute(&stdout, nil, []string{"memory", "propose", "--json"}); err != nil {
		t.Fatalf("confirmed should be soft confirmation_required: %v", err)
	}
	resp = memorypersist.Response{}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != memorypersist.StatusConfirmationRequired {
		t.Fatalf("%#v", resp)
	}
	if resp.Observation != nil {
		t.Fatal("confirmed must not write an observation")
	}
}

func TestExecute_MemoryProposeStrictJSON(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("tui"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root := t.TempDir()
	homePath := t.TempDir()
	writeCLIConfig(t, root, "propose-json")
	prevPersist := cli.MemoryPersistService
	cli.MemoryPersistService = func() memorypersist.Service {
		return memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	}
	defer func() { cli.MemoryPersistService = prevPersist }()
	prevIn := cli.MemoryProposeInput
	defer func() { cli.MemoryProposeInput = prevIn }()

	valid := `{
		"type":"decision",
		"title":"Prefer SQLite Memory Core",
		"content":"Atlas stores durable project Memory in SQLite under Atlas Home, not in the product repository.",
		"origin":"atlas_lifecycle",
		"explicitness":"explicit"
	}`
	cases := []struct {
		name string
		body string
		want string
	}{
		{"valid", valid, memorypersist.StatusAccepted},
		{"empty", "", memorypersist.StatusInvalidInput},
		{"whitespace", "  \n\t ", memorypersist.StatusInvalidInput},
		{"trailing", valid + " {}", memorypersist.StatusInvalidInput},
		{"concat", valid + valid, memorypersist.StatusInvalidInput},
		{"garbage", valid + " nope", memorypersist.StatusInvalidInput},
		{"unknown", `{"type":"decision","title":"Prefer SQLite Memory Core","content":"Atlas stores durable project Memory in SQLite under Atlas Home, not in the product repository.","origin":"atlas_lifecycle","extra":1}`, memorypersist.StatusInvalidInput},
		{"oversize", strings.Repeat("x", memorypersist.MaxCandidateJSONBytes+1), memorypersist.StatusInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli.MemoryProposeInput = func() io.Reader { return strings.NewReader(tc.body) }
			var stdout bytes.Buffer
			_ = cli.Execute(&stdout, nil, []string{"memory", "propose", "--json"})
			var resp memorypersist.Response
			if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
				t.Fatalf("json: %v (%s)", err, stdout.String())
			}
			if resp.Status != tc.want {
				t.Fatalf("status=%q want %q body=%s", resp.Status, tc.want, stdout.String())
			}
			if resp.Error != nil && strings.Contains(resp.Error.Message, strings.Repeat("x", 32)) {
				t.Fatal("must not echo oversized payload")
			}
		})
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
