package memorysource_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/memorysource"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"gopkg.in/yaml.v3"
)

func TestRetrieve_SuccessWithObservations(t *testing.T) {
	env, homePath, projectID, root := seedEnv(t, []memory.Observation{
		{Type: memory.TypeArchitecture, Title: "Memory Core", Content: "Memory uses SQLite and FTS5 under Atlas Home", TopicKey: "architecture/memory"},
		{Type: memory.TypeFact, Title: "CodeGraph provider", Content: "CodeGraph is the Code Intelligence provider for Atlas"},
		{Type: memory.TypeConstraint, Title: "No embeddings", Content: "Slice 36B must not introduce vector search"},
	})

	beforeHome := snapshotTree(t, homePath)
	beforeRoot := snapshotTree(t, root)

	res := memorysource.Retrieve(memorysource.Input{
		Task: "memory sqlite architecture",
		Env:  env,
	})
	if res.RetrievalStatus != memoryretrieve.StatusSuccess {
		t.Fatalf("retrieval=%#v", res)
	}
	if res.ProjectID != projectID {
		t.Fatalf("project_id=%q want %q", res.ProjectID, projectID)
	}
	if len(res.Candidates) == 0 {
		t.Fatalf("expected candidates: %#v", res)
	}
	if res.SourceStatus.Source != contextcompose.SourceMemory ||
		res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("source status=%#v", res.SourceStatus)
	}

	if snapshotTree(t, homePath) != beforeHome {
		t.Fatal("Retrieve mutated Atlas Home")
	}
	if snapshotTree(t, root) != beforeRoot {
		t.Fatal("Retrieve mutated project tree")
	}
	assertNoDerivedWrites(t, root)
}

func TestRetrieve_CandidateFieldMapping(t *testing.T) {
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{
			Type:     memory.TypeDecision,
			Title:    "Prefer FTS5",
			Content:  "Use progressive FTS5 retrieval for Memory context packs",
			TopicKey: "retrieval/fts",
		},
	})

	res := memorysource.Retrieve(memorysource.Input{
		Task: "progressive FTS5 retrieval",
		Env:  env,
	})
	if res.RetrievalStatus != memoryretrieve.StatusSuccess || len(res.Candidates) == 0 {
		t.Fatalf("%#v", res)
	}

	// Cross-check against raw Memory Context for exact ID/score/ref preservation.
	raw := memoryretrieve.Service{Env: env}.Context(memoryretrieve.ContextInput{
		Task:   "progressive FTS5 retrieval",
		Budget: res.RetrievalCap,
	})
	if raw.Count == 0 {
		t.Fatal("raw context empty")
	}
	want := raw.Items[0]
	got := findCandidate(res.Candidates, want.ID)
	if got == nil {
		t.Fatalf("missing candidate id=%s in %#v", want.ID, res.Candidates)
	}
	if got.Source != contextcompose.SourceMemory {
		t.Fatalf("source=%q", got.Source)
	}
	if got.Kind != want.Type {
		t.Fatalf("kind=%q want %q", got.Kind, want.Type)
	}
	if got.ID != want.ID {
		t.Fatalf("id=%q want %q", got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Fatalf("title=%q want %q", got.Title, want.Title)
	}
	if got.Excerpt != want.Excerpt {
		t.Fatalf("excerpt=%q want %q", got.Excerpt, want.Excerpt)
	}
	if got.Score != want.Score {
		t.Fatalf("score=%d want %d", got.Score, want.Score)
	}
	if got.Ref != want.Ref.Get {
		t.Fatalf("ref=%q want %q", got.Ref, want.Ref.Get)
	}
	if got.Path != "" {
		t.Fatalf("path must be empty, got %q", got.Path)
	}
	wantProv := want.Scope + ":" + want.TopicKey
	if got.Provenance != wantProv {
		t.Fatalf("provenance=%q want %q", got.Provenance, wantProv)
	}
}

