package memoryretrieve

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
)

func TestFitContextItem_NeverExceedsRemaining(t *testing.T) {
	o := memory.Observation{
		ID:       "obs_abcdefghijklmnopqrstuvwxyz0123456789",
		Type:     memory.TypeFact,
		Scope:    memory.ScopeProject,
		Title:    strings.Repeat("T", 200),
		TopicKey: strings.Repeat("k", 80),
		Content:  strings.Repeat("memory budget content token ", 100),
	}
	for _, remaining := range []int{0, 1, 5, 10, 20, 40, 80, 120, 200, 500} {
		item, ok := fitContextItem(o, 10, remaining)
		if remaining < runeLen(o.ID)+runeLen(string(o.Type))+runeLen(string(o.Scope)) {
			if ok {
				t.Fatalf("remaining=%d: expected !ok", remaining)
			}
			continue
		}
		if !ok {
			t.Fatalf("remaining=%d: expected ok", remaining)
		}
		cost := contextItemCost(item)
		if cost > remaining {
			t.Fatalf("remaining=%d cost=%d item=%#v", remaining, cost, item)
		}
		if cost != contextItemCost(item) {
			t.Fatal("cost mismatch")
		}
	}
}

func TestSelectContextItems_UsedNeverExceedsBudget(t *testing.T) {
	ranked := []scoredObs{
		{obs: memory.Observation{ID: "obs_a", Type: memory.TypeFact, Scope: memory.ScopeProject, Title: strings.Repeat("Alpha title ", 20), Content: strings.Repeat("alpha body ", 80)}, score: 30},
		{obs: memory.Observation{ID: "obs_b", Type: memory.TypeFact, Scope: memory.ScopeProject, Title: strings.Repeat("Beta title ", 20), Content: strings.Repeat("beta body ", 80)}, score: 20},
		{obs: memory.Observation{ID: "obs_c", Type: memory.TypeFact, Scope: memory.ScopeProject, Title: strings.Repeat("Gamma title ", 20), Content: strings.Repeat("gamma body ", 80)}, score: 10},
	}
	for _, budget := range []int{0, 1, 15, 50, 100, 180, 400, 1000} {
		items, used := selectContextItems(ranked, 10, budget)
		if used > budget {
			t.Fatalf("budget=%d used=%d items=%d", budget, used, len(items))
		}
		sum := 0
		for _, it := range items {
			sum += contextItemCost(it)
		}
		if sum != used {
			t.Fatalf("budget=%d used=%d sum=%d", budget, used, sum)
		}
	}
}

func TestSelectContextItems_TinyBudgetOmitsOrTruncates(t *testing.T) {
	longID := "obs_" + strings.Repeat("x", 60)
	ranked := []scoredObs{
		{obs: memory.Observation{ID: longID, Type: memory.TypeFact, Scope: memory.ScopeProject, Title: "Long", Content: "body"}, score: 1},
		{obs: memory.Observation{ID: "obs_short", Type: memory.TypeFact, Scope: memory.ScopeProject, Title: "S", Content: "b"}, score: 1},
	}
	items, used := selectContextItems(ranked, 5, 25)
	if used > 25 {
		t.Fatalf("used=%d", used)
	}
	for _, it := range items {
		if contextItemCost(it) > 25 {
			t.Fatalf("item over budget: %#v", it)
		}
	}
}
