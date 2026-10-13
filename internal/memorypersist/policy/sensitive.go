package policy

import (
	"regexp"
	"strings"
	"unicode"
)

// Conservative V1 secret/credential heuristics. Prefer false negatives toward
// reject / require_confirmation rather than persisting suspicious material.
var (
	rePEMPrivateKey  = regexp.MustCompile(`(?i)-----BEGIN[^\n]*PRIVATE KEY-----`)
	reBearerToken    = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-_\.=+/]{20,}`)
	reJWT            = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`)
	reGitHubPAT      = regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`)
	reAWSAccessKey   = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	reSlackToken     = regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)
	reOpenAIKey      = regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`)
	reGenericSecret  = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|secret[_-]?key|client[_-]?secret)\b\s*[:=]\s*\S{8,}`)
	rePasswordAssign = regexp.MustCompile(`(?i)\b(password|passwd|pwd)\b\s*[:=]\s*\S{4,}`)
	reEnvCredential  = regexp.MustCompile(`(?i)\b(AWS_SECRET_ACCESS_KEY|GITHUB_TOKEN|OPENAI_API_KEY|ANTHROPIC_API_KEY|DATABASE_URL|POSTGRES_PASSWORD|MYSQL_PWD)\b\s*=\s*\S+`)
)

func sensitivityReject(text string) (Reason, bool) {
	if text == "" {
		return Reason{}, false
	}
	checks := []struct {
		re   *regexp.Regexp
		code string
		msg  string
	}{
		{rePEMPrivateKey, "private_key", "candidate appears to contain a private key"},
		{reBearerToken, "bearer_token", "candidate appears to contain a bearer token"},
		{reJWT, "jwt", "candidate appears to contain a JWT"},
		{reGitHubPAT, "github_pat", "candidate appears to contain a GitHub token"},
		{reAWSAccessKey, "aws_access_key", "candidate appears to contain an AWS access key"},
		{reSlackToken, "slack_token", "candidate appears to contain a Slack token"},
		{reOpenAIKey, "api_key", "candidate appears to contain an API key"},
		{reGenericSecret, "credential_assignment", "candidate appears to contain a credential assignment"},
		{rePasswordAssign, "password", "candidate appears to contain a password"},
		{reEnvCredential, "env_credential", "candidate appears to contain a credential-like environment assignment"},
	}
	for _, c := range checks {
		if c.re.MatchString(text) {
			return Reason{Gate: GateSensitivity, Code: c.code, Message: c.msg}, true
		}
	}
	if looksLikeHighEntropySecret(text) {
		return Reason{
			Gate:    GateSensitivity,
			Code:    "high_entropy_secret",
			Message: "candidate appears to contain a high-entropy secret-like value",
		}, true
	}
	return Reason{}, false
}

// looksLikeHighEntropySecret catches dense token-like blobs after secret keywords.
func looksLikeHighEntropySecret(text string) bool {
	lower := strings.ToLower(text)
	markers := []string{"secret", "token", "password", "api_key", "apikey", "credential"}
	hasMarker := false
	for _, m := range markers {
		if strings.Contains(lower, m) {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return false
	}
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == '"' || r == '\'' || r == '`' || r == ',' || r == ';'
	}) {
		tok = strings.Trim(tok, `:=`)
		if len(tok) < 24 || len(tok) > 256 {
			continue
		}
		if isHighEntropyToken(tok) {
			return true
		}
	}
	return false
}

func isHighEntropyToken(s string) bool {
	var letters, digits, other int
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		case r == '-' || r == '_' || r == '+' || r == '/' || r == '=' || r == '.':
			other++
		default:
			return false
		}
	}
	if letters == 0 || digits == 0 {
		return false
	}
	// Dense mixed alphanumeric without spaces is suspicious near secret markers.
	return letters+digits+other == len([]rune(s)) && float64(digits)/float64(len(s)) >= 0.15
}
