package memorypersist_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist/policy"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"gopkg.in/yaml.v3"
)

func durableCandidate() memorypersist.Candidate {
	return memorypersist.Candidate{
		Type:         string(memory.TypeDecision),
		Title:        "Prefer SQLite Memory Core",
		Content:      "Atlas stores durable project Memory in SQLite under Atlas Home, not in the product repository.",
		Scope:        string(memory.ScopeProject),
		TopicKey:     "memory.storage.engine",
		Provenance:   "cap=test;ref=unit",
		Confidence:   policy.ConfidenceHigh,
		Explicitness: policy.ExplicitnessExplicit,
		Origin:       policy.OriginAtlasLifecycle,
		SourceEvent:  "lifecycle.decision.accepted",
	}
}

func seedProject(t *testing.T) (root, homePath, projectID string) {
	t.Helper()
	root = t.TempDir()
	homePath = t.TempDir()
	name := "persist-demo"
	writeConfig(t, root, name, config.MemoryPersistenceGoverned)
	var err error
	projectID, err = home.ProjectID(root, name)
	if err != nil {
		t.Fatal(err)
	}
	return root, homePath, projectID
}

func writeConfig(t *testing.T, root, name, persistMode string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig(name)
	cfg.Memory.Persistence.Mode = persistMode
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPropose_AcceptedWritesOnce(t *testing.T) {
	t.Parallel()
	root, homePath, projectID := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}

	resp := svc.Propose(durableCandidate())
	if resp.Status != memorypersist.StatusAccepted || resp.Observation == nil {
		t.Fatalf("%#v", resp)
	}
	if resp.Observation.ProjectID != projectID {
		t.Fatalf("project_id=%q want %q", resp.Observation.ProjectID, projectID)
	}
	if !strings.HasPrefix(resp.Observation.SourceKind, "persist:") {
		t.Fatalf("provenance kind=%q", resp.Observation.SourceKind)
	}
	if !strings.Contains(resp.Observation.SourceRef, memorypersist.CapabilitySlice) {
		t.Fatalf("source_ref=%q", resp.Observation.SourceRef)
	}

	store, err := memory.OpenReadOnly(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	list, err := store.ListObservations(memory.ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("count=%d", len(list))
	}
}

func TestPropose_DuplicateReplayNoOp(t *testing.T) {
	t.Parallel()
	root, homePath, _ := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	c := durableCandidate()
	first := svc.Propose(c)
	if first.Status != memorypersist.StatusAccepted {
		t.Fatalf("first=%#v", first)
	}
	second := svc.Propose(c)
	if second.Status != memorypersist.StatusDuplicate {
		t.Fatalf("second=%#v", second)
	}
	if second.Observation == nil || second.Observation.ID != first.Observation.ID {
		t.Fatalf("ids first=%v second=%v", first.Observation, second.Observation)
	}
	if second.Observation.DuplicateCount < 2 {
		t.Fatalf("dup count=%d", second.Observation.DuplicateCount)
	}
}

func TestPropose_RejectedNeverWrites(t *testing.T) {
	t.Parallel()
	root, homePath, projectID := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	c := durableCandidate()
	c.Content = "Export OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz0123456789"
	resp := svc.Propose(c)
	if resp.Status != memorypersist.StatusRejected {
		t.Fatalf("%#v", resp)
	}
	ok, err := memory.Exists(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("rejected candidate must not create memory db")
	}
}

func TestPropose_DisabledMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	homePath := t.TempDir()
	writeConfig(t, root, "off-mode", config.MemoryPersistenceOff)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	resp := svc.Propose(durableCandidate())
	if resp.Status != memorypersist.StatusDisabled {
		t.Fatalf("%#v", resp)
	}
}

func TestPropose_ConfirmationRequired(t *testing.T) {
	t.Parallel()
	root, homePath, projectID := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	c := durableCandidate()
	c.Explicitness = policy.ExplicitnessInferred
	c.Confidence = policy.ConfidenceMedium
	resp := svc.Propose(c)
	if resp.Status != memorypersist.StatusConfirmationRequired {
		t.Fatalf("%#v", resp)
	}
	ok, _ := memory.Exists(homePath, projectID)
	if ok {
		t.Fatal("confirmation_required must not write")
	}
	// Replay inferred — still no write.
	resp2 := svc.Propose(c)
	if resp2.Status != memorypersist.StatusConfirmationRequired {
		t.Fatalf("%#v", resp2)
	}
	ok, _ = memory.Exists(homePath, projectID)
	if ok {
		t.Fatal("inferred replay must not write")
	}
}

func TestPropose_ConfirmedNeverWrites(t *testing.T) {
	t.Parallel()
	root, homePath, projectID := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}

	// Seed one explicit observation so we can assert count stays unchanged
	// after confirmed proposals (DB may already exist).
	if resp := svc.Propose(durableCandidate()); resp.Status != memorypersist.StatusAccepted {
		t.Fatalf("seed %#v", resp)
	}

	cases := []memorypersist.Candidate{
		func() memorypersist.Candidate {
			c := durableCandidate()
			c.Explicitness = policy.ExplicitnessConfirmed
			c.TopicKey = "memory.confirmed.plain"
			return c
		}(),
		func() memorypersist.Candidate {
			c := durableCandidate()
			c.Explicitness = policy.ExplicitnessConfirmed
			c.Origin = policy.OriginDeveloperConfirmation
			c.Confidence = policy.ConfidenceHigh
			c.TopicKey = "memory.confirmed.dev_origin"
			return c
		}(),
		func() memorypersist.Candidate {
			c := durableCandidate()
			c.Explicitness = policy.ExplicitnessConfirmed
			c.Confidence = policy.ConfidenceHigh
			c.TopicKey = "memory.confirmed.high_conf"
			return c
		}(),
	}
	for i, c := range cases {
		resp := svc.Propose(c)
		if resp.Status != memorypersist.StatusConfirmationRequired {
			t.Fatalf("case %d: %#v", i, resp)
		}
	}

	store, err := memory.OpenReadOnly(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	list, err := store.ListObservations(memory.ListFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("confirmed proposals must not change observation count; got %d", len(list))
	}
}

