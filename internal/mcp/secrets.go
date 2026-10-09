package mcp

import (
	"regexp"
	"strings"
)

var (
	// Heuristics for raw secret material in fields that should only hold references.
	bearerTokenRe = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]{20,}`)
	pemBlockRe    = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	githubPATRe   = regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}\b`)
	genericKeyRe  = regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password)\s*[:=]\s*\S{8,}`)
)

// ContainsSecretMaterial reports whether value looks like a raw credential.
func ContainsSecretMaterial(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if pemBlockRe.MatchString(value) {
		return true
	}
	if bearerTokenRe.MatchString(value) {
		return true
	}
	if githubPATRe.MatchString(value) {
		return true
	}
	if genericKeyRe.MatchString(value) {
		return true
	}
	// Long opaque tokens without separators often indicate pasted secrets.
	if len(value) >= 40 && looksOpaqueToken(value) {
		return true
	}
	return false
}

func looksOpaqueToken(value string) bool {
	hasLetter := false
	hasDigit := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasLetter = true
			continue
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		if r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return hasLetter && hasDigit
}
