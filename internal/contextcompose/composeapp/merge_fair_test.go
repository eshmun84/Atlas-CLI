package composeapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
)

func TestMergeCandidatesFair_UnevenOneTwentyTwenty(t *testing.T) {
	t.Parallel()
	groups := []candidateGroup{
		{source: contextcompose.SourceMemory, cands: makeCands(contextcompose.SourceMemory, 1)},
		{source: contextcompose.SourceContextEconomy, cands: makeCands(contextcompose.SourceContextEconomy, 20)},
		{source: contextcompose.SourceCodeGraph, cands: makeCands(contextcompose.SourceCodeGraph, 20)},
	}
	out, _ := mergeCandidatesFair(groups, nil, 32)
	assertMergeCounts(t, out, 32, map[string]int{
		contextcompose.SourceMemory:         1,
		contextcompose.SourceContextEconomy: 20,
		contextcompose.SourceCodeGraph:      11,
	})
}

func TestMergeCandidatesFair_UnevenTwentyOneTwenty(t *testing.T) {
	t.Parallel()
	groups := []candidateGroup{
		{source: contextcompose.SourceMemory, cands: makeCands(contextcompose.SourceMemory, 20)},
		{source: contextcompose.SourceContextEconomy, cands: makeCands(contextcompose.SourceContextEconomy, 1)},
		{source: contextcompose.SourceCodeGraph, cands: makeCands(contextcompose.SourceCodeGraph, 20)},
	}
	out, _ := mergeCandidatesFair(groups, nil, 32)
	assertMergeCounts(t, out, 32, map[string]int{
		contextcompose.SourceMemory:         20,
		contextcompose.SourceContextEconomy: 1,
		contextcompose.SourceCodeGraph:      11,
	})
}

func TestMergeCandidatesFair_UnevenTwentyTwentyOne(t *testing.T) {
	t.Parallel()
	groups := []candidateGroup{
		{source: contextcompose.SourceMemory, cands: makeCands(contextcompose.SourceMemory, 20)},
		{source: contextcompose.SourceContextEconomy, cands: makeCands(contextcompose.SourceContextEconomy, 20)},
		{source: contextcompose.SourceCodeGraph, cands: makeCands(contextcompose.SourceCodeGraph, 1)},
	}
	out, _ := mergeCandidatesFair(groups, nil, 32)
	assertMergeCounts(t, out, 32, map[string]int{
		contextcompose.SourceMemory:         20,
		contextcompose.SourceContextEconomy: 11,
		contextcompose.SourceCodeGraph:      1,
	})
}

func TestMergeCandidatesFair_AllOverQuota(t *testing.T) {
	t.Parallel()
	groups := []candidateGroup{
		{source: contextcompose.SourceMemory, cands: makeCands(contextcompose.SourceMemory, 20)},
		{source: contextcompose.SourceContextEconomy, cands: makeCands(contextcompose.SourceContextEconomy, 20)},
		{source: contextcompose.SourceCodeGraph, cands: makeCands(contextcompose.SourceCodeGraph, 20)},
	}
	out, statuses := mergeCandidatesFair(groups, nil, 32)
	assertMergeCounts(t, out, 32, map[string]int{
		contextcompose.SourceMemory:         11,
		contextcompose.SourceContextEconomy: 11,
		contextcompose.SourceCodeGraph:      10,
	})
	for _, src := range []string{
		contextcompose.SourceMemory,
		contextcompose.SourceContextEconomy,
		contextcompose.SourceCodeGraph,
	} {
		found := false
		for _, st := range statuses {
			if st.Source == src && containsTruncation(st.Message) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected truncation annotation for %s: %#v", src, statuses)
		}
	}
}

func TestMergeCandidatesFair_ZeroCandidateSources(t *testing.T) {
	t.Parallel()
	groups := []candidateGroup{
		{source: contextcompose.SourceMemory, cands: nil},
		{source: contextcompose.SourceContextEconomy, cands: makeCands(contextcompose.SourceContextEconomy, 10)},
		{source: contextcompose.SourceCodeGraph, cands: nil},
	}
	out, _ := mergeCandidatesFair(groups, nil, 32)
	assertMergeCounts(t, out, 10, map[string]int{
		contextcompose.SourceContextEconomy: 10,
	})
}

func TestMergeCandidatesFair_TotalBelowMax(t *testing.T) {
	t.Parallel()
	groups := []candidateGroup{
		{source: contextcompose.SourceMemory, cands: makeCands(contextcompose.SourceMemory, 2)},
		{source: contextcompose.SourceContextEconomy, cands: makeCands(contextcompose.SourceContextEconomy, 3)},
		{source: contextcompose.SourceCodeGraph, cands: makeCands(contextcompose.SourceCodeGraph, 4)},
	}
	out, statuses := mergeCandidatesFair(groups, nil, 32)
	assertMergeCounts(t, out, 9, map[string]int{
		contextcompose.SourceMemory:         2,
		contextcompose.SourceContextEconomy: 3,
		contextcompose.SourceCodeGraph:      4,
	})
	for _, st := range statuses {
		if containsTruncation(st.Message) {
			t.Fatalf("unexpected truncation below max: %#v", statuses)
		}
	}
}

func TestMergeCandidatesFair_Deterministic(t *testing.T) {
	t.Parallel()
	groups := []candidateGroup{
		{source: contextcompose.SourceMemory, cands: makeCands(contextcompose.SourceMemory, 1)},
		{source: contextcompose.SourceContextEconomy, cands: makeCands(contextcompose.SourceContextEconomy, 20)},
		{source: contextcompose.SourceCodeGraph, cands: makeCands(contextcompose.SourceCodeGraph, 20)},
	}
	a, sa := mergeCandidatesFair(groups, nil, 32)
	b, sb := mergeCandidatesFair(groups, nil, 32)
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) != string(bj) {
		t.Fatalf("candidates diverged:\n%s\n%s", aj, bj)
	}
	saj, _ := json.Marshal(sa)
	sbj, _ := json.Marshal(sb)
	if string(saj) != string(sbj) {
		t.Fatalf("statuses diverged:\n%s\n%s", saj, sbj)
	}
}

func makeCands(source string, n int) []contextcompose.Candidate {
	out := make([]contextcompose.Candidate, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, contextcompose.Candidate{
			Source: source,
			ID:     fmt.Sprintf("%s-%d", source, i),
			Title:  fmt.Sprintf("%s-%d", source, i),
			Score:  n - i,
		})
	}
	return out
}

func assertMergeCounts(t *testing.T, out []contextcompose.Candidate, wantTotal int, want map[string]int) {
	t.Helper()
	if len(out) != wantTotal {
		t.Fatalf("total=%d want %d", len(out), wantTotal)
	}
	got := map[string]int{}
	for _, c := range out {
		got[c.Source]++
	}
	for src, n := range want {
		if got[src] != n {
			t.Fatalf("source %s count=%d want %d (all=%v)", src, got[src], n, got)
		}
	}
	for src, n := range got {
		if want[src] != n {
			t.Fatalf("unexpected source %s count=%d want map=%v", src, n, want)
		}
	}
}

func containsTruncation(msg string) bool {
	return strings.Contains(msg, "truncated")
}
