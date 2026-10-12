package contextcompose_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
)

func baseReq(cands ...contextcompose.Candidate) contextcompose.Request {
	return contextcompose.Request{
		Task:        "compose architecture context",
		BudgetChars: contextcompose.DefaultBudgetChars,
		Candidates:  cands,
	}
}

func mem(id, title, excerpt string, score int) contextcompose.Candidate {
	return contextcompose.Candidate{
		Source:  contextcompose.SourceMemory,
		Kind:    "decision",
		ID:      id,
		Title:   title,
		Excerpt: excerpt,
		Score:   score,
		Ref:     id,
	}
}

func ce(id, path string, score int) contextcompose.Candidate {
	return contextcompose.Candidate{
		Source: contextcompose.SourceContextEconomy,
		Kind:   "path",
		ID:     id,
		Title:  path,
		Path:   path,
		Score:  score,
	}
}

func TestCompose_DeterministicOrderingUnderPermutations(t *testing.T) {
	cands := []contextcompose.Candidate{
		mem("obs_b", "Beta", "b", 10),
		mem("obs_a", "Alpha", "a", 50),
		ce("path_z", "docs/z.md", 5),
		ce("path_a", "AGENTS.md", 100),
		mem("obs_c", "Gamma", "c", 50), // same score as Alpha → id tie-break
	}
	perms := [][]int{
		{0, 1, 2, 3, 4},
		{4, 3, 2, 1, 0},
		{2, 0, 4, 1, 3},
		{3, 1, 4, 0, 2},
	}
	var first string
	for i, p := range perms {
		in := make([]contextcompose.Candidate, len(cands))
		for j, idx := range p {
			in[j] = cands[idx]
		}
		resp := contextcompose.Compose(baseReq(in...))
		if resp.Status != contextcompose.StatusSuccess {
			t.Fatalf("perm %d status=%s err=%v", i, resp.Status, resp.Error)
		}
		raw, err := json.Marshal(resp.Items)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = string(raw)
			continue
		}
		if string(raw) != first {
			t.Fatalf("perm %d diverged\nfirst=%s\ngot=%s", i, first, raw)
		}
	}
	// Memory before CE; within memory score desc then id asc.
	resp := contextcompose.Compose(baseReq(cands...))
	wantIDs := []string{"obs_a", "obs_c", "obs_b", "path_a", "path_z"}
	if resp.Count != len(wantIDs) {
		t.Fatalf("count=%d", resp.Count)
	}
	for i, id := range wantIDs {
		if resp.Items[i].ID != id {
			t.Fatalf("item[%d]=%s want %s", i, resp.Items[i].ID, id)
		}
	}
}

func TestCompose_SourceSpecificDedup(t *testing.T) {
	resp := contextcompose.Compose(baseReq(
		mem("obs_same", "First", "one", 10),
		mem("obs_same", "Second", "two", 90), // higher score wins after sort
		ce("obs_same", "README.md", 5),       // same id, different source — kept
	))
	if resp.Count != 2 {
		t.Fatalf("count=%d omissions=%v", resp.Count, resp.Omissions)
	}
	var sawMem, sawCE bool
	for _, it := range resp.Items {
		switch it.Source {
		case contextcompose.SourceMemory:
			sawMem = true
			if it.Title != "Second" {
				t.Fatalf("expected higher score memory title, got %#v", it)
			}
		case contextcompose.SourceContextEconomy:
			sawCE = true
		}
	}
	if !sawMem || !sawCE {
		t.Fatalf("items=%v", resp.Items)
	}
	dup := 0
	for _, o := range resp.Omissions {
		if o.Reason == "duplicate_identity" {
			dup++
		}
	}
	if dup != 1 {
		t.Fatalf("dup omissions=%d %#v", dup, resp.Omissions)
	}
}

func TestCompose_GlobalBudgetInvariant(t *testing.T) {
	var cands []contextcompose.Candidate
	for i := 0; i < 20; i++ {
		cands = append(cands, mem(
			"obs_"+strings.Repeat("x", 8)+string(rune('a'+i)),
			strings.Repeat("title ", 20),
			strings.Repeat("excerpt body ", 40),
			100-i,
		))
	}
	for _, budget := range []int{1, 50, 100, 500, 2000, 8000} {
		req := baseReq(cands...)
		req.BudgetChars = budget
		resp := contextcompose.Compose(req)
		if resp.Status == contextcompose.StatusInvalidRequest {
			t.Fatalf("budget=%d invalid: %v", budget, resp.Error)
		}
		if resp.UsedChars > resp.BudgetChars {
			t.Fatalf("budget=%d used=%d", budget, resp.UsedChars)
		}
		sum := 0
		for _, it := range resp.Items {
			sum += itemCostExported(it)
		}
		if sum != resp.UsedChars {
			t.Fatalf("budget=%d sum=%d used=%d", budget, sum, resp.UsedChars)
		}
		if resp.Count != len(resp.Items) {
			t.Fatalf("count mismatch")
		}
	}
}

func TestCompose_MemoryFiftyPercentCap(t *testing.T) {
	budget := 1000
	req := baseReq(
		mem("obs_1", strings.Repeat("M", 200), strings.Repeat("m", 280), 100),
		mem("obs_2", strings.Repeat("N", 200), strings.Repeat("n", 280), 90),
		mem("obs_3", strings.Repeat("O", 200), strings.Repeat("o", 280), 80),
		ce("p1", "AGENTS.md", 50),
		ce("p2", "README.md", 40),
	)
	req.BudgetChars = budget
	resp := contextcompose.Compose(req)
	memUsed := 0
	for _, it := range resp.Items {
		if it.Source == contextcompose.SourceMemory {
			memUsed += itemCostExported(it)
		}
	}
	cap := budget * contextcompose.MemoryBudgetPercent / 100
	if memUsed > cap {
		t.Fatalf("memory used=%d cap=%d items=%v", memUsed, cap, resp.Items)
	}
	if resp.UsedChars > budget {
		t.Fatalf("used=%d", resp.UsedChars)
	}
}