func TestPropose_ProjectIsolation(t *testing.T) {
	t.Parallel()
	rootA, homeA, idA := seedProject(t)
	rootB := t.TempDir()
	writeConfig(t, rootB, "persist-bravo", config.MemoryPersistenceGoverned)
	idB, err := home.ProjectID(rootB, "persist-bravo")
	if err != nil {
		t.Fatal(err)
	}

	svcA := memorypersist.Service{Env: memorypersist.Env{Root: rootA, Home: homeA}}
	resp := svcA.Propose(durableCandidate())
	if resp.Status != memorypersist.StatusAccepted {
		t.Fatalf("%#v", resp)
	}

	storeB, err := memory.Open(homeA, idB)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()
	list, err := storeB.ListObservations(memory.ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("project B saw %d observations from A (%s)", len(list), idA)
	}
}

func TestPropose_RetrievalSeesAccepted(t *testing.T) {
	t.Parallel()
	root, homePath, _ := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	if resp := svc.Propose(durableCandidate()); resp.Status != memorypersist.StatusAccepted {
		t.Fatalf("%#v", resp)
	}
	retrieve := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	ctx := retrieve.Context(memoryretrieve.ContextInput{Task: "SQLite Memory Core storage"})
	if ctx.Status != memoryretrieve.StatusSuccess || ctx.Count < 1 {
		t.Fatalf("%#v", ctx)
	}
	if !strings.Contains(ctx.Items[0].Title, "SQLite") {
		t.Fatalf("%#v", ctx.Items[0])
	}
}

func TestPropose_RejectedAbsentFromRetrieval(t *testing.T) {
	t.Parallel()
	root, homePath, _ := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	c := durableCandidate()
	c.Title = "Temporary debug workaround"
	c.Content = "Use /tmp/scratch for this one-off debugging session only today."
	if resp := svc.Propose(c); resp.Status != memorypersist.StatusRejected {
		t.Fatalf("%#v", resp)
	}
	retrieve := memoryretrieve.Service{Env: memoryretrieve.Env{Root: root, Home: homePath}}
	ctx := retrieve.Context(memoryretrieve.ContextInput{Task: "temporary debug workaround scratch"})
	if ctx.Status != memoryretrieve.StatusMissingMemoryDB && ctx.Status != memoryretrieve.StatusNoResults {
		t.Fatalf("%#v", ctx)
	}
}

func TestPropose_ConcurrentDuplicateSubmission(t *testing.T) {
	t.Parallel()
	root, homePath, projectID := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{Root: root, Home: homePath}}
	c := durableCandidate()

	const n = 8
	var wg sync.WaitGroup
	results := make([]memorypersist.Response, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = svc.Propose(c)
		}(i)
	}
	wg.Wait()

	var accepted, dups int
	for _, r := range results {
		switch r.Status {
		case memorypersist.StatusAccepted:
			accepted++
		case memorypersist.StatusDuplicate:
			dups++
		default:
			t.Fatalf("unexpected %#v", r)
		}
	}
	if accepted < 1 {
		t.Fatalf("expected at least one accepted write, accepted=%d dups=%d", accepted, dups)
	}
	if accepted+dups != n {
		t.Fatalf("accepted=%d dups=%d", accepted, dups)
	}
	store, err := memory.OpenReadOnly(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	list, err := store.ListObservations(memory.ListFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("observations exploded to %d", len(list))
	}
}

func TestPropose_TopicSupersede(t *testing.T) {
	t.Parallel()
	root, homePath, _ := seedProject(t)
	svc := memorypersist.Service{Env: memorypersist.Env{
		Root: root, Home: homePath,
		Clock: func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) },
	}}
	c := durableCandidate()
	first := svc.Propose(c)
	if first.Status != memorypersist.StatusAccepted {
		t.Fatalf("%#v", first)
	}
	c.Content = "Atlas stores durable project Memory in SQLite under Atlas Home with FTS5 search enabled."
	second := svc.Propose(c)
	if second.Status != memorypersist.StatusUpdated {
		t.Fatalf("%#v", second)
	}
	if second.Observation.ID != first.Observation.ID {
		t.Fatalf("expected same id supersede")
	}
	if second.Observation.RevisionCount < 1 {
		t.Fatalf("revision=%d", second.Observation.RevisionCount)
	}
}

func TestPropose_NotInitialized(t *testing.T) {
	t.Parallel()
	svc := memorypersist.Service{Env: memorypersist.Env{Root: t.TempDir(), Home: t.TempDir()}}
	resp := svc.Propose(durableCandidate())
	if resp.Status != memorypersist.StatusNotInitialized {
		t.Fatalf("%#v", resp)
	}
}
