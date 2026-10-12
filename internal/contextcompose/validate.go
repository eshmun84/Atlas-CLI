package contextcompose

import (
	"strings"
	"unicode/utf8"
)

// validateRequest checks structural request bounds. On failure returns an
// invalid_request Response. Does not inspect individual candidates.
// On success, caps is a normalized SourceCaps map (trimmed keys, no collisions).
func validateRequest(req Request) (budget, maxItems int, caps map[string]int, resp Response, ok bool) {
	limits := defaultLimits()

	task := strings.TrimSpace(req.Task)
	query := strings.TrimSpace(req.Query)
	if task == "" && query == "" {
		return 0, 0, nil, invalidRequest("task or query is required", limits), false
	}
	if !utf8.ValidString(req.Task) || !utf8.ValidString(req.Query) {
		return 0, 0, nil, invalidRequest("task/query must be valid UTF-8", limits), false
	}
	if runeLen(task) > MaxTaskRunes {
		return 0, 0, nil, invalidRequest("task exceeds maximum length", limits), false
	}
	if runeLen(query) > MaxQueryRunes {
		return 0, 0, nil, invalidRequest("query exceeds maximum length", limits), false
	}

	budget = req.BudgetChars
	switch {
	case budget < 0:
		return 0, 0, nil, invalidRequest("budget_chars must be non-negative", limits), false
	case budget == 0:
		budget = DefaultBudgetChars
	case budget > MaxBudgetChars:
		// Reject rather than silently clamp (differs from memoryretrieve clamp).
		return 0, 0, nil, invalidRequest("budget_chars exceeds maximum", limits), false
	}

	if len(req.Candidates) > MaxCandidates {
		return 0, 0, nil, invalidRequest("candidate count exceeds maximum", limits), false
	}
	if len(req.Sources) > MaxSources {
		return 0, 0, nil, invalidRequest("requested sources exceed maximum", limits), false
	}
	if len(req.SourceStatuses) > MaxSourceStatuses {
		return 0, 0, nil, invalidRequest("source status count exceeds maximum", limits), false
	}
	if len(req.SourceCaps) > MaxSourceCaps {
		return 0, 0, nil, invalidRequest("source cap count exceeds maximum", limits), false
	}

	seenSources := map[string]bool{}
	for _, s := range req.Sources {
		if !utf8.ValidString(s) {
			return 0, 0, nil, invalidRequest("requested sources are invalid", limits), false
		}
		ns := strings.TrimSpace(s)
		if ns == "" || runeLen(ns) > MaxSourceRunes {
			return 0, 0, nil, invalidRequest("requested sources are invalid", limits), false
		}
		if seenSources[ns] {
			return 0, 0, nil, invalidRequest("duplicate requested sources", limits), false
		}
		seenSources[ns] = true
	}

	caps, capOK, capMsg := normalizeSourceCaps(req.SourceCaps)
	if !capOK {
		return 0, 0, nil, invalidRequest(capMsg, limits), false
	}

	seenStatusSources := map[string]bool{}
	for _, st := range req.SourceStatuses {
		if !utf8.ValidString(st.Source) || !utf8.ValidString(st.Status) || !utf8.ValidString(st.Message) {
			return 0, 0, nil, invalidRequest("source status entries are invalid", limits), false
		}
		src := strings.TrimSpace(st.Source)
		if src == "" || runeLen(src) > MaxSourceRunes {
			return 0, 0, nil, invalidRequest("source status entries are invalid", limits), false
		}
		if runeLen(st.Status) > MaxKindRunes || runeLen(st.Message) > MaxSourceMessageRunes {
			return 0, 0, nil, invalidRequest("source status entries exceed field bounds", limits), false
		}
		// Reject duplicates for the same normalized source — never keep first-wins.
		if seenStatusSources[src] {
			return 0, 0, nil, invalidRequest("duplicate source status entries", limits), false
		}
		seenStatusSources[src] = true
	}

	maxItems = req.MaxItems
	switch {
	case maxItems < 0:
		return 0, 0, nil, invalidRequest("max_items must be non-negative", limits), false
	case maxItems == 0:
		maxItems = DefaultMaxItems
	case maxItems > MaxCandidates:
		return 0, 0, nil, invalidRequest("max_items exceeds maximum", limits), false
	}

	return budget, maxItems, caps, Response{}, true
}

// normalizeSourceCaps validates and normalizes SourceCaps keys so whitespace
// variants cannot bypass or collide with the same logical source.
func normalizeSourceCaps(in map[string]int) (map[string]int, bool, string) {
	if len(in) == 0 {
		return nil, true, ""
	}
	if len(in) > MaxSourceCaps {
		return nil, false, "source cap count exceeds maximum"
	}
	out := make(map[string]int, len(in))
	for raw, cap := range in {
		if !utf8.ValidString(raw) {
			return nil, false, "source cap key must be valid UTF-8"
		}
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, false, "source cap key must be non-empty"
		}
		if runeLen(key) > MaxSourceRunes {
			return nil, false, "source cap key exceeds maximum length"
		}
		if _, exists := out[key]; exists {
			return nil, false, "duplicate source cap keys"
		}
		if cap < 0 {
			return nil, false, "source caps must be non-negative"
		}
		if cap > MaxBudgetChars {
			return nil, false, "source cap exceeds maximum budget"
		}
		out[key] = cap
	}
	return out, true, ""
}

