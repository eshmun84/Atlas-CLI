package contextcompose

import (
	"strings"
	"unicode/utf8"
)

// itemCost is the rune charge for one emitted Item.
//
//	itemCost = Σ runeLen over textual fields:
//	  source + kind + id + title + excerpt + path + ref + provenance
//
// Score and Truncated are not charged. JSON encoding overhead is not charged.
// Invariant: used_chars = Σ itemCost(items) and used_chars <= budget_chars.
func itemCost(it Item) int {
	return runeLen(it.Source) +
		runeLen(it.Kind) +
		runeLen(it.ID) +
		runeLen(it.Title) +
		runeLen(it.Excerpt) +
		runeLen(it.Path) +
		runeLen(it.Ref) +
		runeLen(it.Provenance)
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

// truncateRunes returns at most max runes. When truncation occurs the ellipsis
// is included in max so callers can budget exactly against the result length.
func truncateRunes(s string, max int) (out string, truncated bool) {
	s = strings.TrimSpace(s)
	if max <= 0 || s == "" {
		return "", s != "" && max <= 0
	}
	if utf8.RuneCountInString(s) <= max {
		return s, false
	}
	if max == 1 {
		return "…", true
	}
	contentMax := max - 1
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= contentMax {
			break
		}
		b.WriteRune(r)
		n++
	}
	out = strings.TrimSpace(b.String())
	if out == "" {
		return "…", true
	}
	return out + "…", true
}

// sourceCapFor returns the effective per-source rune cap.
// Memory defaults to MemoryBudgetPercent of the global budget when unset.
// Other sources without an explicit cap receive the full global budget
// (still constrained by global remaining at pack time).
func sourceCapFor(source string, global int, caps map[string]int) int {
	if caps != nil {
		if v, ok := caps[source]; ok {
			if v < 0 {
				return 0
			}
			if v > global {
				return global
			}
			return v
		}
	}
	if source == SourceMemory {
		cap := global * MemoryBudgetPercent / 100
		if cap > global {
			return global
		}
		return cap
	}
	return global
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fitItem builds an Item that fits in both remainingGlobal and remainingSource.
// Optional fields are reduced in deterministic peel order when space is tight:
// provenance → ref → excerpt → title → kind → path.
// Returns ok=false when source+id cannot fit.
func fitItem(c Candidate, remainingGlobal, remainingSource int) (Item, bool) {
	remaining := minInt(remainingGlobal, remainingSource)
	if remaining <= 0 {
		return Item{}, false
	}

	fixed := runeLen(c.Source) + runeLen(c.ID)
	if fixed > remaining {
		return Item{}, false
	}

	kind := boundField(c.Kind, MaxKindRunes)
	title := boundField(c.Title, MaxTitleRunes)
	excerpt := boundField(c.Excerpt, MaxExcerptChars)
	path := c.Path
	ref := c.Ref
	prov := boundField(c.Provenance, MaxProvenanceRunes)

	it := Item{
		Source:     c.Source,
		Kind:       kind,
		ID:         c.ID,
		Title:      title,
		Excerpt:    excerpt,
		Path:       path,
		Ref:        ref,
		Score:      c.Score,
		Provenance: prov,
	}

	// Peel optional fields until the item fits.
	for itemCost(it) > remaining {
		switch {
		case it.Provenance != "":
			it.Provenance = ""
			it.Truncated = true
		case it.Ref != "":
			it.Ref = ""
			it.Truncated = true
		case it.Excerpt != "":
			room := remaining - (itemCost(it) - runeLen(it.Excerpt))
			next, tr := truncateRunes(it.Excerpt, room)
			if next == it.Excerpt || next == "" {
				it.Excerpt = ""
			} else {
				it.Excerpt = next
			}
			if tr || it.Excerpt == "" {
				it.Truncated = true
			}
		case it.Title != "":
			room := remaining - (itemCost(it) - runeLen(it.Title))
			next, tr := truncateRunes(it.Title, room)
			if next == it.Title || next == "" {
				it.Title = ""
			} else {
				it.Title = next
			}
			if tr || it.Title == "" {
				it.Truncated = true
			}
		case it.Kind != "":
			it.Kind = ""
			it.Truncated = true
		case it.Path != "":
			it.Path = ""
			it.Truncated = true
		default:
			return Item{}, false
		}
	}

	if itemCost(it) > remaining {
		return Item{}, false
	}

	// Mark truncated when any bounded field differs from the normalized candidate.
	if it.Title != c.Title || it.Excerpt != c.Excerpt || it.Provenance != c.Provenance ||
		it.Kind != c.Kind || it.Path != c.Path || it.Ref != c.Ref {
		it.Truncated = true
	}
	return it, true
}

func boundField(s string, max int) string {
	s = strings.TrimSpace(s)
	if runeLen(s) <= max {
		return s
	}
	out, _ := truncateRunes(s, max)
	return out
}