func TestCompose_SourceSubCap(t *testing.T) {
	req := baseReq(
		mem("obs_1", "A", strings.Repeat("x", 100), 10),
		mem("obs_2", "B", strings.Repeat("y", 100), 9),
		ce("p1", "a.go", 5),
	)
	req.BudgetChars = 5000
	req.SourceCaps = map[string]int{
		contextcompose.SourceMemory: 40,
	}
	resp := contextcompose.Compose(req)
	memUsed := 0
	for _, it := range resp.Items {
		if it.Source == contextcompose.SourceMemory {
			memUsed += itemCostExported(it)
		}
	}
	if memUsed > 40 {
		t.Fatalf("memUsed=%d items=%#v", memUsed, resp.Items)
	}
}

func TestCompose_UnicodeRuneBoundaries(t *testing.T) {
	// Multi-byte runes: each emoji is 1 rune.
	excerpt := strings.Repeat("😀", 300)
	req := baseReq(mem("obs_u", "标题", excerpt, 1))
	req.BudgetChars = 2000
	resp := contextcompose.Compose(req)
	if resp.Count != 1 {
		t.Fatalf("count=%d status=%s", resp.Count, resp.Status)
	}
	it := resp.Items[0]
	if utf8.RuneCountInString(it.Excerpt) > contextcompose.MaxExcerptChars {
		t.Fatalf("excerpt runes=%d", utf8.RuneCountInString(it.Excerpt))
	}
	if !utf8.ValidString(it.Excerpt) {
		t.Fatal("invalid utf8 excerpt")
	}
	if resp.UsedChars != itemCostExported(it) {
		t.Fatalf("used=%d cost=%d", resp.UsedChars, itemCostExported(it))
	}
}

func TestCompose_ExcerptTruncation(t *testing.T) {
	long := strings.Repeat("abcdefghij", 50) // 500 runes
	resp := contextcompose.Compose(baseReq(mem("obs_t", "T", long, 1)))
	if resp.Count != 1 {
		t.Fatal(resp)
	}
	if utf8.RuneCountInString(resp.Items[0].Excerpt) > contextcompose.MaxExcerptChars {
		t.Fatalf("excerpt too long")
	}
	if !resp.Items[0].Truncated {
		t.Fatal("expected truncated")
	}
	if !strings.HasSuffix(resp.Items[0].Excerpt, "…") {
		t.Fatalf("excerpt=%q", resp.Items[0].Excerpt)
	}
}

func TestCompose_CandidateCountLimit(t *testing.T) {
	cands := make([]contextcompose.Candidate, contextcompose.MaxCandidates+1)
	for i := range cands {
		cands[i] = mem(fmt.Sprintf("obs_%02d", i), "t", "e", i)
	}
	resp := contextcompose.Compose(baseReq(cands...))
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
}

func TestCompose_CEPathCountLimit(t *testing.T) {
	var cands []contextcompose.Candidate
	for i := 0; i < contextcompose.MaxCEPaths+5; i++ {
		id := fmt.Sprintf("path_%02d", i)
		cands = append(cands, ce(id, fmt.Sprintf("dir/file_%02d.go", i), 100-i))
	}
	resp := contextcompose.Compose(baseReq(cands...))
	ceCount := 0
	for _, it := range resp.Items {
		if it.Source == contextcompose.SourceContextEconomy && it.Path != "" {
			ceCount++
		}
	}
	if ceCount > contextcompose.MaxCEPaths {
		t.Fatalf("ce paths=%d", ceCount)
	}
	limitHits := 0
	for _, o := range resp.Omissions {
		if o.Reason == "ce_path_limit" {
			limitHits++
		}
	}
	if limitHits != 5 {
		t.Fatalf("ce_path_limit omissions=%d %#v", limitHits, resp.Omissions)
	}
}

func TestCompose_MissingInvalidIdentity(t *testing.T) {
	resp := contextcompose.Compose(baseReq(
		contextcompose.Candidate{Source: contextcompose.SourceMemory, ID: "", Title: "x", Score: 1},
		mem("obs_ok", "OK", "body", 1),
	))
	if resp.Count != 1 || resp.Items[0].ID != "obs_ok" {
		t.Fatalf("%#v", resp)
	}
	found := false
	for _, o := range resp.Omissions {
		if o.Reason == "missing_identity" {
			found = true
		}
	}
	if !found {
		t.Fatalf("omissions=%v", resp.Omissions)
	}
	if resp.Status != contextcompose.StatusPartial {
		t.Fatalf("status=%s", resp.Status)
	}
}

