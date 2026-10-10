package memoryretrieve

import (
	"strings"
	"testing"
)

func TestFTSTokensAndQuery(t *testing.T) {
	tokens := ftsTokens(`Fix Memory Core: path#unsafe? and "quotes"`)
	if len(tokens) < 3 {
		t.Fatalf("tokens=%v", tokens)
	}
	q := buildFTSQuery(tokens)
	if !strings.Contains(q, `"memory"`) || !strings.Contains(q, `"core"`) {
		t.Fatalf("query=%q tokens=%v", q, tokens)
	}
	if strings.Contains(q, "#") || strings.Contains(q, "?") {
		t.Fatalf("special chars leaked into FTS query: %q", q)
	}
	if clampSearchLimit(0) != DefaultSearchLimit || clampSearchLimit(9999) != MaxSearchLimit {
		t.Fatal("search limit clamp")
	}
	if clampBudget(0) != DefaultBudgetChars || clampBudget(1<<40) != MaxBudgetChars {
		t.Fatal("budget clamp")
	}
	got := truncateRunes("abcdefghij", 4)
	if got != "abc…" {
		t.Fatalf("truncate=%q want abc…", got)
	}
	if runeLen(got) != 4 {
		t.Fatalf("truncated runeLen=%d want 4", runeLen(got))
	}
}