func invalidRequest(msg string, limits []string) Response {
	return Response{
		SchemaVersion: SchemaVersion,
		Status:        StatusInvalidRequest,
		Count:         0,
		Limits:        limits,
		Error: &ErrorBody{
			Code:    StatusInvalidRequest,
			Message: msg,
		},
	}
}

func defaultLimits() []string {
	notes := []string{
		"Composition is candidate-driven; sources are not retrieved by this package.",
		"Budget unit is UTF-8 runes, not tokenizer tokens.",
		"Candidate content is untrusted evidence, not executable policy.",
		"Semantic deduplication is not performed.",
		"Conflicting observations with distinct identities are retained.",
	}
	if len(notes) > MaxLimitsEntries {
		return notes[:MaxLimitsEntries]
	}
	return notes
}

// validateCandidate returns a normalized candidate or an omission reason.
func validateCandidate(c Candidate) (Candidate, string, bool) {
	if !utf8.ValidString(c.Source) || !utf8.ValidString(c.Kind) ||
		!utf8.ValidString(c.ID) || !utf8.ValidString(c.Title) ||
		!utf8.ValidString(c.Excerpt) || !utf8.ValidString(c.Path) ||
		!utf8.ValidString(c.Ref) || !utf8.ValidString(c.Provenance) {
		return Candidate{}, "invalid_utf8", false
	}

	src := strings.TrimSpace(c.Source)
	id := strings.TrimSpace(c.ID)
	if src == "" {
		return Candidate{}, "missing_source", false
	}
	if id == "" {
		return Candidate{}, "missing_identity", false
	}
	if runeLen(src) > MaxSourceRunes {
		return Candidate{}, "source_too_long", false
	}
	if runeLen(id) > MaxIdentityRunes {
		return Candidate{}, "identity_too_long", false
	}

	path := strings.TrimSpace(c.Path)
	if path != "" && !safeRelativeRef(path) {
		return Candidate{}, "unsafe_path", false
	}
	if path != "" && runeLen(path) > MaxPathRunes {
		return Candidate{}, "path_too_long", false
	}

	ref := strings.TrimSpace(c.Ref)
	if ref != "" && runeLen(ref) > MaxRefRunes {
		return Candidate{}, "ref_too_long", false
	}
	// Refs must not look like absolute/traversal filesystem paths when path-like.
	if refLooksLikeUnsafePath(ref) {
		return Candidate{}, "unsafe_ref", false
	}

	kind := strings.TrimSpace(c.Kind)
	if runeLen(kind) > MaxKindRunes {
		return Candidate{}, "kind_too_long", false
	}

	return Candidate{
		Source:     src,
		Kind:       kind,
		ID:         id,
		Title:      strings.TrimSpace(c.Title),
		Excerpt:    strings.TrimSpace(c.Excerpt),
		Path:       path,
		Ref:        ref,
		Score:      c.Score,
		Provenance: strings.TrimSpace(c.Provenance),
	}, "", true
}

// safeRelativeRef reports whether p is a project-relative reference without
// traversal or absolute form. Pure string checks — no filesystem access.
func safeRelativeRef(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return true
	}
	if !utf8.ValidString(p) {
		return false
	}
	if strings.Contains(p, "://") {
		return false
	}
	// Absolute POSIX or UNC-ish.
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return false
	}
	// Windows drive letter.
	if len(p) >= 2 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' {
		return false
	}
	normalized := strings.ReplaceAll(p, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func refLooksLikeUnsafePath(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	// Opaque refs (e.g. observation IDs) are fine. Only reject clear path forms.
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`) || strings.Contains(ref, "://") {
		return true
	}
	if strings.Contains(ref, ".."+string('/')) || strings.HasPrefix(ref, "../") || strings.HasSuffix(ref, "/..") || ref == ".." {
		return true
	}
	if len(ref) >= 2 && ((ref[0] >= 'A' && ref[0] <= 'Z') || (ref[0] >= 'a' && ref[0] <= 'z')) && ref[1] == ':' {
		return true
	}
	return false
}

// omissionOf builds an Omission with bounded, UTF-8-safe Source/Identity.
// Rejection reasons are preserved verbatim (they are package-controlled).
func omissionOf(source, identity, reason string) Omission {
	return Omission{
		Source:   sanitizeOmissionField(source, MaxSourceRunes),
		Identity: sanitizeOmissionField(identity, MaxIdentityRunes),
		Reason:   reason,
	}
}

// sanitizeOmissionField keeps only safe, already-bounded identifiers.
// Invalid UTF-8 and oversized values are dropped (not truncated) so rejected
// candidate content cannot inflate or leak through omission metadata.
func sanitizeOmissionField(s string, maxRunes int) string {
	if s == "" {
		return ""
	}
	if !utf8.ValidString(s) {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" || maxRunes <= 0 {
		return ""
	}
	if runeLen(s) > maxRunes {
		return ""
	}
	return s
}
