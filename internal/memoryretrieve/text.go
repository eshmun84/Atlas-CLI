package memoryretrieve

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func clampSearchLimit(n int) int {
	if n <= 0 {
		return DefaultSearchLimit
	}
	if n > MaxSearchLimit {
		return MaxSearchLimit
	}
	return n
}

func clampContextLimit(n int) int {
	if n <= 0 {
		return DefaultContextLimit
	}
	if n > MaxContextLimit {
		return MaxContextLimit
	}
	return n
}

func clampBudget(n int) int {
	if n <= 0 {
		return DefaultBudgetChars
	}
	if n > MaxBudgetChars {
		return MaxBudgetChars
	}
	return n
}

// truncateRunes returns at most max runes. When truncation occurs the ellipsis
// is included in max so callers can budget exactly against the result length.
func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	contentMax := max - 1 // reserve one rune for ellipsis
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= contentMax {
			break
		}
		b.WriteRune(r)
		n++
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "…"
	}
	return out + "…"
}

func previewText(content string) string {
	return truncateRunes(content, DefaultPreviewChars)
}

// ftsTokens extracts safe FTS5 tokens from free text (context progressive path).
func ftsTokens(task string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		tok := strings.ToLower(strings.TrimSpace(cur.String()))
		cur.Reset()
		if len(tok) < 2 {
			return
		}
		for _, t := range tokens {
			if t == tok {
				return
			}
		}
		tokens = append(tokens, tok)
	}
	for _, r := range task {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	if len(tokens) > 12 {
		tokens = tokens[:12]
	}
	return tokens
}

// buildFTSQuery builds a quoted OR query that avoids raw FTS syntax injection.
func buildFTSQuery(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		// Escape embedded double quotes inside token (defensive).
		safe := strings.ReplaceAll(t, `"`, "")
		if safe == "" {
			continue
		}
		parts = append(parts, `"`+safe+`"`)
	}
	return strings.Join(parts, " OR ")
}

func tokenOverlapScore(hay string, tokens []string) int {
	hay = strings.ToLower(hay)
	score := 0
	for _, t := range tokens {
		if t == "" {
			continue
		}
		if strings.Contains(hay, t) {
			score++
		}
	}
	return score
}
