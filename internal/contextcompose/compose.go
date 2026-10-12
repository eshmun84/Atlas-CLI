package contextcompose

import (
	"sort"
	"strings"
)

// knownSourcePriority is the explicit stable source order for V1 packing.
// Unknown sources sort after these, alphabetically among themselves.
var knownSourcePriority = []string{
	SourceMemory,
	SourceContextEconomy,
	SourceCodeGraph,
}

// Compose produces a bounded, deterministic context pack from supplied candidates.
// It never retrieves, persists, or inspects the filesystem.
func Compose(req Request) Response {
	limits := defaultLimits()
	budget, maxItems, caps, early, ok := validateRequest(req)
	if !ok {
		// Do not echo rejected, oversized, or invalid UTF-8 task/query content.
		return early
	}

	// Validated above: within MaxTaskRunes / MaxQueryRunes and valid UTF-8.
	task := strings.TrimSpace(req.Task)
	query := strings.TrimSpace(req.Query)

	requested := normalizeRequestedSources(req.Sources, req.Candidates, req.SourceStatuses)
	requestedSet := toSet(requested)
	explicitSources := len(req.Sources) > 0

	var omissions []Omission
	var valid []Candidate
	for _, c := range req.Candidates {
		n, reason, ok := validateCandidate(c)
		if !ok {
			omissions = append(omissions, omissionOf(c.Source, c.ID, reason))
			continue
		}
		if explicitSources && !requestedSet[n.Source] {
			omissions = append(omissions, omissionOf(n.Source, n.ID, "source_not_requested"))
			continue
		}
		valid = append(valid, n)
	}

	// Deterministic order before dedupe so the highest score wins.
	sortCandidates(valid)

	deduped, dupOmissions := dedupeCandidates(valid)
	omissions = append(omissions, dupOmissions...)

	// CE path count cap (output consideration set).
	capped, ceOmissions := capCEPaths(deduped)
	omissions = append(omissions, ceOmissions...)

	items, used, packOmissions := packItems(capped, budget, maxItems, caps)
	omissions = append(omissions, packOmissions...)

	if used > budget {
		// Invariant violation — fail closed.
		return Response{
			SchemaVersion: SchemaVersion,
			Status:        StatusInvalidRequest,
			Task:          task,
			Query:         query,
			BudgetChars:   budget,
			UsedChars:     0,
			Count:         0,
			Limits:        limits,
			Error: &ErrorBody{
				Code:    StatusInvalidRequest,
				Message: "internal budget invariant violated",
			},
		}
	}

	sources := buildSourceStatuses(requested, capped, items, req.SourceStatuses, omissions)
	status := resolveStatus(items, sources, omissions)
	omissions = boundOmissions(sortOmissions(omissions))
	sources = boundSources(sources)

	resp := Response{
		SchemaVersion: SchemaVersion,
		Status:        status,
		Task:          task,
		Query:         query,
		BudgetChars:   budget,
		UsedChars:     used,
		Count:         len(items),
		Items:         items,
		Sources:       sources,
		Omissions:     omitEmpty(omissions),
		Limits:        limits,
	}
	return resp
}

func normalizeRequestedSources(sources []string, cands []Candidate, statuses []SourceStatus) []string {
	if len(sources) > 0 {
		out := make([]string, 0, len(sources))
		seen := map[string]bool{}
		for _, s := range sources {
			s = strings.TrimSpace(s)
			if s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
		return boundSourceNames(orderSources(out), MaxSources)
	}
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		// Only advertise sources that are already safe and bounded.
		s := sanitizeOmissionField(raw, MaxSourceRunes)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, c := range cands {
		add(c.Source)
	}
	for _, st := range statuses {
		add(st.Source)
	}
	// Re-order known sources first for stable reporting, then enforce bound.
	return boundSourceNames(orderSources(out), MaxResponseSources)
}

func boundSourceNames(in []string, max int) []string {
	if max <= 0 || len(in) <= max {
		return in
	}
	return in[:max]
}

func boundSources(in []SourceStatus) []SourceStatus {
	if len(in) <= MaxResponseSources {
		return in
	}
	return in[:MaxResponseSources]
}

func boundOmissions(in []Omission) []Omission {
	if len(in) <= MaxOmissions {
		return in
	}
	return in[:MaxOmissions]
}

func toSet(ss []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func sourcePriority(source string) int {
	for i, s := range knownSourcePriority {
		if s == source {
			return i
		}
	}
	return len(knownSourcePriority) + 1
}

func sortCandidates(cands []Candidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		return candidateLess(cands[i], cands[j])
	})
}

