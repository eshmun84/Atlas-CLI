package capability

import (
	"regexp"
	"strings"
)

var (
	// reUseSkill requires the word "skill" — these are specialized skill requirements.
	reUseSkill = regexp.MustCompile(`(?i)\b(?:use|load|apply)\s+(?:the\s+)?([a-z0-9_-]+(?:\s+review)?)\s+skill\b`)
	// reUseCapability matches "use X" without "skill" for closed capability IDs only.
	reUseCapability = regexp.MustCompile(`(?i)\b(?:use|load|apply)\s+(?:the\s+)?([a-z0-9_-]+(?:\s+review)?)\b`)
	reNoThing       = regexp.MustCompile(`(?i)\b(?:do\s+not|don't|dont|never|skip|without)\s+(?:use\s+|retrieve\s+|call\s+|run\s+)?([a-z0-9_-]+(?:\s+review)?|codegraph|code\s*graph|memory|code\s*intelligence)\b`)
)

// ParsedConstraints is the deterministic explicit-intent parse result.
type ParsedConstraints struct {
	Requires      []string // closed capability IDs
	SkillRequires []string // specialized skill IDs (may be unknown/unpinned)
	Excludes      []string
}

// ParseExplicitConstraints extracts deterministic require/exclude signals from task text.
func ParseExplicitConstraints(task string) ParsedConstraints {
	task = strings.TrimSpace(task)
	if task == "" {
		return ParsedConstraints{}
	}
	lower := strings.ToLower(task)
	var out ParsedConstraints

	// Operation bounds.
	if containsAny(lower, "only inspect", "read-only", "readonly", "do not implement", "don't implement", "dont implement", "no implementation") {
		out.Excludes = appendUnique(out.Excludes, CapImplementation)
	}

	// Memory / CodeGraph exclusions (common phrases).
	if containsAny(lower, "do not retrieve memory", "don't retrieve memory", "dont retrieve memory",
		"no memory", "without memory", "skip memory") {
		out.Excludes = appendUnique(out.Excludes, CapMemoryRetrieval)
		out.Excludes = appendUnique(out.Excludes, "memory")
	}
	if containsAny(lower, "do not use codegraph", "don't use codegraph", "dont use codegraph",
		"no codegraph", "without codegraph", "skip codegraph", "do not use code graph",
		"don't use code intelligence", "no code intelligence") {
		out.Excludes = appendUnique(out.Excludes, CapCodeIntelligence)
		out.Excludes = appendUnique(out.Excludes, "codegraph")
	}

	for _, m := range reNoThing.FindAllStringSubmatch(lower, -1) {
		if len(m) < 2 {
			continue
		}
		id := normalizeCapabilityID(strings.ReplaceAll(m[1], " ", "-"))
		if id == "" {
			continue
		}
		out.Excludes = appendUnique(out.Excludes, id)
		if id == CapCodeIntelligence {
			out.Excludes = appendUnique(out.Excludes, "codegraph")
		}
		if id == CapMemoryRetrieval {
			out.Excludes = appendUnique(out.Excludes, "memory")
		}
	}

	// Explicit skill requests ("use X skill") — specialized skill requirement.
	skillSpans := reUseSkill.FindAllStringSubmatchIndex(lower, -1)
	covered := make([][2]int, 0, len(skillSpans))
	for _, m := range reUseSkill.FindAllStringSubmatch(lower, -1) {
		if len(m) < 2 {
			continue
		}
		skillID := normalizeSkillRequestID(m[1])
		if skillID == "" {
			continue
		}
		out.SkillRequires = appendUnique(out.SkillRequires, skillID)
		// If the skill maps to a closed capability, also require that capability.
		if capID := capabilityForSkill(skillID); capID != "" {
			out.Requires = appendUnique(out.Requires, capID)
		} else if isKnownCapability(skillID) {
			// e.g. "use testing skill" where testing is both skill and capability id
			out.Requires = appendUnique(out.Requires, normalizeCapabilityID(skillID))
		}
		// Unknown skill IDs are NOT added to the capability vocabulary.
	}
	for _, idx := range skillSpans {
		if len(idx) >= 2 {
			covered = append(covered, [2]int{idx[0], idx[1]})
		}
	}

	// "use X" without skill word — only closed capabilities (no vocab expansion).
	for _, m := range reUseCapability.FindAllStringSubmatchIndex(lower, -1) {
		if len(m) < 4 {
			continue
		}
		start, end := m[0], m[1]
		if spanOverlaps(covered, start, end) {
			continue
		}
		raw := lower[m[2]:m[3]]
		id := normalizeCapabilityID(strings.ReplaceAll(raw, " ", "-"))
		if !isKnownCapability(id) {
			continue
		}
		out.Requires = appendUnique(out.Requires, id)
	}

	return out
}

