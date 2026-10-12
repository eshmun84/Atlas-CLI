package cesource_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/cesource"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"gopkg.in/yaml.v3"
)

func TestRetrieve_MissingStoredPackFallsBackDeterministically(t *testing.T) {
	env, homePath, _, root := seedCEOnly(t)
	beforeHome := snapshot(t, homePath)
	beforeRoot := snapshot(t, root)

	a := cesource.Retrieve(cesource.Input{Task: "orient atlas architecture", Env: env})
	b := cesource.Retrieve(cesource.Input{Task: "orient atlas architecture", Env: env})
	if a.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", a)
	}
	if len(a.Candidates) == 0 {
		t.Fatal("expected BuildPack fallback candidates")
	}
	for _, c := range a.Candidates {
		if c.Kind != "capsule" && c.Provenance != "context_economy:index" {
			t.Fatalf("fallback provenance=%q candidate=%#v", c.Provenance, c)
		}
	}
	rawA, _ := yaml.Marshal(a.Candidates)
	rawB, _ := yaml.Marshal(b.Candidates)
	if string(rawA) != string(rawB) {
		t.Fatal("deterministic fallback diverged")
	}
	if snapshot(t, homePath) != beforeHome || snapshot(t, root) != beforeRoot {
		t.Fatal("Retrieve mutated filesystem")
	}
}

func TestRetrieve_ValidStoredPackUsed(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeValidPack(t, homePath, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "authority"},
		{Path: "go.mod", Kind: "manifest", Reason: "module"},
	})

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
	sawPack := false
	for _, c := range res.Candidates {
		if c.Path == "AGENTS.md" {
			sawPack = true
			if c.Provenance != "context_economy:pack" {
				t.Fatalf("provenance=%q", c.Provenance)
			}
		}
	}
	if !sawPack {
		t.Fatalf("expected stored pack path: %#v", res.Candidates)
	}
}

func TestRetrieve_SymlinkedPackFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	id := packID(task)
	rel := atlascontext.ContextPackRel(projectID, id)
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(homePath, "outside-pack.yaml")
	if err := os.WriteFile(outside, []byte("schema_version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, abs); err != nil {
		t.Fatal(err)
	}

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("symlink must fail closed: %#v", res)
	}
	if len(res.Candidates) != 0 {
		t.Fatal("no candidates on integrity failure")
	}
	if !strings.Contains(res.Message, "unsafe") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_CorruptYAMLFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeRawPack(t, homePath, projectID, packID(task), "{{{{not-yaml")

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "yaml") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_InvalidSchemaFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writePackDoc(t, homePath, projectID, atlascontext.PackDocument{
		SchemaVersion: 99,
		PackID:        packID(task),
		ProjectID:     projectID,
		Objective:     task,
		Candidates:    []atlascontext.PackCandidate{{Path: "AGENTS.md", Reason: "x"}},
	})

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "schema") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_WrongProjectIDFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writePackDoc(t, homePath, projectID, atlascontext.PackDocument{
		SchemaVersion: 1,
		PackID:        packID(task),
		ProjectID:     "other-project-id-ffffffffffffffff",
		Objective:     task,
		Candidates:    []atlascontext.PackCandidate{{Path: "AGENTS.md", Reason: "x"}},
	})

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "project identity") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_WrongPackIDFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	// File lives under the expected pack path but claims a different pack_id.
	id := packID(task)
	rel := atlascontext.ContextPackRel(projectID, id)
	writeRawAt(t, homePath, rel, mustYAML(t, atlascontext.PackDocument{
		SchemaVersion: 1,
		PackID:        "wrong-id",
		ProjectID:     projectID,
		Objective:     task,
		Candidates:    []atlascontext.PackCandidate{{Path: "AGENTS.md", Reason: "x"}},
	}))

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "pack identity") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_WrongObjectiveFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writePackDoc(t, homePath, projectID, atlascontext.PackDocument{
		SchemaVersion: 1,
		PackID:        packID(task),
		ProjectID:     projectID,
		Objective:     "completely different objective",
		Candidates:    []atlascontext.PackCandidate{{Path: "AGENTS.md", Reason: "x"}},
	})

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "objective") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_OversizedStoredPackFails(t *testing.T) {
	env, homePath, projectID, root := seedCEOnly(t)
	task := "orient atlas architecture"
	// Create enough real indexed files so membership is not the failure mode.
	for i := 0; i < contextcompose.MaxCEPaths+1; i++ {
		writeFile(t, filepath.Join(root, fmt.Sprintf("file_%02d.go", i)), "package main\n")
	}
	rewriteIndex(t, root, homePath, projectID, "ce-pack-demo")

	cands := make([]atlascontext.PackCandidate, contextcompose.MaxCEPaths+1)
	for i := range cands {
		cands[i] = atlascontext.PackCandidate{
			Path:   fmt.Sprintf("file_%02d.go", i),
			Reason: "overflow",
			Kind:   "code",
		}
	}
	writePackDoc(t, homePath, projectID, atlascontext.PackDocument{
		SchemaVersion: 1,
		PackID:        packID(task),
		ProjectID:     projectID,
		Objective:     task,
		Candidates:    cands,
	})

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "exceeds maximum") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_IndexedPathAccepted(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeValidPack(t, homePath, projectID, task, []atlascontext.PackCandidate{
		{Path: "go.mod", Kind: "manifest", Reason: "module manifest"},
	})
	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
	found := false
	for _, c := range res.Candidates {
		if c.Path == "go.mod" && c.Provenance == "context_economy:pack" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected indexed go.mod from pack: %#v", res.Candidates)
	}
}

