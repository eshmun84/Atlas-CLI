package contextcompose

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

func FuzzCompose(f *testing.F) {
	f.Add("task", "query", 8000, "obs_1", "title", "excerpt", "README.md", 10)
	f.Add("", "only-query", 100, "obs_2", "t", "e", "a/b.go", 0)
	f.Add("task", "", 1, "id", stringsRepeat("x", 100), stringsRepeat("y", 400), "../x", 5)
	f.Add("task", "", 32001, "id", "t", "e", "/abs", 1)
	f.Add(stringsRepeat("😀", 100), "", 500, "obs_u", "标题", stringsRepeat("🎉", 50), "ok.go", 99)

	f.Fuzz(func(t *testing.T, task, query string, budget int, id, title, excerpt, path string, score int) {
		req := Request{
			Task:        task,
			Query:       query,
			BudgetChars: budget,
			Candidates: []Candidate{{
				Source:  SourceMemory,
				Kind:    "fact",
				ID:      id,
				Title:   title,
				Excerpt: excerpt,
				Path:    path,
				Score:   score,
			}},
		}
		resp := Compose(req)
		if resp.SchemaVersion != SchemaVersion {
			t.Fatalf("schema")
		}
		if resp.UsedChars > resp.BudgetChars && resp.Status != StatusInvalidRequest {
			t.Fatalf("used=%d budget=%d status=%s", resp.UsedChars, resp.BudgetChars, resp.Status)
		}
		if resp.Count != len(resp.Items) {
			t.Fatalf("count")
		}
		for _, it := range resp.Items {
			if itemCost(it) <= 0 && it.ID == "" {
				t.Fatalf("empty item")
			}
			if !utf8.ValidString(it.Title) || !utf8.ValidString(it.Excerpt) {
				t.Fatal("invalid utf8 in item")
			}
			if itemCost(it) > resp.BudgetChars {
				t.Fatalf("item over global budget")
			}
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		var round Response
		if err := json.Unmarshal(raw, &round); err != nil {
			t.Fatal(err)
		}
	})
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
