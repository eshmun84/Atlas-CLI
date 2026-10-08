package codegraph

import (
	"encoding/json"
	"fmt"
	"strings"
)

// versionPayload tolerates JSON version output if CodeGraph emits it for --version.
type versionPayload struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

func decodeVersion(stdout []byte) (string, error) {
	text := strings.TrimSpace(string(stdout))
	if text == "" {
		return "", fmt.Errorf("%w: empty version stdout", ErrInvalidJSON)
	}
	if strings.HasPrefix(text, "{") {
		var payload versionPayload
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}
		if strings.TrimSpace(payload.Version) == "" {
			return "", fmt.Errorf("%w: missing version field", ErrInvalidJSON)
		}
		return ParseVersion(payload.Version)
	}
	return ParseVersion(text)
}

func stderrMessage(stderr []byte) string {
	msg := strings.TrimSpace(string(stderr))
	if len(msg) > 240 {
		return msg[:240] + "…"
	}
	return msg
}