func TestRetrieve_DefaultMemorySourceBudget(t *testing.T) {
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "Budget token", Content: "budget coordination memory source share"},
	})
	res := memorysource.Retrieve(memorysource.Input{
		Task: "budget coordination memory",
		Env:  env,
	})
	want := contextcompose.DefaultBudgetChars * contextcompose.MemoryBudgetPercent / 100
	if want != memoryretrieve.DefaultBudgetChars {
		t.Fatalf("fixture assumption: composer memory share %d != memory default %d", want, memoryretrieve.DefaultBudgetChars)
	}
	if res.RetrievalCap != want {
		t.Fatalf("RetrievalCap=%d want %d (status=%s)", res.RetrievalCap, want, res.RetrievalStatus)
	}
}

func TestRetrieve_ExplicitMemorySourceCap(t *testing.T) {
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "Cap token", Content: "explicit memory source retrieval cap"},
	})
	const cap = 1500
	res := memorysource.Retrieve(memorysource.Input{
		Task:      "explicit memory source retrieval",
		MemoryCap: cap,
		Env:       env,
	})
	if res.RetrievalCap != cap {
		t.Fatalf("RetrievalCap=%d want %d %#v", res.RetrievalCap, cap, res)
	}
}

func TestRetrieve_GlobalBudgetCoordination(t *testing.T) {
	long := strings.Repeat("memory composition budget token ", 40)
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "One", Content: long + " alpha"},
		{Type: memory.TypeFact, Title: "Two", Content: long + " beta"},
		{Type: memory.TypeFact, Title: "Three", Content: long + " gamma"},
	})

	const global = 2000
	res := memorysource.Retrieve(memorysource.Input{
		Task:        "memory composition budget",
		BudgetChars: global,
		Env:         env,
	})
	wantCap := global * contextcompose.MemoryBudgetPercent / 100
	if res.RetrievalCap != wantCap {
		t.Fatalf("RetrievalCap=%d want %d", res.RetrievalCap, wantCap)
	}
	if res.RetrievalUsed > res.RetrievalCap {
		t.Fatalf("memory used=%d > cap=%d", res.RetrievalUsed, res.RetrievalCap)
	}

	composed := contextcompose.Compose(contextcompose.Request{
		Task:           "memory composition budget",
		BudgetChars:    global,
		Sources:        []string{contextcompose.SourceMemory},
		Candidates:     res.Candidates,
		SourceStatuses: []contextcompose.SourceStatus{res.SourceStatus},
		SourceCaps:     map[string]int{contextcompose.SourceMemory: wantCap},
	})
	if composed.UsedChars > composed.BudgetChars {
		t.Fatalf("composer used=%d budget=%d", composed.UsedChars, composed.BudgetChars)
	}
	for _, it := range composed.Items {
		cost := itemCost(it)
		if cost <= 0 {
			t.Fatalf("item cost zero: %#v", it)
		}
	}
	// Separate contracts: Memory used_chars is not Composer itemCost.
	if len(composed.Items) > 0 {
		composerSum := 0
		for _, it := range composed.Items {
			composerSum += itemCost(it)
		}
		if composerSum != composed.UsedChars {
			t.Fatalf("composer used_chars=%d sum=%d", composed.UsedChars, composerSum)
		}
		for _, it := range composed.Items {
			if it.Source != contextcompose.SourceMemory {
				t.Fatalf("unexpected source %q", it.Source)
			}
		}
		// Candidates carried Ref before packing (Composer may peel it under budget).
		for _, c := range res.Candidates {
			if c.Ref == "" || c.Ref != c.ID {
				t.Fatalf("candidate ref must preserve observation id: %#v", c)
			}
		}
	}
}