func TestCompose_AbsoluteAndTraversalRefs(t *testing.T) {
	cases := []contextcompose.Candidate{
		{Source: contextcompose.SourceContextEconomy, ID: "a", Path: "/etc/passwd", Score: 1},
		{Source: contextcompose.SourceContextEconomy, ID: "b", Path: "../secret", Score: 1},
		{Source: contextcompose.SourceContextEconomy, ID: "c", Path: `C:\Windows`, Score: 1},
		{Source: contextcompose.SourceContextEconomy, ID: "d", Path: "foo/../../etc", Score: 1},
		{Source: contextcompose.SourceMemory, ID: "e", Ref: "/abs", Score: 1},
		{Source: contextcompose.SourceMemory, ID: "f", Ref: "../x", Score: 1},
		ce("ok", "internal/context/pack.go", 1),
	}
	resp := contextcompose.Compose(baseReq(cases...))
	if resp.Count != 1 || resp.Items[0].ID != "ok" {
		t.Fatalf("count=%d items=%v omissions=%v", resp.Count, resp.Items, resp.Omissions)
	}
	if resp.Status != contextcompose.StatusPartial {
		t.Fatalf("status=%s", resp.Status)
	}
	unsafe := 0
	for _, o := range resp.Omissions {
		if o.Reason == "unsafe_path" || o.Reason == "unsafe_ref" {
			unsafe++
		}
	}
	if unsafe != 6 {
		t.Fatalf("unsafe=%d %#v", unsafe, resp.Omissions)
	}
}

func TestCompose_InvalidUTF8(t *testing.T) {
	bad := string([]byte{0xff, 0xfe, 0xfd})
	resp := contextcompose.Compose(baseReq(
		contextcompose.Candidate{Source: contextcompose.SourceMemory, ID: "obs_bad", Title: bad, Score: 1},
		mem("obs_ok", "OK", "ok", 1),
	))
	if resp.Count != 1 {
		t.Fatalf("%#v", resp)
	}
	found := false
	for _, o := range resp.Omissions {
		if o.Reason == "invalid_utf8" {
			found = true
		}
	}
	if !found {
		t.Fatalf("omissions=%v", resp.Omissions)
	}
	if resp.Status != contextcompose.StatusPartial {
		t.Fatalf("status=%s", resp.Status)
	}
}

func TestCompose_EmptyCandidates(t *testing.T) {
	resp := contextcompose.Compose(baseReq())
	if resp.Status != contextcompose.StatusEmpty {
		t.Fatalf("status=%s", resp.Status)
	}
	if resp.Count != 0 || resp.UsedChars != 0 {
		t.Fatalf("%#v", resp)
	}
}

func TestCompose_PartialSourceFailure(t *testing.T) {
	req := baseReq(mem("obs_1", "Keep", "body", 10))
	req.Sources = []string{contextcompose.SourceMemory, contextcompose.SourceCodeGraph}
	req.SourceStatuses = []contextcompose.SourceStatus{
		{Source: contextcompose.SourceCodeGraph, Status: contextcompose.SourceStatusFailed, Message: "provider unavailable"},
	}
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusPartial {
		t.Fatalf("status=%s sources=%v", resp.Status, resp.Sources)
	}
	if resp.Count != 1 {
		t.Fatalf("count=%d", resp.Count)
	}
	var sawFail bool
	for _, st := range resp.Sources {
		if st.Source == contextcompose.SourceCodeGraph && st.Status == contextcompose.SourceStatusFailed {
			sawFail = true
		}
	}
	if !sawFail {
		t.Fatalf("sources=%v", resp.Sources)
	}
}

func TestCompose_ConflictingObservationsRetained(t *testing.T) {
	resp := contextcompose.Compose(baseReq(
		mem("obs_dec_a", "Use SQLite", "decision A", 10),
		mem("obs_dec_b", "Use Postgres", "decision B", 10),
	))
	if resp.Count != 2 {
		t.Fatalf("count=%d — conflicts must not be merged", resp.Count)
	}
}

func TestCompose_ProvenanceAccounting(t *testing.T) {
	c := mem("obs_p", "T", "E", 1)
	c.Provenance = "memory:obs_p"
	resp := contextcompose.Compose(baseReq(c))
	if resp.Count != 1 {
		t.Fatal(resp)
	}
	it := resp.Items[0]
	if it.Provenance != "memory:obs_p" {
		t.Fatalf("%#v", it)
	}
	costWithout := itemCostExported(contextcompose.Item{
		Source: it.Source, Kind: it.Kind, ID: it.ID, Title: it.Title, Excerpt: it.Excerpt, Ref: it.Ref,
	})
	if resp.UsedChars != costWithout+utf8.RuneCountInString(it.Provenance) {
		t.Fatalf("used=%d base=%d prov=%d", resp.UsedChars, costWithout, utf8.RuneCountInString(it.Provenance))
	}
}

func TestCompose_RejectOversizeBudget(t *testing.T) {
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.BudgetChars = contextcompose.MaxBudgetChars + 1
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
}

func TestCompose_RejectEmptyTaskAndQuery(t *testing.T) {
	resp := contextcompose.Compose(contextcompose.Request{Candidates: []contextcompose.Candidate{mem("a", "t", "e", 1)}})
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	if resp.Task != "" || resp.Query != "" {
		t.Fatalf("must not echo empty rejection fields: task=%q query=%q", resp.Task, resp.Query)
	}
}

func TestCompose_InvalidRequest_DoesNotEchoOversizedTask(t *testing.T) {
	oversized := strings.Repeat("T", contextcompose.MaxTaskRunes+50)
	resp := contextcompose.Compose(contextcompose.Request{
		Task:       oversized,
		Candidates: []contextcompose.Candidate{mem("obs_1", "t", "e", 1)},
	})
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	if resp.Task != "" || strings.Contains(resp.Task, "T") {
		t.Fatalf("oversized task echoed: %q", resp.Task)
	}
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), oversized) {
		t.Fatal("oversized task present in JSON")
	}
}

