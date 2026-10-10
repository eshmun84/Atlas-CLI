package memory

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	maxTitleLen   = 512
	maxContentLen = 64 * 1024
	maxTopicLen   = 256
	maxSourceLen  = 512
	maxProjectLen = 128
)

var validTypes = map[ObservationType]struct{}{
	TypeDecision:     {},
	TypeArchitecture: {},
	TypeConstraint:   {},
	TypeDiscovery:    {},
	TypeBugfix:       {},
	TypePattern:      {},
	TypeConfig:       {},
	TypePreference:   {},
	TypeFact:         {},
}

var validScopes = map[Scope]struct{}{
	ScopeProject: {},
}

// ValidateProjectID checks a canonical project identity token.
func ValidateProjectID(projectID string) error {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return fmt.Errorf("memory: project id is required")
	}
	if len(id) > maxProjectLen {
		return fmt.Errorf("memory: project id too long")
	}
	if strings.Contains(id, "/") || strings.Contains(id, `\`) || strings.Contains(id, "..") {
		return fmt.Errorf("memory: project id must not contain path elements")
	}
	for _, r := range id {
		if r < 32 || r == 127 {
			return fmt.Errorf("memory: project id contains control characters")
		}
	}
	return nil
}

// NormalizeTopicKey trims and validates an optional topic key.
// Empty input returns ("", nil). Topic keys are not filesystem paths.
func NormalizeTopicKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", nil
	}
	if len(key) > maxTopicLen {
		return "", fmt.Errorf("memory: topic key too long")
	}
	if strings.Contains(key, `\`) || strings.Contains(key, "..") {
		return "", fmt.Errorf("memory: topic key invalid")
	}
	for _, r := range key {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("memory: topic key contains control characters")
		}
		if unicode.IsSpace(r) && r != ' ' {
			return "", fmt.Errorf("memory: topic key contains control whitespace")
		}
	}
	// Collapse internal runs of spaces for determinism.
	parts := strings.Fields(key)
	key = strings.Join(parts, " ")
	if key == "" {
		return "", fmt.Errorf("memory: topic key empty after normalization")
	}
	return key, nil
}

// ValidateObservation checks durable fields before persistence.
func ValidateObservation(o Observation) error {
	if err := ValidateProjectID(o.ProjectID); err != nil {
		return err
	}
	if _, ok := validTypes[o.Type]; !ok {
		return fmt.Errorf("memory: invalid observation type %q", o.Type)
	}
	scope := o.Scope
	if scope == "" {
		scope = ScopeProject
	}
	if _, ok := validScopes[scope]; !ok {
		return fmt.Errorf("memory: invalid observation scope %q", o.Scope)
	}
	title := strings.TrimSpace(o.Title)
	if title == "" {
		return fmt.Errorf("memory: title is required")
	}
	if len(title) > maxTitleLen {
		return fmt.Errorf("memory: title too long")
	}
	content := strings.TrimSpace(o.Content)
	if content == "" {
		return fmt.Errorf("memory: content is required")
	}
	if len(content) > maxContentLen {
		return fmt.Errorf("memory: content too long")
	}
	if _, err := NormalizeTopicKey(o.TopicKey); err != nil {
		return err
	}
	if len(strings.TrimSpace(o.SourceKind)) > maxSourceLen {
		return fmt.Errorf("memory: source kind too long")
	}
	if len(strings.TrimSpace(o.SourceRef)) > maxSourceLen {
		return fmt.Errorf("memory: source ref too long")
	}
	if sid := strings.TrimSpace(o.SessionID); sid != "" {
		if strings.Contains(sid, "/") || strings.Contains(sid, "..") {
			return fmt.Errorf("memory: session id invalid")
		}
	}
	return nil
}
