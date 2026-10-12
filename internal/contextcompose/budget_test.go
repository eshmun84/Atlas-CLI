package contextcompose

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestItemCost_ChargesTextualFieldsOnly(t *testing.T) {
	it := Item{
		Source:     SourceMemory,
		Kind:       "decision",
		ID:         "obs_1",
		Title:      "Hello",
		Excerpt:    "World",
		Path:       "a.go",
		Ref:        "obs_1",
		Score:      999,
		Provenance: "prov",
		Truncated:  true,
	}
	want := runeLen(it.Source) + runeLen(it.Kind) + runeLen(it.ID) +
		runeLen(it.Title) + runeLen(it.Excerpt) + runeLen(it.Path) +
		runeLen(it.Ref) + runeLen(it.Provenance)
	if itemCost(it) != want {
		t.Fatalf("cost=%d want=%d", itemCost(it), want)
	}
}

func TestFitItem_RespectsSourceAndGlobal(t *testing.T) {
	c := Candidate{
		Source:  SourceMemory,
		Kind:    "fact",
		ID:      "obs_abcdefgh",
		Title:   strings.Repeat("T", 80),
		Excerpt: strings.Repeat("E", 200),
		Ref:     "obs_abcdefgh",
		Score:   1,
	}
	for _, rem := range []int{0, 5, 20, 40, 100, 400} {
		it, ok := fitItem(c, rem, rem)
		fixed := runeLen(c.Source) + runeLen(c.ID)
		if rem < fixed {
			if ok {
				t.Fatalf("rem=%d expected !ok", rem)
			}
			continue
		}
		if !ok {
			t.Fatalf("rem=%d expected ok", rem)
		}
		if itemCost(it) > rem {
			t.Fatalf("rem=%d cost=%d %#v", rem, itemCost(it), it)
		}
	}
}

func TestFitItem_SourceCapIndependent(t *testing.T) {
	c := Candidate{
		Source:  SourceMemory,
		ID:      "obs_1",
		Title:   "Title",
		Excerpt: strings.Repeat("x", 100),
		Score:   1,
	}
	it, ok := fitItem(c, 5000, 30)
	if !ok {
		t.Fatal("expected ok")
	}
	if itemCost(it) > 30 {
		t.Fatalf("cost=%d", itemCost(it))
	}
}

func TestTruncateRunes_EllipsisInsideMax(t *testing.T) {
	got, tr := truncateRunes("abcdefghij", 4)
	if !tr || utf8.RuneCountInString(got) != 4 || !strings.HasSuffix(got, "…") {
		t.Fatalf("got=%q tr=%v", got, tr)
	}
}

func TestSourceCapFor_MemoryDefault(t *testing.T) {
	if got := sourceCapFor(SourceMemory, 8000, nil); got != 4000 {
		t.Fatalf("got=%d", got)
	}
	if got := sourceCapFor(SourceContextEconomy, 8000, nil); got != 8000 {
		t.Fatalf("got=%d", got)
	}
	caps := map[string]int{SourceMemory: 100}
	if got := sourceCapFor(SourceMemory, 8000, caps); got != 100 {
		t.Fatalf("got=%d", got)
	}
}