func TestRetrieve_GovernancePathAccepted(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeValidPack(t, homePath, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "project authority"},
	})
	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
	found := false
	for _, c := range res.Candidates {
		if c.Path == "AGENTS.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected governance AGENTS.md: %#v", res.Candidates)
	}
}

func TestRetrieve_PathAbsentFromIndexFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeValidPack(t, homePath, projectID, task, []atlascontext.PackCandidate{
		{Path: "does/not/exist.go", Kind: "code", Reason: "fabricated"},
	})
	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "not in index") {
		t.Fatalf("message=%q", res.Message)
	}
	if len(res.Candidates) != 0 {
		t.Fatal("must not fall back to BuildPack")
	}
}

func TestRetrieve_MixedValidInvalidCandidatesFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeValidPack(t, homePath, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "authority"},
		{Path: "ghost/path.go", Kind: "code", Reason: "absent"},
	})
	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "not in index") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestRetrieve_EmptyCandidateListOK(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeValidPack(t, homePath, projectID, task, nil)
	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	// Empty pack is valid; capsule alone may still supply candidates.
	if res.SourceStatus.Status == contextcompose.SourceStatusFailed {
		t.Fatalf("empty candidates must not fail integrity: %#v", res)
	}
	for _, c := range res.Candidates {
		if c.Path == "ghost" {
			t.Fatal("unexpected path")
		}
	}
}

func TestRetrieve_CrossProjectPathRejected(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	// Path shaped like another project's relative tree entry, absent here.
	writeValidPack(t, homePath, projectID, task, []atlascontext.PackCandidate{
		{Path: "other-project/secret.go", Kind: "code", Reason: "cross"},
	})
	a := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	b := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if a.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", a)
	}
	if a.Message != b.Message || a.SourceStatus.Status != b.SourceStatus.Status {
		t.Fatalf("non-deterministic\n%#v\n%#v", a, b)
	}
	if !strings.Contains(a.Message, "not in index") {
		t.Fatalf("message=%q", a.Message)
	}
}

func TestRetrieve_CorruptDefaultPackDoesNotSilentFallback(t *testing.T) {
	// Task pack missing; default pack present but corrupt → must fail, not BuildPack.
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "unique-task-without-stored-pack"
	defID := packID(atlascontext.DefaultPackObjective)
	writeRawPack(t, homePath, projectID, defID, "not: [valid: yaml:")

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("corrupt default pack must not fall back: %#v", res)
	}
}

func TestRetrieve_ReadOnlyNoMutation(t *testing.T) {
	env, homePath, projectID, root := seedCEOnly(t)
	writeValidPack(t, homePath, projectID, "orient atlas architecture", []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Reason: "authority", Kind: "runtime_atlas"},
	})
	beforeHome := snapshot(t, homePath)
	beforeRoot := snapshot(t, root)
	_ = cesource.Retrieve(cesource.Input{Task: "orient atlas architecture", Env: env})
	_ = cesource.Retrieve(cesource.Input{Task: "missing pack objective zzz", Env: env})
	if snapshot(t, homePath) != beforeHome || snapshot(t, root) != beforeRoot {
		t.Fatal("mutated filesystem")
	}
}

// --- helpers ---

func seedCEOnly(t *testing.T) (memoryretrieve.Env, string, string, string) {
	t.Helper()
	root := t.TempDir()
	homePath := t.TempDir()
	name := "ce-pack-demo"
	writeConfig(t, root, name)
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
	return memoryretrieve.Env{Root: root, Home: homePath}, homePath, projectID, root
}

func writeConfig(t *testing.T, root, name string) {
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

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rewriteIndex(t *testing.T, root, homePath, projectID, name string) {
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

func writeValidPack(t *testing.T, homePath, projectID, objective string, cands []atlascontext.PackCandidate) {
	t.Helper()
	writePackDoc(t, homePath, projectID, atlascontext.PackDocument{
		SchemaVersion: 1,
		PackID:        packID(objective),
		ProjectID:     projectID,
		Objective:     objective,
		CreatedAt:     "2026-10-12T12:00:00Z",
		Candidates:    cands,
	})
}

func writePackDoc(t *testing.T, homePath, projectID string, pack atlascontext.PackDocument) {
	t.Helper()
	id := strings.TrimSpace(pack.PackID)
	if id == "" {
		id = packID(pack.Objective)
	}
	rel := atlascontext.ContextPackRel(projectID, id)
	writeRawAt(t, homePath, rel, mustYAML(t, pack))
}

func writeRawPack(t *testing.T, homePath, projectID, packID, raw string) {
	t.Helper()
	writeRawAt(t, homePath, atlascontext.ContextPackRel(projectID, packID), raw)
}

func writeRawAt(t *testing.T, homePath, rel, raw string) {
	t.Helper()
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustYAML(t *testing.T, v any) string {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func packID(objective string) string {
	// Mirror cesource/context packIDFor for test path placement.
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

func snapshot(t *testing.T, dir string) string {
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
			b.WriteString(fmt.Sprintf("%d:%x", info.Size(), data))
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