func TestCompose_InvalidRequest_DoesNotEchoOversizedQuery(t *testing.T) {
	oversized := strings.Repeat("Q", contextcompose.MaxQueryRunes+50)
	resp := contextcompose.Compose(contextcompose.Request{
		Query:      oversized,
		Candidates: []contextcompose.Candidate{mem("obs_1", "t", "e", 1)},
	})
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	if resp.Query != "" {
		t.Fatalf("oversized query echoed: %q", resp.Query)
	}
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), oversized) {
		t.Fatal("oversized query present in JSON")
	}
}

func TestCompose_InvalidRequest_DoesNotEchoInvalidUTF8(t *testing.T) {
	bad := "bad\xff\xfetask"
	resp := contextcompose.Compose(contextcompose.Request{
		Task:       bad,
		Candidates: []contextcompose.Candidate{mem("obs_1", "t", "e", 1)},
	})
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	if resp.Task != "" || resp.Query != "" {
		t.Fatalf("invalid utf8 echoed: task=%q query=%q", resp.Task, resp.Query)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("JSON marshal failed (invalid UTF-8 leaked?): %v", err)
	}
	if strings.Contains(string(raw), "\xff") || strings.Contains(string(raw), bad) {
		t.Fatalf("invalid utf8 in JSON: %s", raw)
	}
}

func TestCompose_InvalidBudget_DoesNotEchoSensitiveTask(t *testing.T) {
	secret := "password=super-secret-token-do-not-leak"
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.Task = secret
	req.BudgetChars = contextcompose.MaxBudgetChars + 1
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	if resp.Task != "" || resp.Query != "" {
		t.Fatalf("task echoed on budget rejection: %q", resp.Task)
	}
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "super-secret") {
		t.Fatalf("sensitive task in JSON: %s", raw)
	}
}

func TestCompose_InvalidRequest_DeterministicErrorJSON(t *testing.T) {
	mk := func() contextcompose.Response {
		return contextcompose.Compose(contextcompose.Request{
			Task:        strings.Repeat("X", contextcompose.MaxTaskRunes+1),
			BudgetChars: -1,
			Candidates:  []contextcompose.Candidate{mem("obs_1", "t", "e", 1)},
		})
	}
	a, b := mk(), mk()
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	if string(ra) != string(rb) {
		t.Fatalf("non-deterministic error JSON\n%s\n%s", ra, rb)
	}
	if a.Task != "" || a.Query != "" {
		t.Fatalf("echoed fields: %#v", a)
	}
	if a.Error == nil || a.Error.Code != contextcompose.StatusInvalidRequest {
		t.Fatalf("error=%v", a.Error)
	}
}

func TestCompose_JSONSchemaStability(t *testing.T) {
	resp := contextcompose.Compose(baseReq(
		mem("obs_1", "Title", "excerpt", 7),
		ce("p1", "AGENTS.md", 3),
	))
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "status", "budget_chars", "used_chars", "count", "items", "sources", "limits"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}
	var round contextcompose.Response
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if round.SchemaVersion != contextcompose.SchemaVersion || round.Count != resp.Count {
		t.Fatalf("%#v", round)
	}
	again, _ := json.Marshal(round)
	if string(again) != string(raw) {
		t.Fatalf("roundtrip diverged\n%s\n%s", raw, again)
	}
}

func TestCompose_NoFilesystemMutation(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "sentinel.txt")
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = contextcompose.Compose(baseReq(
		mem("obs_1", "t", "e", 1),
		ce("p1", "README.md", 1),
	))
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("directory mutated")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "ok" {
		t.Fatalf("sentinel changed: %s %v", got, err)
	}
}

func TestCompose_NeverClaimsRetrieval(t *testing.T) {
	resp := contextcompose.Compose(baseReq(mem("obs_1", "t", "e", 1)))
	for _, st := range resp.Sources {
		if st.Status == "retrieved" || st.Status == "success" {
			t.Fatalf("must not claim retrieval: %#v", st)
		}
		if st.Status != contextcompose.SourceStatusSupplied {
			t.Fatalf("status=%q", st.Status)
		}
	}
}

func TestCompose_QueryOnlyAllowed(t *testing.T) {
	resp := contextcompose.Compose(contextcompose.Request{
		Query:      "find constraints",
		Candidates: []contextcompose.Candidate{mem("obs_1", "t", "e", 1)},
	})
	if resp.Status != contextcompose.StatusSuccess {
		t.Fatalf("%#v", resp)
	}
}

func TestCompose_MaxItems(t *testing.T) {
	var cands []contextcompose.Candidate
	for i := 0; i < 10; i++ {
		cands = append(cands, mem("obs_"+string(rune('a'+i)), "t", "e", 10-i))
	}
	req := baseReq(cands...)
	req.MaxItems = 3
	resp := contextcompose.Compose(req)
	if resp.Count != 3 {
		t.Fatalf("count=%d", resp.Count)
	}
}

