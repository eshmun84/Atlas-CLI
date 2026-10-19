package hookactions

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxSessionIDLen = 128

// ValidSessionID reports whether sid is a usable native session/conversation
// identifier for per-session maintenance dedupe (Phase B.8).
//
// Rejects empty values, path-like tokens, and unsafe characters.
// Never invents an identifier — callers must fail-open when this returns false.
func ValidSessionID(sid string) bool {
	s := strings.TrimSpace(sid)
	if s == "" {
		return false
	}
	if !utf8.ValidString(s) {
		return false
	}
	if len(s) > maxSessionIDLen {
		return false
	}
	if strings.Contains(s, "..") || strings.Contains(s, `\`) || strings.Contains(s, "/") {
		return false
	}
	if s == "." || s == ".." {
		return false
	}
	if strings.HasPrefix(s, "/") {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
		if unicode.IsSpace(r) && r != ' ' {
			return false
		}
	}
	return true
}
