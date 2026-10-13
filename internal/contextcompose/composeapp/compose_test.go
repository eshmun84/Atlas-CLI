package composeapp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"gopkg.in/yaml.v3"
)

func TestCompose_MemoryOnly(t *testing.T) {
	env, homePath, _, root := seedProject(t, true, false, []memory.Observation{
		{Type: memory.TypeArchitecture, Title: "Memory Core", Content: "Memory uses SQLite and FTS5 under Atlas Home"},
	})
	beforeHome := snapshotTree(t, homePath)
	beforeRoot := snapshotTree(t, root)

	res := composeapp.Compose(composeapp.Input{
		Task:    "memory sqlite architecture",
		Sources: []string{contextcompose.SourceMemory},
		Env:     env,
	})
	if res.Command != composeapp.CommandCompose {
		t.Fatalf("command=%q", res.Command)
	}
	if res.Status != contextcompose.StatusSuccess {
		t.Fatalf("%#v", res)
	}
	if res.Count == 0 {
		t.Fatal("expected memory items")
	}
	for _, it := range res.Items {
		if it.Source != contextcompose.SourceMemory {
			t.Fatalf("unexpected source %#v", it)
		}
	}
	if snapshotTree(t, homePath) != beforeHome || snapshotTree(t, root) != beforeRoot {
		t.Fatal("compose mutated filesystem")
	}
}

func TestCompose_CEOnly(t *testing.T) {
	env, _, _, _ := seedProject(t, false, true, nil)
	res := composeapp.Compose(composeapp.Input{
		Task:    "orient to atlas project",
		Sources: []string{contextcompose.SourceContextEconomy},
		Env:     env,
	})
	if res.Status != contextcompose.StatusSuccess {
		t.Fatalf("%#v", res)
	}
	if res.Count == 0 {
		t.Fatal("expected CE items")
	}
	sawPath := false
	for _, it := range res.Items {
		if it.Source != contextcompose.SourceContextEconomy {
			t.Fatalf("unexpected source %#v", it)
		}
		if it.Path != "" {
			sawPath = true
			if strings.HasPrefix(it.Path, "/") {
				t.Fatalf("path must be relative: %q", it.Path)
			}
		}
	}
	if !sawPath {
		t.Fatal("expected at least one path candidate")
	}
}

func TestCompose_CodeGraphUnavailableContinues(t *testing.T) {
	env, _, _, _ := seedProject(t, true, true, []memory.Observation{
		{Type: memory.TypeFact, Title: "Compose token", Content: "memory survives codegraph absence"},
	})
	res := composeapp.Compose(composeapp.Input{
		Task: "memory survives codegraph absence helper",
		Env:  env,
	})
	if res.Status != contextcompose.StatusSuccess && res.Status != contextcompose.StatusPartial {
		t.Fatalf("%#v", res)
	}
	if sourceStatus(res, contextcompose.SourceCodeGraph) != contextcompose.SourceStatusUnavailable {
		t.Fatalf("codegraph status=%q sources=%#v", sourceStatus(res, contextcompose.SourceCodeGraph), res.Sources)
	}
	var sawMem, sawCE bool
	for _, it := range res.Items {
		switch it.Source {
		case contextcompose.SourceMemory:
			sawMem = true
		case contextcompose.SourceContextEconomy:
			sawCE = true
		case contextcompose.SourceCodeGraph:
			t.Fatalf("unexpected codegraph item %#v", it)
		}
	}
	if !sawMem || !sawCE {
		t.Fatalf("expected memory+ce items; mem=%v ce=%v items=%#v", sawMem, sawCE, res.Items)
	}
}

func TestCompose_CombinedMemoryAndCE(t *testing.T) {
	env, _, _, _ := seedProject(t, true, true, []memory.Observation{
		{Type: memory.TypeFact, Title: "Compose token", Content: "combined composition memory and context economy"},
	})
	res := composeapp.Compose(composeapp.Input{
		Task: "combined composition memory context",
		Env:  env,
	})
	if res.Status != contextcompose.StatusSuccess && res.Status != contextcompose.StatusPartial {
		t.Fatalf("%#v", res)
	}
	var sawMem, sawCE bool
	for _, it := range res.Items {
		switch it.Source {
		case contextcompose.SourceMemory:
			sawMem = true
		case contextcompose.SourceContextEconomy:
			sawCE = true
		}
	}
	if !sawMem || !sawCE {
		t.Fatalf("expected both sources; items=%#v sources=%#v", res.Items, res.Sources)
	}
	if res.UsedChars > res.BudgetChars {
		t.Fatalf("used=%d budget=%d", res.UsedChars, res.BudgetChars)
	}
}

