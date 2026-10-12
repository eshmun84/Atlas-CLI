package cesource_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/cesource"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"gopkg.in/yaml.v3"
)

func TestReadPackBounded_ValidBelowLimit(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	writeValidPack(t, homePath, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "authority"},
	})
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	data, err := cesource.ReadPackBoundedForTest(homePath, rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || len(data) > cesource.MaxStoredPackBytes {
		t.Fatalf("len=%d", len(data))
	}
	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusSupplied {
		t.Fatalf("%#v", res)
	}
}

func TestReadPackBounded_ExactLimitAccepted(t *testing.T) {
	_, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	body := packYAMLBody(t, projectID, task, []atlascontext.PackCandidate{
		{Path: "go.mod", Kind: "manifest", Reason: "module"},
	})
	padded := padPackYAML(body, cesource.MaxStoredPackBytes)
	if len(padded) != cesource.MaxStoredPackBytes {
		t.Fatalf("pad len=%d want %d", len(padded), cesource.MaxStoredPackBytes)
	}
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	writeRawAt(t, homePath, rel, padded)

	data, err := cesource.ReadPackBoundedForTest(homePath, rel)
	if err != nil {
		t.Fatalf("exact limit must succeed: %v", err)
	}
	if len(data) != cesource.MaxStoredPackBytes {
		t.Fatalf("len=%d", len(data))
	}
}

func TestRetrieve_PackExceedingSizeLimitFails(t *testing.T) {
	env, homePath, projectID, root := seedCEOnly(t)
	task := "orient atlas architecture"
	body := packYAMLBody(t, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "authority"},
	})
	oversized := padPackYAML(body, cesource.MaxStoredPackBytes+1)
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	writeRawAt(t, homePath, rel, oversized)

	beforeHome := snapshot(t, homePath)
	beforeRoot := snapshot(t, root)

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "maximum size") {
		t.Fatalf("message=%q", res.Message)
	}
	if len(res.Candidates) != 0 {
		t.Fatal("must not fall back to BuildPack after size rejection")
	}
	if snapshot(t, homePath) != beforeHome || snapshot(t, root) != beforeRoot {
		t.Fatal("mutated filesystem")
	}
}

func TestRetrieve_LargeYAMLFewCandidatesFails(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	// Few candidates but YAML padded far past the bound (YAML bomb shape).
	body := packYAMLBody(t, projectID, task, []atlascontext.PackCandidate{
		{Path: "go.mod", Kind: "manifest", Reason: "module"},
	})
	padded := padPackYAML(body, cesource.MaxStoredPackBytes+4096)
	writeRawAt(t, homePath, atlascontext.ContextPackRel(projectID, packID(task)), padded)

	res := cesource.Retrieve(cesource.Input{Task: task, Env: env})
	if res.SourceStatus.Status != contextcompose.SourceStatusFailed {
		t.Fatalf("%#v", res)
	}
	if !strings.Contains(res.Message, "maximum size") {
		t.Fatalf("message=%q", res.Message)
	}
}

