package capability

import (
	"strings"
)

func taskLower(ev TaskEvidence) string {
	return strings.ToLower(strings.TrimSpace(ev.TaskText))
}

func phaseLower(ev TaskEvidence) string {
	return strings.ToLower(strings.TrimSpace(ev.Phase))
}

func classificationLower(ev TaskEvidence) string {
	return strings.ToLower(strings.TrimSpace(ev.Classification))
}

// trivialTask reports tasks that should not pull Memory or specialized skills.
func trivialTask(ev TaskEvidence) bool {
	t := taskLower(ev)
	if t == "" {
		return true
	}
	if containsAny(t, "--version", "atlas --version", "atlas version") {
		return true
	}
	if containsAny(t, "typo", "fix typo", "spelling", "whitespace", "formatting only", "pure formatting") {
		return true
	}
	// Very short self-contained edits.
	if len([]rune(t)) < 12 && !containsAny(t, "continue", "resume", "auth", "security", "test", "implement") {
		return true
	}
	return false
}

func wantsDocumentation(ev TaskEvidence) bool {
	t := taskLower(ev)
	return containsAny(t, "readme", "documentation", "docs", "write docs", "update doc", "explain ", "changelog")
}

func wantsTesting(ev TaskEvidence) bool {
	t := taskLower(ev)
	p := phaseLower(ev)
	return containsAny(t, "test", "testing", "coverage", "unit test", "integration test", "failing test", "verify") ||
		p == "verify" || p == "implement"
}

func wantsDebugging(ev TaskEvidence) bool {
	t := taskLower(ev)
	return containsAny(t, "debug", "race condition", "failing", "broken", "stack trace", "panic", "troubleshoot", "fix bug", "fix the")
}

func wantsSecurity(ev TaskEvidence) bool {
	t := taskLower(ev)
	return containsAny(t,
		"auth", "authentication", "authorization", "credential", "credentials", "secret", "secrets",
		"permission", "permissions", "cryptograph", "crypto", "session", "security", "trust boundary",
		"vulnerability", "oauth", "jwt", "password",
	)
}

func wantsCodeReview(ev TaskEvidence) bool {
	t := taskLower(ev)
	p := phaseLower(ev)
	return containsAny(t, "review", "pull request", "code review", "diff") || p == "verify"
}

func wantsImplementation(ev TaskEvidence) bool {
	if isExcluded(ev.ExplicitExcludes, CapImplementation) {
		return false
	}
	t := taskLower(ev)
	p := phaseLower(ev)
	c := classificationLower(ev)
	if c == "direct" || containsAny(t, "only inspect", "read-only", "explain architecture", "explain existing") {
		return false
	}
	if wantsDocumentation(ev) && !containsAny(t, "implement", "add feature", "build ", "create endpoint") {
		// Docs-only unless explicit implement language.
		if !containsAny(t, "implement", "fix ", "refactor", "change behavior") {
			return false
		}
	}
	return containsAny(t, "implement", "add feature", "build ", "create ", "change behavior", "refactor", "fix ", "wire ", "mutate") ||
		p == "implement" || c == "governed-sdd"
}

func wantsArchitecture(ev TaskEvidence) bool {
	t := taskLower(ev)
	return containsAny(t, "architecture", "architect", "system design", "explain existing architecture", "how is it structured", "subsystem")
}

func wantsResearch(ev TaskEvidence) bool {
	t := taskLower(ev)
	p := phaseLower(ev)
	return containsAny(t, "research", "explore", "investigate", "survey") ||
		p == "explore" || p == "research"
}

func wantsSpecAuthoring(ev TaskEvidence) bool {
	t := taskLower(ev)
	p := phaseLower(ev)
	return containsAny(t, "proposal", "propose", "write spec", "author spec", "openspec change") ||
		p == "propose" || p == "update"
}

func wantsCodeIntelligence(ev TaskEvidence) bool {
	if isExcluded(ev.ExplicitExcludes, CapCodeIntelligence) {
		return false
	}
	t := taskLower(ev)
	// Negative: trivial docs / labels.
	if wantsDocumentation(ev) && !containsAny(t, "who calls", "caller", "impact", "dependency") {
		return false
	}
	if containsAny(t, "readme", "title", "label", "config string", "typo", "formatting") {
		return false
	}
	return containsAny(t,
		"who calls", "who uses", "caller", "callee", "callers of",
		"impact analysis", "what will this change impact", "dependency", "dependencies",
		"implementation path", "find implementation", "cross-file", "structural",
		"legacy subsystem", "codegraph", "symbol relationship",
	)
}

// memoryRelevant uses layered rules: workflow/phase first, then lexical cues, then negatives.
func memoryRelevant(ev TaskEvidence) (bool, string) {
	if isExcluded(ev.ExplicitExcludes, CapMemoryRetrieval) {
		return false, "excluded"
	}
	if trivialTask(ev) {
		return false, "trivial_task"
	}

	p := phaseLower(ev)
	c := classificationLower(ev)
	t := taskLower(ev)

	if ev.ActiveChange != "" && (p == "implement" || p == "verify" || p == "update") {
		return true, "active_change_" + p
	}
	if p == "implement" || p == "verify" || p == "propose" || p == "archive" {
		return true, "sdd_phase_" + p
	}
	if c == "governed-sdd" && containsAny(t, "continue", "resume", "prior", "previous") {
		return true, "governed_continue"
	}
	if wantsArchitecture(ev) || wantsResearch(ev) {
		return true, "architecture_or_research"
	}
	if containsAny(t,
		"continue", "resume", "previous decision", "prior decision", "established convention",
		"why was", "why did we", "project policy", "ongoing change", "previous implementation",
		"project context", "recall", "remember",
	) {
		return true, "lexical_durable_context"
	}
	return false, ""
}

func contextEconomyUseful(ev TaskEvidence) bool {
	if trivialTask(ev) {
		return false
	}
	t := taskLower(ev)
	p := phaseLower(ev)
	return wantsArchitecture(ev) || wantsResearch(ev) || wantsSpecAuthoring(ev) ||
		p == "explore" || p == "research" || p == "propose" ||
		containsAny(t, "orient", "project overview", "where things live", "codebase structure")
}

func openspecEvidenceUseful(ev TaskEvidence) bool {
	p := phaseLower(ev)
	if p != "" && p != "none" {
		return true
	}
	return strings.TrimSpace(ev.ActiveChange) != "" ||
		strings.EqualFold(strings.TrimSpace(ev.Workflow), "sdd")
}