func TestCompose_MissingMemory(t *testing.T) {
	env, _, _, _ := seedProject(t, false, true, nil)
	res := composeapp.Compose(composeapp.Input{
		Task: "orient project",
		Env:  env,
	})
	memStatus := sourceStatus(res, contextcompose.SourceMemory)
	if memStatus != contextcompose.SourceStatusUnavailable {
		t.Fatalf("memory status=%q %#v", memStatus, res.Sources)
	}
	if res.Status != contextcompose.StatusPartial && res.Status != contextcompose.StatusSuccess {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestCompose_MissingCE(t *testing.T) {
	env, _, _, _ := seedProject(t, true, false, []memory.Observation{
		{Type: memory.TypeFact, Title: "Only memory", Content: "memory without context economy artifacts"},
	})
	res := composeapp.Compose(composeapp.Input{
		Task: "memory without context economy",
		Env:  env,
	})
	ceStatus := sourceStatus(res, contextcompose.SourceContextEconomy)
	if ceStatus != contextcompose.SourceStatusUnavailable {
		t.Fatalf("ce status=%q %#v", ceStatus, res.Sources)
	}
}

func TestCompose_StaleCE(t *testing.T) {
	env, homePath, projectID, root := seedProject(t, false, true, nil)
	// Mutate project tree so fingerprint drifts.
	if err := os.WriteFile(filepath.Join(root, "extra_stale.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = homePath
	_ = projectID

	res := composeapp.Compose(composeapp.Input{
		Task:    "orient to atlas project",
		Sources: []string{contextcompose.SourceContextEconomy},
		Env:     env,
	})
	if res.Count == 0 {
		t.Fatalf("stale CE should still supply candidates: %#v", res)
	}
	found := false
	for _, st := range res.Sources {
		if st.Source == contextcompose.SourceContextEconomy {
			found = true
			if !strings.Contains(strings.ToLower(st.Message), "fingerprint") &&
				!strings.Contains(strings.ToLower(st.Message), "stale") {
				t.Fatalf("stale message missing honesty: %#v", st)
			}
		}
	}
	if !found {
		t.Fatal("missing CE source status")
	}
}

func TestCompose_UnsafeCEIndex(t *testing.T) {
	env, homePath, projectID, _ := seedProject(t, false, true, nil)
	rel := atlascontext.ContextIndexRel(projectID)
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(homePath, "outside-index.yaml")
	if err := os.WriteFile(outside, []byte("schema_version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, abs); err != nil {
		t.Fatal(err)
	}

	res := composeapp.Compose(composeapp.Input{
		Task:    "x",
		Sources: []string{contextcompose.SourceContextEconomy},
		Env:     env,
	})
	if sourceStatus(res, contextcompose.SourceContextEconomy) != contextcompose.SourceStatusFailed {
		t.Fatalf("unsafe must fail closed: %#v", res.Sources)
	}
	if res.Count != 0 {
		t.Fatal("unsafe must not yield candidates")
	}
}

func TestCompose_ProjectIsolation(t *testing.T) {
	homePath := t.TempDir()

	envA, _, idA, _ := seedNamedProject(t, homePath, "iso-a", true, true, []memory.Observation{
		{Type: memory.TypeFact, Title: "AlphaOnly", Content: "isolation alpha token uniqueaaa"},
	})
	envB, _, idB, _ := seedNamedProject(t, homePath, "iso-b", true, true, []memory.Observation{
		{Type: memory.TypeFact, Title: "BetaOnly", Content: "isolation beta token uniquebbb"},
	})

	resA := composeapp.Compose(composeapp.Input{Task: "isolation alpha token uniqueaaa", Env: envA})
	resB := composeapp.Compose(composeapp.Input{Task: "isolation beta token uniquebbb", Env: envB})
	if resA.ProjectID != idA || resB.ProjectID != idB {
		t.Fatalf("project ids A=%s/%s B=%s/%s", resA.ProjectID, idA, resB.ProjectID, idB)
	}
	if itemTitleHas(resA, "BetaOnly") || itemTitleHas(resB, "AlphaOnly") {
		t.Fatal("cross-project memory leak")
	}
}

func TestCompose_GlobalAndSourceBudgets(t *testing.T) {
	env, _, _, _ := seedProject(t, true, true, []memory.Observation{
		{Type: memory.TypeFact, Title: "BudgetOne", Content: strings.Repeat("budget composition token ", 50)},
		{Type: memory.TypeFact, Title: "BudgetTwo", Content: strings.Repeat("budget composition token ", 50)},
	})
	const global = 2000
	res := composeapp.Compose(composeapp.Input{
		Task:        "budget composition token",
		BudgetChars: global,
		Env:         env,
	})
	if res.BudgetChars != global {
		t.Fatalf("budget=%d", res.BudgetChars)
	}
	if res.UsedChars > global {
		t.Fatalf("used=%d", res.UsedChars)
	}
	memUsed := 0
	for _, it := range res.Items {
		if it.Source == contextcompose.SourceMemory {
			memUsed += itemCost(it)
		}
	}
	cap := global * contextcompose.MemoryBudgetPercent / 100
	if memUsed > cap {
		t.Fatalf("memory used=%d cap=%d", memUsed, cap)
	}
}

func TestCompose_DeterministicAndUnicode(t *testing.T) {
	env, _, _, _ := seedProject(t, true, true, []memory.Observation{
		{Type: memory.TypeFact, Title: "Unicode café", Content: "记忆 context composition 日本語 🎯"},
	})
	var first string
	for i := 0; i < 3; i++ {
		res := composeapp.Compose(composeapp.Input{
			Task:        "context composition café",
			BudgetChars: 3000,
			Env:         env,
		})
		raw, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = string(raw)
			continue
		}
		if string(raw) != first {
			t.Fatalf("non-deterministic\n%s\n%s", first, raw)
		}
	}
	res := composeapp.Compose(composeapp.Input{Task: "context composition café", Env: env})
	sum := 0
	for _, it := range res.Items {
		sum += itemCost(it)
		if !utf8.ValidString(it.Title) || !utf8.ValidString(it.Excerpt) {
			t.Fatal("invalid utf8 in item")
		}
	}
	if sum != res.UsedChars {
		t.Fatalf("used=%d sum=%d", res.UsedChars, sum)
	}
}

func TestCompose_InvalidAndOversizedInput(t *testing.T) {
	env, _, _, _ := seedProject(t, true, true, nil)
	if res := composeapp.Compose(composeapp.Input{Env: env}); res.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("%#v", res)
	}
	if res := composeapp.Compose(composeapp.Input{
		Task:        "x",
		BudgetChars: contextcompose.MaxBudgetChars + 1,
		Env:         env,
	}); res.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("%#v", res)
	}
	huge := strings.Repeat("t", contextcompose.MaxTaskRunes+10)
	if res := composeapp.Compose(composeapp.Input{Task: huge, Env: env}); res.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("%#v", res)
	}
}

func TestCompose_MalformedProject(t *testing.T) {
	res := composeapp.Compose(composeapp.Input{
		Task: "x",
		Env:  memoryretrieve.Env{Root: t.TempDir(), Home: t.TempDir()},
	})
	if res.Status != contextcompose.StatusPartial && res.Status != contextcompose.StatusEmpty {
		// Both sources unavailable → partial with zero items typically.
		if res.Status == contextcompose.StatusSuccess {
			t.Fatalf("must not fabricate success: %#v", res)
		}
	}
	if res.Count != 0 {
		t.Fatalf("count=%d", res.Count)
	}
}

func TestCompose_CandidateOverflowFairness(t *testing.T) {
	homePath := t.TempDir()
	rows := make([]memory.Observation, 0, 40)
	for i := 0; i < 40; i++ {
		rows = append(rows, memory.Observation{
			Type:    memory.TypeFact,
			Title:   fmt.Sprintf("FairMem %02d", i),
			Content: fmt.Sprintf("fairness overflow token body %02d sharedkeyword", i),
		})
	}
	env, _, projectID, root := seedNamedProject(t, homePath, "fair-overflow", true, true, rows)

	// Create real files so the stored pack paths are index members.
	task := "fairness overflow token sharedkeyword"
	cands := make([]atlascontext.PackCandidate, contextcompose.MaxCEPaths)
	for i := range cands {
		rel := fmt.Sprintf("pkg/file_%02d.go", i)
		if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("package pkg\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cands[i] = atlascontext.PackCandidate{
			Path:   rel,
			Reason: "fairness path",
			Kind:   "code",
		}
	}
	rewriteCEIndex(t, root, homePath, projectID, "fair-overflow")
	writeStoredPack(t, homePath, projectID, task, cands)

	beforeHome := snapshotTree(t, homePath)
	beforeRoot := snapshotTree(t, root)

	res := composeapp.Compose(composeapp.Input{
		Task:     task,
		MaxItems: contextcompose.MaxCandidates,
		Env:      env,
	})
	if res.Status == contextcompose.StatusInvalidRequest {
		t.Fatalf("must not reject after fair merge: %#v", res.Error)
	}
	if len(res.Items) > contextcompose.MaxCandidates {
		t.Fatalf("packed items=%d", len(res.Items))
	}

	// Inspect pre-compose fairness via source diversity in response items/sources.
	memN, ceN := 0, 0
	for _, it := range res.Items {
		switch it.Source {
		case contextcompose.SourceMemory:
			memN++
		case contextcompose.SourceContextEconomy:
			ceN++
		}
	}
	if memN == 0 || ceN == 0 {
		t.Fatalf("fairness lost: memory=%d ce=%d items=%#v sources=%#v", memN, ceN, res.Items, res.Sources)
	}
	truncNote := false
	for _, st := range res.Sources {
		if strings.Contains(st.Message, "truncated") {
			truncNote = true
		}
	}
	if !truncNote {
		t.Fatalf("expected truncation annotation on sources: %#v", res.Sources)
	}
	if snapshotTree(t, homePath) != beforeHome || snapshotTree(t, root) != beforeRoot {
		t.Fatal("compose mutated filesystem")
	}
}

func TestCompose_CorruptPackFailsSource(t *testing.T) {
	env, homePath, projectID, _ := seedProject(t, true, true, []memory.Observation{
		{Type: memory.TypeFact, Title: "Keep", Content: "corrupt pack still allows memory"},
	})
	task := "corrupt pack still allows memory"
	id := slugPackID(task)
	rel := atlascontext.ContextPackRel(projectID, id)
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("{{{{bad"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := composeapp.Compose(composeapp.Input{Task: task, Env: env})
	if sourceStatus(res, contextcompose.SourceContextEconomy) != contextcompose.SourceStatusFailed {
		t.Fatalf("CE status=%#v", res.Sources)
	}
	if res.Status != contextcompose.StatusPartial {
		t.Fatalf("want partial when CE failed and Memory may supply, got %s %#v", res.Status, res)
	}
}

func writeStoredPack(t *testing.T, homePath, projectID, objective string, cands []atlascontext.PackCandidate) {
	t.Helper()
	id := slugPackID(objective)
	pack := atlascontext.PackDocument{
		SchemaVersion: 1,
		PackID:        id,
		ProjectID:     projectID,
		Objective:     objective,
		CreatedAt:     "2026-10-12T12:00:00Z",
		Candidates:    cands,
	}
	data, err := yaml.Marshal(&pack)
	if err != nil {
		t.Fatal(err)
	}
	path := atlascontext.PackPath(homePath, projectID, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func rewriteCEIndex(t *testing.T, root, homePath, projectID, name string) {
	t.Helper()
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
}

func slugPackID(objective string) string {
	cleaned := strings.ToLower(strings.TrimSpace(objective))
	var b strings.Builder
	lastDash := false
	for _, r := range cleaned {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "pack"
	}
	if len(id) > 48 {
		id = strings.Trim(id[:48], "-")
	}
	return id
}

// --- helpers ---

func seedProject(t *testing.T, withMemory, withCE bool, rows []memory.Observation) (memoryretrieve.Env, string, string, string) {
	t.Helper()
	return seedNamedProject(t, t.TempDir(), "compose-demo", withMemory, withCE, rows)
}

func seedNamedProject(t *testing.T, homePath, name string, withMemory, withCE bool, rows []memory.Observation) (memoryretrieve.Env, string, string, string) {
	t.Helper()
	root := t.TempDir()
	writeProjectConfig(t, root, name)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectID, err := home.ProjectID(root, name)
	if err != nil {
		t.Fatal(err)
	}

	if withMemory {
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
	}

	if withCE {
		writeCEArtifacts(t, root, homePath, projectID, name)
	}

	return memoryretrieve.Env{Root: root, Home: homePath}, homePath, projectID, root
}

func writeCEArtifacts(t *testing.T, root, homePath, projectID, name string) {
	t.Helper()
	idx, err := atlascontext.BuildIndex(root, name, time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	idx.ProjectID = projectID
	indexYAML, err := atlascontext.RenderIndexYAML(idx)
	if err != nil {
		t.Fatal(err)
	}
	capsule := atlascontext.RenderCapsule(idx)
	pack := atlascontext.BuildPack(idx, atlascontext.DefaultPackObjective, time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC))
	packYAML, err := atlascontext.RenderPackYAML(pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		path string
		data string
	}{
		{atlascontext.IndexPath(homePath, projectID), indexYAML},
		{atlascontext.CapsulePath(homePath, projectID), capsule},
		{atlascontext.PackPath(homePath, projectID, pack.PackID), packYAML},
	} {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.path, []byte(p.data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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

func sourceStatus(res composeapp.Result, source string) string {
	for _, st := range res.Sources {
		if st.Source == source {
			return st.Status
		}
	}
	return ""
}

func itemTitleHas(res composeapp.Result, title string) bool {
	for _, it := range res.Items {
		if it.Title == title {
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