func TestRetrieve_UnicodeRuneAccounting(t *testing.T) {
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{
			Type:    memory.TypeFact,
			Title:   "Unicode café résumé",
			Content: "记忆 memory 日本語 context pack with emoji 🎯 and accents café",
		},
	})
	res := memorysource.Retrieve(memorysource.Input{
		Task:        "memory context pack café",
		BudgetChars: 500,
		MemoryCap:   250,
		Env:         env,
	})
	if res.RetrievalStatus != memoryretrieve.StatusSuccess && res.RetrievalStatus != memoryretrieve.StatusNoResults {
		t.Fatalf("%#v", res)
	}
	composed := contextcompose.Compose(contextcompose.Request{
		Task:        "memory context pack café",
		BudgetChars: 500,
		Candidates:  res.Candidates,
		SourceCaps:  map[string]int{contextcompose.SourceMemory: 250},
	})
	if composed.UsedChars > composed.BudgetChars {
		t.Fatalf("used=%d budget=%d", composed.UsedChars, composed.BudgetChars)
	}
	sum := 0
	for _, it := range composed.Items {
		sum += itemCost(it)
		if utf8.RuneCountInString(it.Excerpt) > len(it.Excerpt) && !utf8.ValidString(it.Excerpt) {
			t.Fatalf("invalid utf8 excerpt: %q", it.Excerpt)
		}
	}
	if sum != composed.UsedChars {
		t.Fatalf("used_chars=%d sum(itemCost)=%d", composed.UsedChars, sum)
	}
}

func TestRetrieve_MissingMemoryDB(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "missing-db")
	beforeHome := snapshotTree(t, homePath)

	res := memorysource.Retrieve(memorysource.Input{
		Task: "anything",
		Env:  memoryretrieve.Env{Root: root, Home: homePath},
	})
	if res.RetrievalStatus != memoryretrieve.StatusMissingMemoryDB {
		t.Fatalf("status=%s %#v", res.RetrievalStatus, res)
	}
	if res.SourceStatus.Status != contextcompose.SourceStatusUnavailable {
		t.Fatalf("source=%#v", res.SourceStatus)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("candidates=%#v", res.Candidates)
	}
	composed := contextcompose.Compose(contextcompose.Request{
		Task:           "anything",
		Sources:        []string{contextcompose.SourceMemory},
		SourceStatuses: []contextcompose.SourceStatus{res.SourceStatus},
	})
	if composed.Status != contextcompose.StatusPartial && composed.Status != contextcompose.StatusEmpty {
		// unavailable → hasFailure → partial when empty items
		t.Fatalf("compose status=%s", composed.Status)
	}
	if composed.Status != contextcompose.StatusPartial {
		t.Fatalf("want partial for unavailable source, got %s", composed.Status)
	}
	if snapshotTree(t, homePath) != beforeHome {
		t.Fatal("missing-db path mutated Atlas Home")
	}
}

func TestRetrieve_EmptyDatabase(t *testing.T) {
	env, homePath, _, root := seedEnv(t, nil)
	beforeHome := snapshotTree(t, homePath)
	beforeRoot := snapshotTree(t, root)

	res := memorysource.Retrieve(memorysource.Input{
		Task: "no matching observations here",
		Env:  env,
	})
	if res.RetrievalStatus != memoryretrieve.StatusNoResults {
		t.Fatalf("%#v", res)
	}
	if res.SourceStatus.Status != contextcompose.SourceStatusNoCandidates {
		t.Fatalf("source=%#v", res.SourceStatus)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("candidates=%#v", res.Candidates)
	}
	composed := contextcompose.Compose(contextcompose.Request{
		Task:           "no matching observations here",
		Sources:        []string{contextcompose.SourceMemory},
		Candidates:     res.Candidates,
		SourceStatuses: []contextcompose.SourceStatus{res.SourceStatus},
	})
	if composed.Status != contextcompose.StatusEmpty {
		t.Fatalf("compose status=%s want empty", composed.Status)
	}
	if snapshotTree(t, homePath) != beforeHome || snapshotTree(t, root) != beforeRoot {
		t.Fatal("empty-db retrieval mutated filesystem")
	}
}