func TestCompose_SameIDTieBreak_PermutationIndependent(t *testing.T) {
	// Same source+id+score+path; differing title/excerpt/provenance must resolve
	// via total field order, never input order.
	a := contextcompose.Candidate{
		Source: contextcompose.SourceMemory, Kind: "decision", ID: "obs_same",
		Title: "Zebra", Excerpt: "second", Path: "", Ref: "obs_same",
		Score: 10, Provenance: "prov-b",
	}
	b := contextcompose.Candidate{
		Source: contextcompose.SourceMemory, Kind: "decision", ID: "obs_same",
		Title: "Alpha", Excerpt: "first", Path: "", Ref: "obs_same",
		Score: 10, Provenance: "prov-a",
	}
	c := contextcompose.Candidate{
		Source: contextcompose.SourceMemory, Kind: "decision", ID: "obs_same",
		Title: "Alpha", Excerpt: "first", Path: "", Ref: "obs_same",
		Score: 10, Provenance: "prov-z",
	}
	perms := [][]contextcompose.Candidate{
		{a, b, c},
		{c, b, a},
		{b, c, a},
		{a, c, b},
	}
	var firstItems, firstOmissions string
	for i, p := range perms {
		resp := contextcompose.Compose(baseReq(p...))
		if resp.Count != 1 {
			t.Fatalf("perm %d count=%d items=%v", i, resp.Count, resp.Items)
		}
		// Title Alpha < Zebra; among Alpha, provenance prov-a < prov-z.
		if resp.Items[0].Title != "Alpha" || resp.Items[0].Provenance != "prov-a" {
			t.Fatalf("perm %d winner=%#v", i, resp.Items[0])
		}
		rawItems, _ := json.Marshal(resp.Items)
		rawOm, _ := json.Marshal(resp.Omissions)
		if i == 0 {
			firstItems, firstOmissions = string(rawItems), string(rawOm)
			continue
		}
		if string(rawItems) != firstItems || string(rawOm) != firstOmissions {
			t.Fatalf("perm %d diverged\nitems %s vs %s\nomissions %s vs %s",
				i, firstItems, rawItems, firstOmissions, rawOm)
		}
	}
}

func TestCompose_DuplicateSourceStatus_Rejected(t *testing.T) {
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.SourceStatuses = []contextcompose.SourceStatus{
		{Source: contextcompose.SourceMemory, Status: contextcompose.SourceStatusFailed, Message: "a"},
		{Source: "  memory  ", Status: contextcompose.SourceStatusUnavailable, Message: "b"},
	}
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s err=%v", resp.Status, resp.Error)
	}
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "duplicate source status") {
		t.Fatalf("error=%v", resp.Error)
	}
}

func TestCompose_DuplicateSourceStatus_IdenticalStillRejected(t *testing.T) {
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.SourceStatuses = []contextcompose.SourceStatus{
		{Source: contextcompose.SourceMemory, Status: contextcompose.SourceStatusFailed, Message: "same"},
		{Source: contextcompose.SourceMemory, Status: contextcompose.SourceStatusFailed, Message: "same"},
	}
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
}

func TestCompose_SourceStatus_PermutationIndependent(t *testing.T) {
	statuses := []contextcompose.SourceStatus{
		{Source: contextcompose.SourceCodeGraph, Status: contextcompose.SourceStatusUnavailable, Message: "missing"},
		{Source: contextcompose.SourceMemory, Status: contextcompose.SourceStatusFailed, Message: "db"},
	}
	perms := [][]contextcompose.SourceStatus{
		{statuses[0], statuses[1]},
		{statuses[1], statuses[0]},
	}
	var first string
	for i, p := range perms {
		req := baseReq(mem("obs_1", "t", "e", 1))
		req.Sources = []string{contextcompose.SourceMemory, contextcompose.SourceCodeGraph}
		req.SourceStatuses = p
		resp := contextcompose.Compose(req)
		if resp.Status != contextcompose.StatusPartial {
			t.Fatalf("perm %d status=%s", i, resp.Status)
		}
		raw, _ := json.Marshal(resp.Sources)
		if i == 0 {
			first = string(raw)
			continue
		}
		if string(raw) != first {
			t.Fatalf("sources diverged\n%s\n%s", first, raw)
		}
	}
}

func TestCompose_StatusSemantics_Matrix(t *testing.T) {
	t.Run("no_items_no_failures_empty", func(t *testing.T) {
		resp := contextcompose.Compose(baseReq())
		if resp.Status != contextcompose.StatusEmpty {
			t.Fatalf("%s", resp.Status)
		}
	})
	t.Run("no_items_source_failure_partial", func(t *testing.T) {
		req := contextcompose.Request{
			Task:    "t",
			Sources: []string{contextcompose.SourceCodeGraph},
			SourceStatuses: []contextcompose.SourceStatus{
				{Source: contextcompose.SourceCodeGraph, Status: contextcompose.SourceStatusFailed, Message: "down"},
			},
		}
		resp := contextcompose.Compose(req)
		if resp.Status != contextcompose.StatusPartial {
			t.Fatalf("status=%s sources=%v", resp.Status, resp.Sources)
		}
		if resp.Count != 0 {
			t.Fatalf("count=%d", resp.Count)
		}
		if len(resp.Sources) != 1 || resp.Sources[0].Status != contextcompose.SourceStatusFailed {
			t.Fatalf("sources=%v", resp.Sources)
		}
	})
	t.Run("no_items_unsafe_omissions_partial", func(t *testing.T) {
		resp := contextcompose.Compose(baseReq(
			contextcompose.Candidate{Source: contextcompose.SourceMemory, ID: "", Title: "x", Score: 1},
			contextcompose.Candidate{Source: contextcompose.SourceContextEconomy, ID: "p", Path: "/abs", Score: 1},
		))
		if resp.Status != contextcompose.StatusPartial {
			t.Fatalf("status=%s omissions=%v", resp.Status, resp.Omissions)
		}
		if resp.Count != 0 {
			t.Fatalf("count=%d", resp.Count)
		}
		if len(resp.Omissions) < 2 {
			t.Fatalf("omissions=%v", resp.Omissions)
		}
	})
	t.Run("items_no_failures_success", func(t *testing.T) {
		resp := contextcompose.Compose(baseReq(mem("obs_1", "t", "e", 1)))
		if resp.Status != contextcompose.StatusSuccess {
			t.Fatalf("%s", resp.Status)
		}
	})
	t.Run("items_failures_partial", func(t *testing.T) {
		req := baseReq(mem("obs_1", "t", "e", 1))
		req.Sources = []string{contextcompose.SourceMemory, contextcompose.SourceCodeGraph}
		req.SourceStatuses = []contextcompose.SourceStatus{
			{Source: contextcompose.SourceCodeGraph, Status: contextcompose.SourceStatusUnavailable},
		}
		resp := contextcompose.Compose(req)
		if resp.Status != contextcompose.StatusPartial || resp.Count != 1 {
			t.Fatalf("status=%s count=%d", resp.Status, resp.Count)
		}
	})
	t.Run("invalid_request", func(t *testing.T) {
		resp := contextcompose.Compose(contextcompose.Request{})
		if resp.Status != contextcompose.StatusInvalidRequest {
			t.Fatalf("%s", resp.Status)
		}
	})
}

