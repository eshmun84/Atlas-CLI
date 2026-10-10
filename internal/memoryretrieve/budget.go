package memoryretrieve

import (
	"strings"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
)

// used_chars contract (Slice 36A progressive retrieval):
//
//	used_chars = Σ contextItemCost(item) over returned items
//	contextItemCost = runeLen(id)+runeLen(type)+runeLen(scope)
//	                + runeLen(title)+runeLen(topic_key)+runeLen(excerpt)
//
// Counting unit is Unicode code points (UTF-8 runes), not bytes and not
// JSON encoding overhead. Score and ref are not charged.
// Invariant (always): used_chars <= budget_chars.

// contextItemCost is the character count charged against budget_chars.
func contextItemCost(item ContextItem) int {
	return runeLen(item.ID) +
		runeLen(item.Type) +
		runeLen(item.Scope) +
		runeLen(item.Title) +
		runeLen(item.TopicKey) +
		runeLen(item.Excerpt)
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

// fitContextItem builds a ContextItem that fits in remaining budget characters.
// Returns ok=false when even identity fields (id+type+scope) cannot fit.
// Invariant: when ok, contextItemCost(item) <= remaining.
func fitContextItem(o memory.Observation, score, remaining int) (ContextItem, bool) {
	if remaining <= 0 {
		return ContextItem{}, false
	}
	id := o.ID
	typ := string(o.Type)
	scope := string(o.Scope)
	fixed := runeLen(id) + runeLen(typ) + runeLen(scope)
	if fixed > remaining {
		return ContextItem{}, false
	}
	room := remaining - fixed

	title := strings.TrimSpace(o.Title)
	topic := strings.TrimSpace(o.TopicKey)
	content := strings.TrimSpace(o.Content)

	titleUse := title
	topicUse := topic
	if runeLen(titleUse)+runeLen(topicUse) > room {
		topicUse = ""
	}
	if runeLen(titleUse) > room {
		titleUse = truncateRunes(title, room)
	}
	left := room - runeLen(titleUse) - runeLen(topicUse)
	if left < 0 {
		return ContextItem{}, false
	}

	excerptUse := ""
	if left > 0 && content != "" {
		maxExcerpt := DefaultExcerptChars
		if maxExcerpt > left {
			maxExcerpt = left
		}
		excerptUse = truncateRunes(content, maxExcerpt)
	}

	item := ContextItem{
		ID:       id,
		Type:     typ,
		Scope:    scope,
		Title:    titleUse,
		TopicKey: topicUse,
		Score:    score,
		Excerpt:  excerptUse,
		Ref:      ContextRef{Get: id},
	}
	if contextItemCost(item) > remaining {
		return ContextItem{}, false
	}
	return item, true
}

// selectContextItems packs ranked observations under budget and item limit.
// Guarantees used == Σ contextItemCost(items) and used <= budget.
func selectContextItems(ranked []scoredObs, limit, budget int) (items []ContextItem, used int) {
	if budget < 0 {
		budget = 0
	}
	items = make([]ContextItem, 0, limit)
	for _, r := range ranked {
		if len(items) >= limit {
			break
		}
		item, ok := fitContextItem(r.obs, r.score, budget-used)
		if !ok {
			continue
		}
		items = append(items, item)
		used += contextItemCost(item)
	}
	return items, used
}

type scoredObs struct {
	obs   memory.Observation
	score int
}