// candidateLess defines a total order so identical multisets sort identically
// regardless of input permutation. Score is descending; all other fields
// ascending. After source/id/score/path ties, kind/title/excerpt/ref/provenance
// break remaining ties — never input order.
func candidateLess(a, b Candidate) bool {
	pa, pb := sourcePriority(a.Source), sourcePriority(b.Source)
	if pa != pb {
		return pa < pb
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	if a.Excerpt != b.Excerpt {
		return a.Excerpt < b.Excerpt
	}
	if a.Ref != b.Ref {
		return a.Ref < b.Ref
	}
	return a.Provenance < b.Provenance
}

func orderSources(sources []string) []string {
	cp := append([]string(nil), sources...)
	sort.SliceStable(cp, func(i, j int) bool {
		pi, pj := sourcePriority(cp[i]), sourcePriority(cp[j])
		if pi != pj {
			return pi < pj
		}
		return cp[i] < cp[j]
	})
	return cp
}

func dedupeCandidates(cands []Candidate) ([]Candidate, []Omission) {
	seen := map[string]bool{}
	out := make([]Candidate, 0, len(cands))
	var omissions []Omission
	for _, c := range cands {
		key := c.Source + "\x00" + c.ID
		if seen[key] {
			omissions = append(omissions, omissionOf(c.Source, c.ID, "duplicate_identity"))
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out, omissions
}

func capCEPaths(cands []Candidate) ([]Candidate, []Omission) {
	out := make([]Candidate, 0, len(cands))
	cePaths := 0
	var omissions []Omission
	for _, c := range cands {
		if c.Source == SourceContextEconomy && c.Path != "" {
			if cePaths >= MaxCEPaths {
				omissions = append(omissions, omissionOf(c.Source, c.ID, "ce_path_limit"))
				continue
			}
			cePaths++
		}
		out = append(out, c)
	}
	return out, omissions
}

func packItems(cands []Candidate, budget, maxItems int, caps map[string]int) ([]Item, int, []Omission) {
	items := make([]Item, 0, minInt(len(cands), maxItems))
	used := 0
	sourceUsed := map[string]int{}
	var omissions []Omission
	budgetOmitted := 0

	for _, c := range cands {
		if len(items) >= maxItems {
			omissions = append(omissions, omissionOf(c.Source, c.ID, "max_items"))
			continue
		}
		cap := sourceCapFor(c.Source, budget, caps)
		remainingSource := cap - sourceUsed[c.Source]
		remainingGlobal := budget - used
		it, ok := fitItem(c, remainingGlobal, remainingSource)
		if !ok {
			budgetOmitted++
			omissions = append(omissions, omissionOf(c.Source, c.ID, "budget"))
			continue
		}
		cost := itemCost(it)
		if used+cost > budget || sourceUsed[c.Source]+cost > cap {
			budgetOmitted++
			omissions = append(omissions, omissionOf(c.Source, c.ID, "budget"))
			continue
		}
		items = append(items, it)
		used += cost
		sourceUsed[c.Source] += cost
	}

	_ = budgetOmitted
	return items, used, omissions
}

func buildSourceStatuses(
	requested []string,
	considered []Candidate,
	items []Item,
	caller []SourceStatus,
	omissions []Omission,
) []SourceStatus {
	// Duplicates are rejected in validateRequest; one entry per source here.
	callerBy := map[string]SourceStatus{}
	for _, st := range caller {
		src := strings.TrimSpace(st.Source)
		if src == "" {
			continue
		}
		msg := st.Message
		if runeLen(msg) > MaxSourceMessageRunes {
			msg, _ = truncateRunes(msg, MaxSourceMessageRunes)
		}
		callerBy[src] = SourceStatus{
			Source:  src,
			Status:  strings.TrimSpace(st.Status),
			Message: msg,
		}
	}

	candCount := map[string]int{}
	for _, c := range considered {
		candCount[c.Source]++
	}
	itemCount := map[string]int{}
	for _, it := range items {
		itemCount[it.Source]++
	}
	unsafeCount := map[string]int{}
	for _, o := range omissions {
		switch o.Reason {
		case "invalid_utf8", "missing_source", "missing_identity", "unsafe_path", "unsafe_ref",
			"source_too_long", "identity_too_long", "path_too_long", "ref_too_long", "kind_too_long":
			unsafeCount[o.Source]++
		}
	}

	sources := orderSources(requested)
	out := make([]SourceStatus, 0, len(sources))
	for _, src := range sources {
		if st, ok := callerBy[src]; ok && st.Status != "" {
			// Never upgrade caller status; pass through honestly.
			out = append(out, st)
			continue
		}
		switch {
		case itemCount[src] > 0:
			out = append(out, SourceStatus{Source: src, Status: SourceStatusSupplied, Message: "candidates packed from request"})
		case candCount[src] > 0:
			out = append(out, SourceStatus{Source: src, Status: SourceStatusOmitted, Message: "candidates present but not packed"})
		case unsafeCount[src] > 0:
			out = append(out, SourceStatus{Source: src, Status: SourceStatusOmitted, Message: "candidates rejected as unsafe or invalid"})
		default:
			out = append(out, SourceStatus{Source: src, Status: SourceStatusNoCandidates})
		}
	}
	return out
}

func resolveStatus(items []Item, sources []SourceStatus, omissions []Omission) string {
	hasFailure := false
	for _, st := range sources {
		switch st.Status {
		case SourceStatusFailed, SourceStatusUnavailable:
			hasFailure = true
		}
	}
	for _, o := range omissions {
		switch o.Reason {
		case "invalid_utf8", "missing_source", "missing_identity", "unsafe_path", "unsafe_ref",
			"source_too_long", "identity_too_long", "path_too_long", "ref_too_long", "kind_too_long":
			hasFailure = true
		}
	}

	switch {
	case len(items) == 0 && !hasFailure:
		return StatusEmpty
	case len(items) == 0 && hasFailure:
		return StatusPartial
	case len(items) > 0 && hasFailure:
		return StatusPartial
	default:
		return StatusSuccess
	}
}

func sortOmissions(in []Omission) []Omission {
	if len(in) == 0 {
		return in
	}
	out := append([]Omission(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Identity != out[j].Identity {
			return out[i].Identity < out[j].Identity
		}
		if out[i].Reason != out[j].Reason {
			return out[i].Reason < out[j].Reason
		}
		return out[i].Count < out[j].Count
	})
	return out
}

func omitEmpty(in []Omission) []Omission {
	if len(in) == 0 {
		return nil
	}
	return in
}