func TestRetrieve_UnsafeSymlinkDatabase(t *testing.T) {
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

	res := memorysource.Retrieve(memorysource.Input{
		Task: "x",
		Env:  memoryretrieve.Env{Root: root, Home: homePath},
	})
	if res.RetrievalStatus != memoryretrieve.StatusUnsafeFilesystem {
		t.Fatalf("%#v", res)
	}
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("unsafe must not become empty success: %#v", res.SourceStatus)
	}
	if len(res.Candidates) != 0 {
		t.Fatal("candidates must be empty on unsafe storage")
	}
}

func TestRetrieve_MalformedProjectIdentity(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	beforeHome := snapshotTree(t, homePath)

	res := memorysource.Retrieve(memorysource.Input{
		Task: "x",
		Env:  memoryretrieve.Env{Root: root, Home: homePath},
	})
	if res.RetrievalStatus != memoryretrieve.StatusNotInitialized {
		t.Fatalf("status=%s %#v", res.RetrievalStatus, res)
	}
	if res.SourceStatus.Status != contextcompose.SourceStatusUnavailable {
		t.Fatalf("source=%#v", res.SourceStatus)
	}
	if snapshotTree(t, homePath) != beforeHome {
		t.Fatal("malformed identity mutated Atlas Home")
	}
}

func TestRetrieve_ConfigSymlinkUnsafe(t *testing.T) {
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
	beforeHome := snapshotTree(t, homePath)

	res := memorysource.Retrieve(memorysource.Input{
		Task: "x",
		Env:  memoryretrieve.Env{Root: root, Home: homePath},
	})
	if res.RetrievalStatus != memoryretrieve.StatusUnsafeFilesystem {
		t.Fatalf("status=%s %#v", res.RetrievalStatus, res)
	}
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("source=%#v", res.SourceStatus)
	}
	if snapshotTree(t, homePath) != beforeHome {
		t.Fatal("symlink config mutated Atlas Home")
	}
}

func TestRetrieve_CrossProjectIsolation(t *testing.T) {
	homePath := t.TempDir()

	rootA := t.TempDir()
	writeProjectConfig(t, rootA, "proj-a")
	idA, err := home.ProjectID(rootA, "proj-a")
	if err != nil {
		t.Fatal(err)
	}
	storeA, err := memory.Open(homePath, idA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.SaveObservation(memory.Observation{
		Type: memory.TypeFact, Title: "SecretAlpha", Content: "only project A secretalpha token",
	}); err != nil {
		t.Fatal(err)
	}
	_ = storeA.Close()

	rootB := t.TempDir()
	writeProjectConfig(t, rootB, "proj-b")
	idB, err := home.ProjectID(rootB, "proj-b")
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := memory.Open(homePath, idB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeB.SaveObservation(memory.Observation{
		Type: memory.TypeFact, Title: "SecretBeta", Content: "only project B secretbeta token",
	}); err != nil {
		t.Fatal(err)
	}
	_ = storeB.Close()

	resA := memorysource.Retrieve(memorysource.Input{
		Task: "secretalpha token",
		Env:  memoryretrieve.Env{Root: rootA, Home: homePath},
	})
	resB := memorysource.Retrieve(memorysource.Input{
		Task: "secretbeta token",
		Env:  memoryretrieve.Env{Root: rootB, Home: homePath},
	})
	if resA.ProjectID != idA || resB.ProjectID != idB {
		t.Fatalf("ids A=%s/%s B=%s/%s", resA.ProjectID, idA, resB.ProjectID, idB)
	}
	if containsTitle(resA.Candidates, "SecretBeta") {
		t.Fatal("project A leaked project B observation")
	}
	if containsTitle(resB.Candidates, "SecretAlpha") {
		t.Fatal("project B leaked project A observation")
	}

	leak := memorysource.Retrieve(memorysource.Input{
		Task: "secretalpha token",
		Env:  memoryretrieve.Env{Root: rootB, Home: homePath},
	})
	if containsTitle(leak.Candidates, "SecretAlpha") {
		t.Fatal("cross-project isolation failed")
	}
}

func TestRetrieve_DeterministicComposition(t *testing.T) {
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "shared alpha", Content: "shared keyword body one"},
		{Type: memory.TypeFact, Title: "shared beta", Content: "shared keyword body two"},
		{Type: memory.TypeArchitecture, Title: "shared gamma", Content: "shared keyword body three"},
	})

	var first string
	for i := 0; i < 3; i++ {
		res := memorysource.Retrieve(memorysource.Input{
			Task:        "shared keyword body",
			BudgetChars: 4000,
			Env:         env,
		})
		composed := contextcompose.Compose(contextcompose.Request{
			Task:           "shared keyword body",
			BudgetChars:    4000,
			Candidates:     res.Candidates,
			SourceStatuses: []contextcompose.SourceStatus{res.SourceStatus},
		})
		raw, err := json.Marshal(composed)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = string(raw)
			continue
		}
		if string(raw) != first {
			t.Fatalf("non-deterministic composition\n%s\n%s", first, raw)
		}
	}
}