func TestCompose_UnknownSourceSupported(t *testing.T) {
	resp := contextcompose.Compose(baseReq(
		contextcompose.Candidate{
			Source: "custom_source", Kind: "note", ID: "c1",
			Title: "Custom", Excerpt: "body", Score: 5,
		},
		mem("obs_1", "Mem", "m", 10),
	))
	if resp.Status != contextcompose.StatusSuccess || resp.Count != 2 {
		t.Fatalf("%#v", resp)
	}
	if resp.Items[0].Source != contextcompose.SourceMemory {
		t.Fatalf("memory should precede unknown: %#v", resp.Items)
	}
	if resp.Items[1].Source != "custom_source" {
		t.Fatalf("%#v", resp.Items[1])
	}
}

func TestCompose_Omission_DoesNotEchoOversizedSource(t *testing.T) {
	oversized := strings.Repeat("S", contextcompose.MaxSourceRunes+40)
	resp := contextcompose.Compose(baseReq(
		contextcompose.Candidate{Source: oversized, ID: "obs_ok_id", Title: "t", Score: 1},
		mem("obs_keep", "Keep", "body", 2),
	))
	if resp.Count != 1 || resp.Items[0].ID != "obs_keep" {
		t.Fatalf("%#v", resp)
	}
	found := false
	for _, o := range resp.Omissions {
		if o.Reason == "source_too_long" {
			found = true
			if o.Source != "" {
				t.Fatalf("oversized source echoed: %q", o.Source)
			}
			if o.Identity != "obs_ok_id" {
				t.Fatalf("safe id should be preserved: %#v", o)
			}
		}
	}
	if !found {
		t.Fatalf("omissions=%v", resp.Omissions)
	}
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), oversized) {
		t.Fatal("oversized source leaked into JSON")
	}
}

func TestCompose_Omission_DoesNotEchoOversizedID(t *testing.T) {
	oversized := strings.Repeat("I", contextcompose.MaxIdentityRunes+40)
	resp := contextcompose.Compose(baseReq(
		contextcompose.Candidate{Source: contextcompose.SourceMemory, ID: oversized, Title: "t", Score: 1},
		mem("obs_keep", "Keep", "body", 2),
	))
	if resp.Count != 1 {
		t.Fatalf("%#v", resp)
	}
	found := false
	for _, o := range resp.Omissions {
		if o.Reason == "identity_too_long" {
			found = true
			if o.Identity != "" {
				t.Fatalf("oversized id echoed: %q", o.Identity)
			}
			if o.Source != contextcompose.SourceMemory {
				t.Fatalf("safe source should be preserved: %#v", o)
			}
		}
	}
	if !found {
		t.Fatalf("omissions=%v", resp.Omissions)
	}
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), oversized) {
		t.Fatal("oversized id leaked into JSON")
	}
}

func TestCompose_Omission_DoesNotEchoInvalidUTF8SourceOrID(t *testing.T) {
	badSrc := "mem\xffory"
	badID := "obs_\xfe_bad"
	resp := contextcompose.Compose(baseReq(
		contextcompose.Candidate{Source: badSrc, ID: "obs_x", Title: "t", Score: 1},
		contextcompose.Candidate{Source: contextcompose.SourceMemory, ID: badID, Title: "t", Score: 1},
		mem("obs_keep", "Keep", "body", 2),
	))
	if resp.Count != 1 || resp.Items[0].ID != "obs_keep" {
		t.Fatalf("%#v", resp)
	}
	utf8Hits := 0
	for _, o := range resp.Omissions {
		if o.Reason != "invalid_utf8" {
			continue
		}
		utf8Hits++
		if o.Source != "" && !utf8.ValidString(o.Source) {
			t.Fatalf("invalid utf8 source in omission: %#v", o)
		}
		if o.Identity != "" && !utf8.ValidString(o.Identity) {
			t.Fatalf("invalid utf8 id in omission: %#v", o)
		}
		if strings.Contains(o.Source, "\xff") || strings.Contains(o.Identity, "\xfe") {
			t.Fatalf("raw invalid bytes echoed: %#v", o)
		}
	}
	if utf8Hits != 2 {
		t.Fatalf("utf8Hits=%d omissions=%v", utf8Hits, resp.Omissions)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "\xff") || strings.Contains(string(raw), "\xfe") {
		t.Fatalf("invalid utf8 in JSON: %s", raw)
	}
}

