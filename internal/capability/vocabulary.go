package capability

import "strings"

// Closed V1 capability vocabulary.
const (
	CapTesting          = "testing"
	CapCodeReview       = "code-review"
	CapSecurityReview   = "security-review"
	CapDocumentation    = "documentation"
	CapMemoryRetrieval  = "memory-retrieval"
	CapImplementation   = "implementation"
	CapSpecAuthoring    = "spec-authoring"
	CapArchitecture     = "architecture"
	CapDebugging        = "debugging"
	CapCodeIntelligence = "code-intelligence"
	CapResearch         = "research"
)

// AllCapabilities is the closed V1 set (stable order for tests).
func AllCapabilities() []string {
	return []string{
		CapTesting,
		CapCodeReview,
		CapSecurityReview,
		CapDocumentation,
		CapMemoryRetrieval,
		CapImplementation,
		CapSpecAuthoring,
		CapArchitecture,
		CapDebugging,
		CapCodeIntelligence,
		CapResearch,
	}
}

// skillForCapability maps capability IDs to Skills v1 pin IDs when a specialized
// skill exists. Empty means agent/core or context-only capability.
func skillForCapability(capID string) string {
	switch strings.TrimSpace(capID) {
	case CapTesting:
		return "testing"
	case CapCodeReview:
		return "code-review"
	case CapSecurityReview:
		return "security-review"
	case CapDocumentation:
		return "documentation"
	case CapMemoryRetrieval:
		return "memory"
	default:
		return ""
	}
}

// capabilityForSkill maps a skill pin ID back to its primary capability.
func capabilityForSkill(skillID string) string {
	switch strings.TrimSpace(skillID) {
	case "testing":
		return CapTesting
	case "code-review":
		return CapCodeReview
	case "security-review":
		return CapSecurityReview
	case "documentation":
		return CapDocumentation
	case "memory":
		return CapMemoryRetrieval
	default:
		return ""
	}
}

func isKnownCapability(id string) bool {
	id = strings.TrimSpace(strings.ToLower(id))
	for _, c := range AllCapabilities() {
		if c == id {
			return true
		}
	}
	// Also accept skill IDs as capability aliases.
	if capabilityForSkill(id) != "" {
		return true
	}
	return false
}

func normalizeCapabilityID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.ReplaceAll(raw, "_", "-")
	if cap := capabilityForSkill(raw); cap != "" {
		return cap
	}
	switch raw {
	case "docs", "doc", "readme":
		return CapDocumentation
	case "test", "tests", "verify":
		return CapTesting
	case "review", "code review":
		return CapCodeReview
	case "security", "auth-review", "security review":
		return CapSecurityReview
	case "memory", "recall":
		return CapMemoryRetrieval
	case "implement", "impl", "code":
		return CapImplementation
	case "spec", "propose", "openspec":
		return CapSpecAuthoring
	case "architect", "design":
		return CapArchitecture
	case "debug", "fix", "troubleshoot":
		return CapDebugging
	case "codegraph", "code-graph", "code intel", "codeintelligence":
		return CapCodeIntelligence
	case "explore", "investigate":
		return CapResearch
	default:
		return raw
	}
}