func TestRetrieve_DuplicateObservationIDs(t *testing.T) {
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "Dedup token", Content: "duplicate identity composition token"},
	})
	res := memorysource.Retrieve(memorysource.Input{
		Task: "duplicate identity composition",
		Env:  env,
	})
	if len(res.Candidates) == 0 {
		t.Fatalf("%#v", res)
	}
	dup := res.Candidates[0]
	dup.Title = "Other title"
	dup.Score = res.Candidates[0].Score + 100

	composed := contextcompose.Compose(contextcompose.Request{
		Task: "duplicate identity composition",
		Candidates: []contextcompose.Candidate{
			res.Candidates[0],
			dup,
		},
	})
	count := 0
	for _, it := range composed.Items {
		if it.Source == contextcompose.SourceMemory && it.ID == res.Candidates[0].ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected single packed identity, got %d items=%#v", count, composed.Items)
	}
	foundDupOmit := false
	for _, o := range composed.Omissions {
		if o.Reason == "duplicate_identity" && o.Identity == res.Candidates[0].ID {
			foundDupOmit = true
		}
	}
	if !foundDupOmit {
		t.Fatalf("expected duplicate_identity omission: %#v", composed.Omissions)
	}
}

func TestRetrieve_NoFilesystemOrDatabaseMutation(t *testing.T) {
	env, homePath, projectID, root := seedEnv(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "Immutable", Content: "must not mutate on retrieve"},
	})
	dbRel, err := memory.DatabaseRel(projectID)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(homePath, filepath.FromSlash(dbRel))
	beforeDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeHome := snapshotTree(t, homePath)
	beforeRoot := snapshotTree(t, root)

	_ = memorysource.Retrieve(memorysource.Input{Task: "must not mutate", Env: env})
	_ = memorysource.Retrieve(memorysource.Input{Task: "must not mutate again", Env: env})

	afterDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeDB) != string(afterDB) {
		t.Fatal("memory database bytes changed after Retrieve")
	}
	if snapshotTree(t, homePath) != beforeHome {
		t.Fatal("Atlas Home mutated")
	}
	if snapshotTree(t, root) != beforeRoot {
		t.Fatal("project root mutated")
	}
}

func TestRetrieve_InvalidBudgetRejected(t *testing.T) {
	env, homePath, _, _ := seedEnv(t, []memory.Observation{
		{Type: memory.TypeFact, Title: "X", Content: "y"},
	})
	beforeHome := snapshotTree(t, homePath)

	res := memorysource.Retrieve(memorysource.Input{
		Task:        "x",
		BudgetChars: contextcompose.MaxBudgetChars + 1,
		Env:         env,
	})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if res.RetrievalStatus != "" {
		t.Fatalf("must not call Memory on invalid budget; status=%s", res.RetrievalStatus)
	}
	if snapshotTree(t, homePath) != beforeHome {
		t.Fatal("invalid budget path mutated Home")
	}
}