func normalizeSkillRequestID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "_", "-")
	raw = strings.ReplaceAll(raw, " ", "-")
	// Known skill aliases.
	switch raw {
	case "security", "security-review", "auth-review":
		return "security-review"
	case "code-review", "review":
		return "code-review"
	case "docs", "doc", "documentation":
		return "documentation"
	case "test", "tests", "testing":
		return "testing"
	case "memory", "recall":
		return "memory"
	default:
		return raw
	}
}

func spanOverlaps(spans [][2]int, start, end int) bool {
	for _, s := range spans {
		if start < s[1] && end > s[0] {
			return true
		}
	}
	return false
}

func mergeConstraints(ev *TaskEvidence) {
	parsed := ParseExplicitConstraints(ev.TaskText)

	if len(ev.ExplicitRequires) == 0 {
		ev.ExplicitRequires = append([]string{}, parsed.Requires...)
	} else {
		for i, r := range ev.ExplicitRequires {
			ev.ExplicitRequires[i] = normalizeCapabilityID(r)
		}
		for _, r := range parsed.Requires {
			ev.ExplicitRequires = appendUnique(ev.ExplicitRequires, normalizeCapabilityID(r))
		}
	}

	if len(ev.ExplicitSkillRequires) == 0 {
		ev.ExplicitSkillRequires = append([]string{}, parsed.SkillRequires...)
	} else {
		for i, s := range ev.ExplicitSkillRequires {
			ev.ExplicitSkillRequires[i] = normalizeSkillRequestID(s)
		}
		for _, s := range parsed.SkillRequires {
			ev.ExplicitSkillRequires = appendUnique(ev.ExplicitSkillRequires, normalizeSkillRequestID(s))
		}
	}

	if len(ev.ExplicitExcludes) == 0 {
		ev.ExplicitExcludes = append([]string{}, parsed.Excludes...)
	} else {
		for i, e := range ev.ExplicitExcludes {
			ev.ExplicitExcludes[i] = normalizeCapabilityID(e)
		}
		for _, e := range parsed.Excludes {
			ev.ExplicitExcludes = appendUnique(ev.ExplicitExcludes, normalizeCapabilityID(e))
		}
	}
}

func isExcluded(excludes []string, id string) bool {
	id = normalizeCapabilityID(id)
	skill := skillForCapability(id)
	for _, e := range excludes {
		e = normalizeCapabilityID(e)
		if e == id || (skill != "" && e == skill) || e == capabilityForSkill(id) {
			return true
		}
		// Aliases for retrieval sources.
		if id == CapMemoryRetrieval && (e == "memory" || e == RetrievalMemory) {
			return true
		}
		if id == CapCodeIntelligence && (e == "codegraph" || e == RetrievalCodeGraph || e == "code-graph") {
			return true
		}
		// Skill id exclusion.
		if e == normalizeSkillRequestID(id) {
			return true
		}
	}
	return false
}

func containsAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

func appendUnique(list []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