func TestReadPackBounded_ConcurrentGrowthStillBounded(t *testing.T) {
	_, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatal(err)
	}

	// Start near the limit so a concurrent append pushes past Max during read.
	base := packYAMLBody(t, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "authority"},
	})
	near := padPackYAML(base, cesource.MaxStoredPackBytes-64)
	if err := os.WriteFile(abs, []byte(near), 0o600); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		f, err := os.OpenFile(abs, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.Write(bytesRepeat('#', 4096))
	}()

	data, err := cesource.ReadPackBoundedForTest(homePath, rel)
	wg.Wait()
	// Either a bounded success (≤ Max bytes returned) or oversized rejection.
	// Post-read on-disk size may exceed Max if the append won after LimitReader
	// finished — that is not an unbounded acceptance.
	if err == nil {
		if len(data) > cesource.MaxStoredPackBytes {
			t.Fatalf("accepted %d bytes above MaxStoredPackBytes", len(data))
		}
		return
	}
	if !errors.Is(err, cesource.ErrPackOversizedForTest) && !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestRetrieve_MalformedYAMLStillFails(t *testing.T) {
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

func TestCompose_MemoryPartialAfterCESizeFailure(t *testing.T) {
	env, homePath, projectID, _ := seedCEOnly(t)
	// Add Memory observations into the same Home/project.
	store, err := memory.Open(homePath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveObservation(memory.Observation{
		Type: memory.TypeFact, Title: "Survives", Content: "memory survives ce pack size failure token",
	}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	task := "memory survives ce pack size failure token"
	body := packYAMLBody(t, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "authority"},
	})
	writeRawAt(t, homePath, atlascontext.ContextPackRel(projectID, packID(task)), padPackYAML(body, cesource.MaxStoredPackBytes+8))

	res := composeapp.Compose(composeapp.Input{Task: task, Env: env})
	ceFailed := false
	for _, st := range res.Sources {
		if st.Source == contextcompose.SourceContextEconomy && st.Status == contextcompose.SourceStatusFailed {
			ceFailed = true
		}
	}
	if !ceFailed {
		t.Fatalf("CE must fail: %#v", res.Sources)
	}
	if res.Status != contextcompose.StatusPartial {
		t.Fatalf("want partial with Memory usable, got %s %#v", res.Status, res)
	}
	sawMem := false
	for _, it := range res.Items {
		if it.Source == contextcompose.SourceMemory {
			sawMem = true
		}
	}
	if !sawMem {
		t.Fatalf("expected Memory items: %#v", res.Items)
	}
}

func packYAMLBody(t *testing.T, projectID, objective string, cands []atlascontext.PackCandidate) string {
	t.Helper()
	pack := atlascontext.PackDocument{
		SchemaVersion: 1,
		PackID:        packID(objective),
		ProjectID:     projectID,
		Objective:     objective,
		CreatedAt:     "2026-10-12T12:00:00Z",
		Candidates:    cands,
	}
	b, err := yaml.Marshal(&pack)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func padPackYAML(body string, size int) string {
	if len(body) > size {
		return body[:size]
	}
	// YAML allows trailing whitespace/comments; pad with '#' comment lines.
	var b strings.Builder
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	for b.Len() < size {
		need := size - b.Len()
		line := "# " + strings.Repeat("p", 60) + "\n"
		if len(line) > need {
			b.WriteString(strings.Repeat("#", need))
			break
		}
		b.WriteString(line)
	}
	out := b.String()
	if len(out) > size {
		return out[:size]
	}
	if len(out) < size {
		return out + strings.Repeat(" ", size-len(out))
	}
	return out
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestReadPackBounded_SymlinkLeafRejected(t *testing.T) {
	_, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(homePath, "outside.yaml")
	if err := os.WriteFile(outside, []byte("schema_version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, abs); err != nil {
		t.Fatal(err)
	}
	_, err := cesource.ReadPackBoundedForTest(homePath, rel)
	if err == nil {
		t.Fatal("expected symlink leaf refusal")
	}
}

func TestReadPackBounded_RegularFileAccepted(t *testing.T) {
	_, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	body := packYAMLBody(t, projectID, task, []atlascontext.PackCandidate{
		{Path: "go.mod", Kind: "manifest", Reason: "module"},
	})
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	writeRawAt(t, homePath, rel, body)
	data, err := cesource.ReadPackBoundedForTest(homePath, rel)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("body mismatch len=%d want=%d", len(data), len(body))
	}
}

func TestOpenStrategy_Documented(t *testing.T) {
	switch cesource.OpenStrategyForTest {
	case "openat_nofollow", "lstat_open_fallback":
	default:
		t.Fatalf("unexpected OpenStrategy %q", cesource.OpenStrategyForTest)
	}
}

func TestMaxStoredPackBytesJustification(t *testing.T) {
	// Sanity: bound is above worst-case field mass and below multi-MB bombs.
	worst := contextcompose.MaxCEPaths *
		(contextcompose.MaxPathRunes + contextcompose.MaxExcerptChars + contextcompose.MaxKindRunes) * 4
	if cesource.MaxStoredPackBytes < worst {
		t.Fatalf("MaxStoredPackBytes=%d < worst-case field mass %d", cesource.MaxStoredPackBytes, worst)
	}
	if cesource.MaxStoredPackBytes > 1024*1024 {
		t.Fatalf("MaxStoredPackBytes unexpectedly large: %d", cesource.MaxStoredPackBytes)
	}
	_ = fmt.Sprintf("%d", cesource.MaxStoredPackBytes)
}