func TestRetrieve_QueryFallbackAndStatusMappingTable(t *testing.T) {
	env, _, _, _ := seedEnv(t, []memory.Observation{
		{Type: memory.TypePreference, Title: "Query fallback", Content: "query used when task empty"},
	})
	res := memorysource.Retrieve(memorysource.Input{
		Query: "query used when task empty",
		Env:   env,
	})
	if res.RetrievalStatus != memoryretrieve.StatusSuccess {
		t.Fatalf("%#v", res)
	}

	// Empty task+query → invalid before Memory open.
	bad := memorysource.Retrieve(memorysource.Input{Env: env})
	if bad.RetrievalStatus != memoryretrieve.StatusInvalidQuery {
		t.Fatalf("%#v", bad)
	}
	if bad.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("source=%#v", bad.SourceStatus)
	}
}

func TestStatusMapping_RealPaths(t *testing.T) {
	// missing DB → unavailable
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "map-miss")
	miss := memorysource.Retrieve(memorysource.Input{
		Task: "x", Env: memoryretrieve.Env{Root: root, Home: homePath},
	})
	if miss.RetrievalStatus != memoryretrieve.StatusMissingMemoryDB ||
		miss.SourceStatus.Status != contextcompose.SourceStatusUnavailable {
		t.Fatalf("missing → %#v", miss)
	}

	// empty DB / no hits → no_candidates (not fabricated success)
	emptyEnv, _, _, _ := seedEnv(t, nil)
	noRes := memorysource.Retrieve(memorysource.Input{Task: "zzzzznomatch", Env: emptyEnv})
	if noRes.RetrievalStatus != memoryretrieve.StatusNoResults ||
		noRes.SourceStatus.Status != contextcompose.SourceStatusNoCandidates {
		t.Fatalf("empty → %#v", noRes)
	}

	// not initialized → unavailable
	bare := memorysource.Retrieve(memorysource.Input{
		Task: "x", Env: memoryretrieve.Env{Root: t.TempDir(), Home: t.TempDir()},
	})
	if bare.RetrievalStatus != memoryretrieve.StatusNotInitialized ||
		bare.SourceStatus.Status != contextcompose.SourceStatusUnavailable {
		t.Fatalf("not init → %#v", bare)
	}
}

// --- helpers ---

func seedEnv(t *testing.T, rows []memory.Observation) (memoryretrieve.Env, string, string, string) {
	t.Helper()
	root := t.TempDir()
	homePath := t.TempDir()
	writeProjectConfig(t, root, "mem-compose")
	projectID, err := home.ProjectID(root, "mem-compose")
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
	return memoryretrieve.Env{Root: root, Home: homePath}, homePath, projectID, root
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

func findCandidate(cands []contextcompose.Candidate, id string) *contextcompose.Candidate {
	for i := range cands {
		if cands[i].ID == id {
			return &cands[i]
		}
	}
	return nil
}

func containsTitle(cands []contextcompose.Candidate, title string) bool {
	for _, c := range cands {
		if c.Title == title {
			return true
		}
	}
	return false
}

func itemCost(it contextcompose.Item) int {
	return utf8.RuneCountInString(it.Source) +
		utf8.RuneCountInString(it.Kind) +
		utf8.RuneCountInString(it.ID) +
		utf8.RuneCountInString(it.Title) +
		utf8.RuneCountInString(it.Excerpt) +
		utf8.RuneCountInString(it.Path) +
		utf8.RuneCountInString(it.Ref) +
		utf8.RuneCountInString(it.Provenance)
}

func assertNoDerivedWrites(t *testing.T, root string) {
	t.Helper()
	for _, p := range []string{
		filepath.Join(root, ".atlas", "memory"),
		filepath.Join(root, ".atlas", "context", "memory-capsule.md"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("unexpected path %s: %v", p, err)
		}
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
		if info != nil && !info.IsDir() {
			data, _ := os.ReadFile(path)
			b.WriteString(fmt.Sprintf("%d:%x", info.Size(), data))
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
