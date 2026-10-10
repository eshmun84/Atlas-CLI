package memoryretrieve_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

func TestSearchGetContext_HappyPath(t *testing.T) {
	svc, homePath, projectID, root := seedMemoryService(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "CodeGraph provider", Content: "CodeGraph is the Code Intelligence provider for Atlas"},
		{Type: memory.TypeArchitecture, Title: "Memory Core", Content: "Memory uses SQLite and FTS5 under Atlas Home", TopicKey: "architecture/memory"},
		{Type: memory.TypeConstraint, Title: "No embeddings", Content: "Slice 36A must not introduce vector search"},
	})

	search := svc.Search(memoryretrieve.SearchInput{Query: "CodeGraph", Limit: 10})
	if search.Status != memoryretrieve.StatusSuccess || search.Count != 1 {
		t.Fatalf("search=%#v", search)
	}
	if search.ProjectID != projectID {
		t.Fatalf("project_id=%q", search.ProjectID)
	}
	if search.Results[0].Title != "CodeGraph provider" {
		t.Fatalf("hit=%#v", search.Results[0])
	}
	if search.Results[0].Preview == "" {
		t.Fatal("preview required")
	}
	if strings.Contains(mustJSON(t, search), `"content"`) {
		t.Fatal("search JSON must not include full content field")
	}

	got := svc.Get(memoryretrieve.GetInput{ID: search.Results[0].ID})
	if got.Status != memoryretrieve.StatusSuccess || got.Observation == nil {
		t.Fatalf("get=%#v", got)
	}
	if got.Observation.Content == "" || got.Observation.ProjectID != projectID {
		t.Fatalf("observation=%#v", got.Observation)
	}

	ctx := svc.Context(memoryretrieve.ContextInput{Task: "memory sqlite architecture", Limit: 5, Budget: 2000})
	if ctx.Status != memoryretrieve.StatusSuccess || ctx.Count == 0 {
		t.Fatalf("context=%#v", ctx)
	}
	if ctx.BudgetChars != 2000 || ctx.UsedChars <= 0 {
		t.Fatalf("budget=%d used=%d", ctx.BudgetChars, ctx.UsedChars)
	}
	if ctx.Items[0].Ref.Get == "" || ctx.Items[0].Excerpt == "" {
		t.Fatalf("item=%#v", ctx.Items[0])
	}
	for _, note := range ctx.Limits {
		if strings.Contains(strings.ToLower(note), "embedding") {
			goto okNote
		}
	}
	t.Fatal("expected honesty note about embeddings")
okNote:

	// Read-only: no derived files under Home or project.
	assertNoDerivedWrites(t, homePath, root)
}

func TestSearch_NoResultsAndInvalidQuery(t *testing.T) {
	svc, _, _, _ := seedMemoryService(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "Alpha", Content: "unique token zzzyx"},
	})

	empty := svc.Search(memoryretrieve.SearchInput{Query: "nomatchtokenzzz"})
	if empty.Status != memoryretrieve.StatusNoResults || empty.Count != 0 {
		t.Fatalf("%#v", empty)
	}

	bad := svc.Search(memoryretrieve.SearchInput{Query: `"unterminated`})
	if bad.Status != memoryretrieve.StatusInvalidQuery {
		t.Fatalf("status=%s", bad.Status)
	}
	if bad.Error == nil || bad.Error.Code != memoryretrieve.StatusInvalidQuery {
		t.Fatalf("error=%#v", bad.Error)
	}
}

func TestSearch_LimitAndDeterministicOrder(t *testing.T) {
	svc, _, _, _ := seedMemoryService(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "shared alpha", Content: "shared keyword body one"},
		{Type: memory.TypeFact, Title: "shared beta", Content: "shared keyword body two"},
		{Type: memory.TypeFact, Title: "shared gamma", Content: "shared keyword body three"},
	})

	a := svc.Search(memoryretrieve.SearchInput{Query: "shared", Limit: 2})
	if a.Count != 2 || a.Limit != 2 {
		t.Fatalf("%#v", a)
	}
	b := svc.Search(memoryretrieve.SearchInput{Query: "shared", Limit: 2})
	if mustJSON(t, a) != mustJSON(t, b) {
		t.Fatalf("non-deterministic search\n%s\n%s", mustJSON(t, a), mustJSON(t, b))
	}
}