func TestCompose_Omission_UnsafeRefsBounded(t *testing.T) {
	resp := contextcompose.Compose(baseReq(
		contextcompose.Candidate{Source: contextcompose.SourceContextEconomy, ID: "p1", Path: "/etc/passwd", Score: 1},
		contextcompose.Candidate{Source: contextcompose.SourceMemory, ID: "r1", Ref: "../secret", Score: 1},
		mem("obs_keep", "Keep", "ok", 3),
	))
	if resp.Count != 1 || resp.Status != contextcompose.StatusPartial {
		t.Fatalf("%#v", resp)
	}
	assertOmissionBounds(t, resp)
	reasons := map[string]bool{}
	for _, o := range resp.Omissions {
		reasons[o.Reason] = true
		if o.Source == "" || o.Identity == "" {
			t.Fatalf("safe ids should remain for unsafe path/ref: %#v", o)
		}
	}
	if !reasons["unsafe_path"] || !reasons["unsafe_ref"] {
		t.Fatalf("reasons=%v", reasons)
	}
}

func TestCompose_Omission_MultipleInvalid_DeterministicJSON(t *testing.T) {
	oversizedSrc := strings.Repeat("Z", contextcompose.MaxSourceRunes+10)
	oversizedID := strings.Repeat("Y", contextcompose.MaxIdentityRunes+10)
	cands := []contextcompose.Candidate{
		{Source: oversizedSrc, ID: "id_a", Title: "t", Score: 1},
		{Source: contextcompose.SourceMemory, ID: oversizedID, Title: "t", Score: 1},
		{Source: "bad\xff", ID: "id_b", Title: "t", Score: 1},
		{Source: contextcompose.SourceContextEconomy, ID: "id_c", Path: "/abs", Score: 1},
		{Source: contextcompose.SourceMemory, ID: "id_\xfe", Title: "t", Score: 1},
		mem("obs_keep", "Keep", "body", 9),
	}
	perms := [][]int{
		{0, 1, 2, 3, 4, 5},
		{5, 4, 3, 2, 1, 0},
		{2, 5, 0, 3, 1, 4},
	}
	var first string
	for i, p := range perms {
		in := make([]contextcompose.Candidate, len(cands))
		for j, idx := range p {
			in[j] = cands[idx]
		}
		resp := contextcompose.Compose(baseReq(in...))
		if resp.Count != 1 || resp.Items[0].ID != "obs_keep" {
			t.Fatalf("perm %d %#v", i, resp)
		}
		assertOmissionBounds(t, resp)
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), oversizedSrc) || strings.Contains(string(raw), oversizedID) {
			t.Fatal("oversized metadata leaked")
		}
		if i == 0 {
			first = string(raw)
			continue
		}
		if string(raw) != first {
			t.Fatalf("perm %d non-deterministic\n%s\n%s", i, first, raw)
		}
	}
}

func TestCompose_Omission_ValidCandidateUnchanged(t *testing.T) {
	c := mem("obs_stable", "Stable Title", "stable excerpt", 42)
	c.Provenance = "memory:obs_stable"
	c.Ref = "obs_stable"
	resp := contextcompose.Compose(baseReq(c))
	if resp.Status != contextcompose.StatusSuccess || resp.Count != 1 {
		t.Fatalf("%#v", resp)
	}
	it := resp.Items[0]
	if it.ID != "obs_stable" || it.Title != "Stable Title" || it.Excerpt != "stable excerpt" ||
		it.Provenance != "memory:obs_stable" || it.Ref != "obs_stable" || it.Score != 42 {
		t.Fatalf("%#v", it)
	}
	if len(resp.Omissions) != 0 {
		t.Fatalf("omissions=%v", resp.Omissions)
	}
}

func TestCompose_RejectThousandsOfRequestedSources(t *testing.T) {
	sources := make([]string, 1000)
	for i := range sources {
		sources[i] = fmt.Sprintf("src_%04d", i)
	}
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.Sources = sources
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	assertNoRejectedEcho(t, resp, sources[0], sources[999])
	assertCollectionBounds(t, resp)
}

func TestCompose_RejectThousandsOfSourceStatuses(t *testing.T) {
	statuses := make([]contextcompose.SourceStatus, 1000)
	for i := range statuses {
		statuses[i] = contextcompose.SourceStatus{
			Source:  fmt.Sprintf("src_%04d", i),
			Status:  contextcompose.SourceStatusFailed,
			Message: "x",
		}
	}
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.SourceStatuses = statuses
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	assertNoRejectedEcho(t, resp, "src_0000", "src_0999")
	assertCollectionBounds(t, resp)
}

func TestCompose_RejectThousandsOfSourceCaps(t *testing.T) {
	caps := make(map[string]int, 1000)
	for i := 0; i < 1000; i++ {
		caps[fmt.Sprintf("src_%04d", i)] = 10
	}
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.SourceCaps = caps
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	assertNoRejectedEcho(t, resp, "src_0000", "src_0999")
	assertCollectionBounds(t, resp)
}

func TestCompose_RejectOversizedSourceCapKey(t *testing.T) {
	key := strings.Repeat("K", contextcompose.MaxSourceRunes+1)
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.SourceCaps = map[string]int{key: 100}
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
	assertNoRejectedEcho(t, resp, key)
	assertCollectionBounds(t, resp)
}

