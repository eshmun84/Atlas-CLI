package memorypersist

import (
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/memorypersist/policy"
)

// buildProvenance constructs bounded SourceKind / SourceRef for persistence.
// Absolute Home paths and secrets are refused earlier by policy; this still
// sanitizes residual separators. SourceRef is kept stable across equivalent
// candidates so durable hashing / dedupe remain deterministic (timestamps live
// on the observation row, not in hash inputs).
func buildProvenance(c Candidate) (sourceKind, sourceRef string) {
	origin := string(c.Origin)
	if origin == "" {
		origin = string(policy.OriginAtlasOperation)
	}
	sourceKind = "persist:" + sanitizeToken(origin, 64)

	// Only explicitness=explicit is trusted evidence on the current propose path.
	// confirmed is reserved and never persisted in V1.
	explicit := "0"
	if c.Explicitness == policy.ExplicitnessExplicit {
		explicit = "1"
	}

	parts := []string{
		"cap=" + CapabilitySlice,
		"explicit=" + explicit,
	}
	if ev := sanitizeToken(c.SourceEvent, 96); ev != "" {
		parts = append(parts, "event="+ev)
	}
	if p := sanitizeToken(c.Provenance, 160); p != "" {
		parts = append(parts, "ref="+p)
	}
	sourceRef = strings.Join(parts, ";")
	if len(sourceRef) > 512 {
		sourceRef = sourceRef[:512]
	}
	return sourceKind, sourceRef
}

func sanitizeToken(raw string, max int) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, ";", ",")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, `\`, "_")
	s = strings.ReplaceAll(s, "..", "_")
	if strings.HasPrefix(s, "/") || strings.Contains(strings.ToLower(s), "atlas_home") {
		return ""
	}
	if max > 0 && len(s) > max {
		s = s[:max]
	}
	return s
}

func toPolicyCandidate(c Candidate) policy.Candidate {
	return policy.Candidate{
		Type:         c.Type,
		Title:        c.Title,
		Content:      c.Content,
		Scope:        c.Scope,
		TopicKey:     c.TopicKey,
		Provenance:   c.Provenance,
		Confidence:   c.Confidence,
		Explicitness: c.Explicitness,
		Origin:       c.Origin,
		SourceEvent:  c.SourceEvent,
	}
}