func TestContext_BudgetLimit(t *testing.T) {
	long := strings.Repeat("memory retrieval budget token ", 40)
	svc, _, _, _ := seedMemoryService(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "One", Content: long + " alpha"},
		{Type: memory.TypeFact, Title: "Two", Content: long + " beta"},
		{Type: memory.TypeFact, Title: "Three", Content: long + " gamma"},
	})

	for _, budget := range []int{1, 20, 50, 180, 400} {
		ctx := svc.Context(memoryretrieve.ContextInput{Task: "memory retrieval budget", Limit: 10, Budget: budget})
		if ctx.Status != memoryretrieve.StatusSuccess && ctx.Status != memoryretrieve.StatusNoResults {
			t.Fatalf("budget=%d %#v", budget, ctx)
		}
		if ctx.UsedChars > ctx.BudgetChars {
			t.Fatalf("invariant violated: used=%d budget=%d", ctx.UsedChars, ctx.BudgetChars)
		}
		if ctx.BudgetChars != budget {
			t.Fatalf("budget_chars=%d want %d", ctx.BudgetChars, budget)
		}
	}

	ctx := svc.Context(memoryretrieve.ContextInput{Task: "memory retrieval budget", Limit: 10, Budget: 180})
	if ctx.Status != memoryretrieve.StatusSuccess {
		t.Fatalf("%#v", ctx)
	}
	if ctx.Count < 1 {
		t.Fatal("expected at least one item")
	}
	if ctx.UsedChars > ctx.BudgetChars {
		t.Fatalf("used=%d budget=%d", ctx.UsedChars, ctx.BudgetChars)
	}
}

func TestContext_LongTitleTinyBudget(t *testing.T) {
	// Title near validate max (512) so fitContextItem must truncate under tiny budget.
	title := strings.Repeat("VeryLongTitleToken", 28) // 504 chars
	svc, _, _, _ := seedMemoryService(t, []memory.Observation{
		{
			Type:    memory.TypeFact,
			Title:   title,
			Content: strings.Repeat("memory progressive retrieval content ", 50),
		},
	})
	ctx := svc.Context(memoryretrieve.ContextInput{Task: "memory progressive retrieval", Limit: 5, Budget: 60})
	if ctx.UsedChars > ctx.BudgetChars {
		t.Fatalf("used=%d budget=%d items=%#v", ctx.UsedChars, ctx.BudgetChars, ctx.Items)
	}
	if ctx.Status == memoryretrieve.StatusSuccess && ctx.Count > 0 {
		it := ctx.Items[0]
		if len([]rune(it.Title)) > 60 {
			t.Fatalf("title not truncated under budget: %d", len([]rune(it.Title)))
		}
	}
}

func TestGet_NotFoundAndProjectIsolation(t *testing.T) {
	svc, homePath, projectID, _ := seedMemoryService(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "Owned", Content: "belongs to project"},
	})
	search := svc.Search(memoryretrieve.SearchInput{Query: "Owned"})
	id := search.Results[0].ID

	missing := svc.Get(memoryretrieve.GetInput{ID: "obs_does_not_exist"})
	if missing.Status != memoryretrieve.StatusNotFound {
		t.Fatalf("%#v", missing)
	}

	// Foreign observation id in another project's DB must not leak.
	otherRoot := t.TempDir()
	writeProjectConfig(t, otherRoot, "other-proj")
	otherID, err := home.ProjectID(otherRoot, "other-proj")
	if err != nil {
		t.Fatal(err)
	}
	otherStore, err := memory.Open(homePath, otherID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherStore.SaveObservation(memory.Observation{
		Type: memory.TypeFact, Title: "Foreign", Content: "other project only",
	}); err != nil {
		t.Fatal(err)
	}
	_ = otherStore.Close()

	foreign := svc.Get(memoryretrieve.GetInput{ID: id})
	if foreign.Status != memoryretrieve.StatusSuccess || foreign.ProjectID != projectID {
		t.Fatalf("own get failed: %#v", foreign)
	}

	// Open other project service and ensure it cannot get first project's obs by id.
	otherSvc := memoryretrieve.Service{Env: memoryretrieve.Env{Root: otherRoot, Home: homePath}}
	leak := otherSvc.Get(memoryretrieve.GetInput{ID: id})
	if leak.Status != memoryretrieve.StatusNotFound {
		t.Fatalf("cross-project leak: %#v", leak)
	}
}

