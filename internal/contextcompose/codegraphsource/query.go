package codegraphsource

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxQuerySymbolRunes = 128

// Identifier-like tokens suitable for codegraph query <name>.
var identRe = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]{0,127}\b`)

// Words that never make useful CodeGraph symbol queries.
var queryStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "for": true,
	"to": true, "of": true, "in": true, "on": true, "with": true, "from": true,
	"by": true, "as": true, "is": true, "are": true, "be": true, "this": true,
	"that": true, "these": true, "those": true, "it": true, "its": true,
	"how": true, "what": true, "where": true, "when": true, "why": true, "which": true,
	"please": true, "about": true, "using": true, "use": true, "show": true,
	"find": true, "get": true, "list": true, "explain": true, "describe": true,
	"review": true, "check": true, "look": true, "into": true, "across": true,
	"function": true, "functions": true, "method": true, "methods": true,
	"class": true, "classes": true, "type": true, "types": true, "file": true,
	"files": true, "code": true, "caller": true, "callers": true, "callee": true,
	"callees": true, "dependency": true, "dependencies": true, "graph": true,
	"context": true, "compose": true, "atlas": true, "project": true,
	"symbol": true, "symbols": true, "path": true, "paths": true,
	"related": true, "relevant": true, "help": true, "me": true, "my": true,
}

// deriveQuerySymbol extracts one bounded identifier from the task for
// `codegraph query <name>`. Deterministic: prefer CamelCase / snake_case /
// longer tokens; never invent synonyms or call an LLM.
func deriveQuerySymbol(task string) (string, bool) {
	task = strings.TrimSpace(task)
	if task == "" || !utf8.ValidString(task) {
		return "", false
	}
	matches := identRe.FindAllString(task, -1)
	if len(matches) == 0 {
		return "", false
	}

	best := ""
	bestScore := -1
	for _, m := range matches {
		if queryStopwords[strings.ToLower(m)] {
			continue
		}
		if utf8.RuneCountInString(m) > maxQuerySymbolRunes {
			continue
		}
		score := symbolScore(m)
		if score > bestScore || (score == bestScore && m > best) {
			best = m
			bestScore = score
		}
	}
	if best == "" || bestScore < 0 {
		return "", false
	}
	return best, true
}

func symbolScore(s string) int {
	score := utf8.RuneCountInString(s)
	hasUpper := false
	hasLower := false
	hasUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r == '_':
			hasUnderscore = true
		}
	}
	if hasUpper && hasLower {
		score += 20 // CamelCase / mixed
	}
	if hasUnderscore {
		score += 10
	}
	if len(s) >= 2 && s[0] >= 'A' && s[0] <= 'Z' {
		score += 5
	}
	return score
}