func TestCompose_RejectDuplicateNormalizedSourceCapKeys(t *testing.T) {
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.SourceCaps = map[string]int{
		"memory":  100,
		" memory": 200,
	}
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s err=%v", resp.Status, resp.Error)
	}
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "duplicate source cap") {
		t.Fatalf("error=%v", resp.Error)
	}
}

func TestCompose_SourceCapWhitespaceNormalized(t *testing.T) {
	req := baseReq(
		mem("obs_1", "A", strings.Repeat("x", 100), 10),
		mem("obs_2", "B", strings.Repeat("y", 100), 9),
	)
	req.BudgetChars = 5000
	req.SourceCaps = map[string]int{
		"  memory  ": 40,
	}
	resp := contextcompose.Compose(req)
	memUsed := 0
	for _, it := range resp.Items {
		if it.Source == contextcompose.SourceMemory {
			memUsed += itemCostExported(it)
		}
	}
	if memUsed > 40 {
		t.Fatalf("whitespace cap bypass: memUsed=%d items=%#v", memUsed, resp.Items)
	}
}

func TestCompose_RejectDuplicateNormalizedRequestedSources(t *testing.T) {
	req := baseReq(mem("obs_1", "t", "e", 1))
	req.Sources = []string{"memory", "  memory  "}
	resp := contextcompose.Compose(req)
	if resp.Status != contextcompose.StatusInvalidRequest {
		t.Fatalf("status=%s", resp.Status)
	}
}

func TestCompose_ResponseCollectionsBounded(t *testing.T) {
	var cands []contextcompose.Candidate
	for i := 0; i < contextcompose.MaxCandidates; i++ {
		cands = append(cands, contextcompose.Candidate{
			Source: contextcompose.SourceMemory,
			ID:     "",
			Title:  "x",
			Score:  i,
		})
	}
	cands[0] = mem("obs_keep", "Keep", "body", 100)
	resp := contextcompose.Compose(baseReq(cands...))
	assertCollectionBounds(t, resp)
	if len(resp.Omissions) > contextcompose.MaxOmissions {
		t.Fatalf("omissions=%d", len(resp.Omissions))
	}
	if len(resp.Sources) > contextcompose.MaxResponseSources {
		t.Fatalf("sources=%d", len(resp.Sources))
	}
	if len(resp.Limits) > contextcompose.MaxLimitsEntries {
		t.Fatalf("limits=%d", len(resp.Limits))
	}
}

func TestCompose_CollectionReject_DeterministicErrorJSON(t *testing.T) {
	mk := func() contextcompose.Response {
		sources := make([]string, contextcompose.MaxSources+1)
		for i := range sources {
			sources[i] = fmt.Sprintf("s%02d", i)
		}
		req := baseReq(mem("obs_1", "secret-task-token", "e", 1))
		req.Task = "password=should-not-leak"
		req.Sources = sources
		return contextcompose.Compose(req)
	}
	a, b := mk(), mk()
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	if string(ra) != string(rb) {
		t.Fatalf("non-deterministic\n%s\n%s", ra, rb)
	}
	if strings.Contains(string(ra), "password=should-not-leak") {
		t.Fatal("rejected task echoed")
	}
	assertCollectionBounds(t, a)
}

func assertNoRejectedEcho(t *testing.T, resp contextcompose.Response, needles ...string) {
	t.Helper()
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, n := range needles {
		if n != "" && strings.Contains(s, n) {
			t.Fatalf("rejected content %q leaked into JSON", n)
		}
	}
	if resp.Task != "" || resp.Query != "" {
		t.Fatalf("task/query echoed on invalid request: %#v", resp)
	}
}

func assertCollectionBounds(t *testing.T, resp contextcompose.Response) {
	t.Helper()
	if len(resp.Sources) > contextcompose.MaxResponseSources {
		t.Fatalf("sources len=%d", len(resp.Sources))
	}
	if len(resp.Omissions) > contextcompose.MaxOmissions {
		t.Fatalf("omissions len=%d", len(resp.Omissions))
	}
	if len(resp.Limits) > contextcompose.MaxLimitsEntries {
		t.Fatalf("limits len=%d", len(resp.Limits))
	}
	if len(resp.Items) > contextcompose.MaxCandidates {
		t.Fatalf("items len=%d", len(resp.Items))
	}
}

func assertOmissionBounds(t *testing.T, resp contextcompose.Response) {
	t.Helper()
	for _, o := range resp.Omissions {
		if o.Source != "" {
			if !utf8.ValidString(o.Source) {
				t.Fatalf("invalid utf8 source: %#v", o)
			}
			if utf8.RuneCountInString(o.Source) > contextcompose.MaxSourceRunes {
				t.Fatalf("source over bound: %#v", o)
			}
		}
		if o.Identity != "" {
			if !utf8.ValidString(o.Identity) {
				t.Fatalf("invalid utf8 identity: %#v", o)
			}
			if utf8.RuneCountInString(o.Identity) > contextcompose.MaxIdentityRunes {
				t.Fatalf("identity over bound: %#v", o)
			}
		}
		if o.Reason == "" {
			t.Fatalf("missing reason: %#v", o)
		}
	}
}

// itemCostExported mirrors package accounting for black-box verification.
func itemCostExported(it contextcompose.Item) int {
	return utf8.RuneCountInString(it.Source) +
		utf8.RuneCountInString(it.Kind) +
		utf8.RuneCountInString(it.ID) +
		utf8.RuneCountInString(it.Title) +
		utf8.RuneCountInString(it.Excerpt) +
		utf8.RuneCountInString(it.Path) +
		utf8.RuneCountInString(it.Ref) +
		utf8.RuneCountInString(it.Provenance)
}