func TestMissingDBAndIncompatibleSchema(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "missing-db")
	svc := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}

	miss := svc.Search(memoryretrieve.SearchInput{Query: "x"})
	if miss.Status != memoryretrieve.StatusMissingMemoryDB {
		t.Fatalf("%#v", miss)
	}

	// Create empty/incompatible DB: open RW then bump schema.
	projectID, err := home.ProjectID(root, "missing-db")
	if err != nil {
		t.Fatal(err)
	}
	store, err := memory.Open(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	dbRel, err := memory.DatabaseRel(projectID)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(homePath, filepath.FromSlash(dbRel)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES (999, 't')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	bad := svc.Search(memoryretrieve.SearchInput{Query: "x"})
	if bad.Status != memoryretrieve.StatusIncompatibleSchema {
		t.Fatalf("%#v", bad)
	}
}

func TestUnsafeSymlinkDatabase(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "symlink-db")
	projectID, err := home.ProjectID(root, "symlink-db")
	if err != nil {
		t.Fatal(err)
	}
	rel, err := memory.DatabaseRel(projectID)
	if err != nil {
		t.Fatal(err)
	}
	dirRel, err := memory.MemoryDirRel(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homePath, filepath.FromSlash(dirRel)), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(homePath, "outside.db")
	if err := os.WriteFile(target, []byte("not-a-db"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	svc := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	resp := svc.Search(memoryretrieve.SearchInput{Query: "x"})
	if resp.Status != memoryretrieve.StatusUnsafeFilesystem {
		t.Fatalf("%#v", resp)
	}
}

func TestNotInitialized(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	beforeRoot := snapshotTreeDir(t, root)
	beforeHome := snapshotTreeDir(t, homePath)
	svc := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	resp := svc.Search(memoryretrieve.SearchInput{Query: "x"})
	if resp.Status != memoryretrieve.StatusNotInitialized {
		t.Fatalf("%#v", resp)
	}
	if snapshotTreeDir(t, root) != beforeRoot || snapshotTreeDir(t, homePath) != beforeHome {
		t.Fatal("resolve/search mutated filesystem on missing config")
	}
}

func TestConfigSymlinkUnsafe(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(outside, []byte("project:\n  name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, filepath.FromSlash(config.FileConfig))
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	beforeHome := snapshotTreeDir(t, homePath)
	svc := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	resp := svc.Search(memoryretrieve.SearchInput{Query: "x"})
	if resp.Status != memoryretrieve.StatusUnsafeFilesystem {
		t.Fatalf("status=%s want unsafe_filesystem; %#v", resp.Status, resp)
	}
	if snapshotTreeDir(t, homePath) != beforeHome {
		t.Fatal("symlink config path mutated Atlas Home")
	}
}

func TestConfigCorruptInternalError(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), []byte("{{{{not-yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeHome := snapshotTreeDir(t, homePath)
	svc := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	resp := svc.Search(memoryretrieve.SearchInput{Query: "x"})
	if resp.Status != memoryretrieve.StatusInternalError {
		t.Fatalf("status=%s want internal_error; %#v", resp.Status, resp)
	}
	if snapshotTreeDir(t, homePath) != beforeHome {
		t.Fatal("corrupt config path mutated Atlas Home")
	}
}

func TestUnexpectedWorkingDirectoryError(t *testing.T) {
	svc := memoryretrieve.Service{Env: memoryretrieve.Env{
		Getwd: func() (string, error) { return "", errors.New("getwd boom") },
	}}
	resp := svc.Search(memoryretrieve.SearchInput{Query: "x"})
	if resp.Status != memoryretrieve.StatusInternalError {
		t.Fatalf("status=%s want internal_error; %#v", resp.Status, resp)
	}
}

func TestJSONEnvelopeStable(t *testing.T) {
	svc, _, _, _ := seedMemoryService(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "Stable", Content: "stable json body"},
	})
	resp := svc.Search(memoryretrieve.SearchInput{Query: "Stable"})
	raw := mustJSON(t, resp)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "command", "status", "project_id", "count", "results"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}
	if int(decoded["schema_version"].(float64)) != memoryretrieve.SchemaVersion {
		t.Fatalf("schema=%v", decoded["schema_version"])
	}
}

func seedMemoryService(t *testing.T, rows []memory.Observation) (memoryretrieve.Service, string, string, string) {
	t.Helper()
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "mem-demo")
	projectID, err := home.ProjectID(root, "mem-demo")
	if err != nil {
		t.Fatal(err)
	}
	store, err := memory.Open(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if _, err := store.SaveObservation(row); err != nil {
			t.Fatal(err)
		}
	}
	_ = store.Close()
	svc := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	return svc, homePath, projectID, root
}

func writeProjectConfig(t *testing.T, root, name string) {
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

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertNoDerivedWrites(t *testing.T, homePath, root string) {
	t.Helper()
	// Memory CLI must not create Context Economy artifacts or project .atlas/memory.
	for _, p := range []string{
		filepath.Join(root, ".atlas", "memory"),
		filepath.Join(root, ".atlas", "context", "memory-capsule.md"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("unexpected path %s: %v", p, err)
		}
	}
	_ = homePath
}

func snapshotTreeDir(t *testing.T, dir string) string {
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
			b.WriteString(fmt.Sprintf("%d:%x", info.Size(), data))
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
