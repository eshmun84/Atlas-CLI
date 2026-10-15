package capability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// DiscoverPlan builds a deterministic CapabilityPlan from bounded evidence.
// It never queries Memory/CodeGraph/MCP and never mutates the filesystem.
func DiscoverPlan(ev TaskEvidence) CapabilityPlan {
	task := strings.TrimSpace(ev.TaskText)
	if task == "" {
		return CapabilityPlan{
			SchemaVersion: SchemaVersion,
			Status:        StatusInvalidRequest,
			Error: &ErrorBody{
				Code:    StatusInvalidRequest,
				Message: "task text is required",
			},
		}
	}

	mergeConstraints(&ev)

	caps := inferCapabilities(ev)
	ctxNeeds := inferContextNeeds(ev, caps)
	actions := inferRetrievalActions(ev, ctxNeeds)

	// Apply exclusions to capabilities / context / actions.
	caps, ctxNeeds, actions = applyExclusions(ev.ExplicitExcludes, caps, ctxNeeds, actions)

	selected, skillNotes, skillErr := resolveTaskSkills(ev, caps)
	notes := append([]string{}, skillNotes...)
	if skillErr != nil {
		return CapabilityPlan{
			SchemaVersion: SchemaVersion,
			Status:        StatusInvalidRequest,
			Task:          task,
			Error: &ErrorBody{
				Code:    StatusInvalidRequest,
				Message: skillErr.Error(),
			},
		}
	}

	projected := make([]string, 0, len(ev.EnabledPins))
	for _, p := range ev.EnabledPins {
		projected = append(projected, p.ID+"@"+p.Version)
	}
	sort.Strings(projected)

	requiredSkills := filterExcludedSkills(ev.ExplicitSkillRequires, ev.ExplicitExcludes)
	sort.Strings(requiredSkills)

	plan := CapabilityPlan{
		SchemaVersion:    SchemaVersion,
		Status:           StatusSuccess,
		Task:             task,
		Capabilities:     caps,
		ContextNeeds:     ctxNeeds,
		SelectedSkills:   selected,
		RequiredSkills:   requiredSkills,
		RetrievalActions: actions,
		Exclusions:       append([]string{}, ev.ExplicitExcludes...),
		Notes:            notes,
		ProjectedPins:    projected,
	}

	// Explicit specialized skill requests + required mapped capabilities → blockers.
	blockers := requiredSkillBlockers(caps, selected, ev.EnabledPins)
	blockers = append(blockers, explicitSkillBlockers(requiredSkills, selected, ev.EnabledPins, ev.Catalog)...)
	sort.Strings(blockers)
	if len(blockers) > 0 {
		plan.Blockers = blockers
		plan.Status = StatusBlocked
		plan.Notes = append(plan.Notes, "required skill unavailable; do not bypass governance")
	}

	// Optional/relevant missing specialized skill → note and continue.
	for _, c := range caps {
		if c.Strength == StrengthRequired {
			continue
		}
		skillID := skillForCapability(c.ID)
		if skillID == "" {
			continue
		}
		if !skillSelected(selected, skillID) && !pinPresent(ev.EnabledPins, skillID) {
			plan.Notes = append(plan.Notes, fmt.Sprintf("optional skill %q unavailable for capability %q; continuing with core/agent", skillID, c.ID))
		} else if !skillSelected(selected, skillID) && pinPresent(ev.EnabledPins, skillID) {
			// Pinned but not selected by resolve — still OK for relevant.
			plan.Notes = append(plan.Notes, fmt.Sprintf("capability %q has no task-active skill selection; core/agent remains usable", c.ID))
		}
	}

	return plan
}

func inferCapabilities(ev TaskEvidence) []CapabilityNeed {
	byID := map[string]*CapabilityNeed{}

	add := func(id, strength, reason string) {
		id = normalizeCapabilityID(id)
		if id == "" || isExcluded(ev.ExplicitExcludes, id) {
			return
		}
		if existing, ok := byID[id]; ok {
			existing.Reasons = appendUnique(existing.Reasons, reason)
			existing.Strength = stronger(existing.Strength, strength)
			return
		}
		byID[id] = &CapabilityNeed{ID: id, Strength: strength, Reasons: []string{reason}}
	}

	// Explicit capability requires outrank inference (closed vocabulary only).
	for _, r := range ev.ExplicitRequires {
		r = normalizeCapabilityID(r)
		if isExcluded(ev.ExplicitExcludes, r) {
			continue
		}
		if !isKnownCapability(r) {
			// Do not expand the capability vocabulary with unknown IDs.
			continue
		}
		add(r, StrengthRequired, "explicit_developer_require")
	}
	// Explicit skill requires that map to closed capabilities also elevate those caps.
	for _, sid := range ev.ExplicitSkillRequires {
		sid = normalizeSkillRequestID(sid)
		if isExcluded(ev.ExplicitExcludes, sid) {
			continue
		}
		if capID := capabilityForSkill(sid); capID != "" {
			add(capID, StrengthRequired, "explicit_skill_require")
		}
	}

	p := phaseLower(ev)

	// SDD phase mapping (PRIMARY when present).
	switch p {
	case "propose", "explore", "research":
		add(CapSpecAuthoring, StrengthRelevant, "sdd_phase_"+p)
		add(CapResearch, StrengthRelevant, "sdd_phase_"+p)
		add(CapDocumentation, StrengthRelevant, "sdd_phase_"+p)
	case "implement":
		add(CapImplementation, StrengthRelevant, "sdd_phase_implement")
		add(CapTesting, StrengthRelevant, "sdd_phase_implement")
		if ev.TestingRequired {
			add(CapTesting, StrengthRequired, "governance_testing_required")
		}
	case "verify":
		add(CapTesting, StrengthRelevant, "sdd_phase_verify")
		add(CapCodeReview, StrengthRelevant, "sdd_phase_verify")
		if ev.TestingRequired {
			add(CapTesting, StrengthRequired, "governance_testing_required")
		}
		if ev.ReviewRequired {
			add(CapCodeReview, StrengthRequired, "governance_review_required")
		}
	case "archive":
		add(CapDocumentation, StrengthRelevant, "sdd_phase_archive")
	}

	// Task-text / classification inference (relevant/optional only).
	if wantsDocumentation(ev) {
		add(CapDocumentation, StrengthRelevant, "task_documentation")
	}
	if wantsTesting(ev) {
		add(CapTesting, StrengthRelevant, "task_testing")
	}
	if wantsDebugging(ev) {
		add(CapDebugging, StrengthRelevant, "task_debugging")
		add(CapTesting, StrengthRelevant, "task_debugging")
	}
	if wantsSecurity(ev) {
		add(CapSecurityReview, StrengthRelevant, "task_security_sensitive")
	}
	if wantsCodeReview(ev) {
		add(CapCodeReview, StrengthRelevant, "task_code_review")
	}
	if wantsImplementation(ev) {
		add(CapImplementation, StrengthRelevant, "task_implementation")
		add(CapTesting, StrengthRelevant, "implementation_implies_testing")
	}
	if wantsArchitecture(ev) {
		add(CapArchitecture, StrengthRelevant, "task_architecture")
	}
	if wantsResearch(ev) {
		add(CapResearch, StrengthRelevant, "task_research")
	}
	if wantsSpecAuthoring(ev) {
		add(CapSpecAuthoring, StrengthRelevant, "task_spec_authoring")
	}
	if wantsCodeIntelligence(ev) {
		add(CapCodeIntelligence, StrengthRelevant, "task_structural_code_query")
	}
	if ok, reason := memoryRelevant(ev); ok {
		add(CapMemoryRetrieval, StrengthRelevant, "memory_"+reason)
	}

	// Technology is secondary — refine reasons only, never invent capabilities alone.
	if len(ev.Technologies) > 0 && (wantsDebugging(ev) || wantsImplementation(ev) || wantsTesting(ev)) {
		tech := strings.Join(ev.Technologies, ",")
		if n, ok := byID[CapTesting]; ok {
			n.Reasons = appendUnique(n.Reasons, "tech_evidence:"+tech)
		}
		if n, ok := byID[CapDebugging]; ok {
			n.Reasons = appendUnique(n.Reasons, "tech_evidence:"+tech)
		}
	}

	out := make([]CapabilityNeed, 0, len(byID))
	for _, n := range byID {
		out = append(out, *n)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Strength != out[j].Strength {
			return strengthRank(out[i].Strength) > strengthRank(out[j].Strength)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func inferContextNeeds(ev TaskEvidence, caps []CapabilityNeed) []ContextNeed {
	byID := map[string]*ContextNeed{}
	add := func(id, reason string) {
		if isExcluded(ev.ExplicitExcludes, id) {
			return
		}
		// Map capability exclusions onto context sources.
		if id == ContextMemory && isExcluded(ev.ExplicitExcludes, CapMemoryRetrieval) {
			return
		}
		if id == ContextCodeIntelligence && isExcluded(ev.ExplicitExcludes, CapCodeIntelligence) {
			return
		}
		if existing, ok := byID[id]; ok {
			existing.Reasons = appendUnique(existing.Reasons, reason)
			return
		}
		byID[id] = &ContextNeed{ID: id, Reasons: []string{reason}}
	}

	if ok, reason := memoryRelevant(ev); ok {
		add(ContextMemory, reason)
	}
	for _, c := range caps {
		if c.ID == CapMemoryRetrieval {
			add(ContextMemory, "capability_memory_retrieval")
		}
		if c.ID == CapCodeIntelligence {
			add(ContextCodeIntelligence, "capability_code_intelligence")
		}
	}
	if wantsCodeIntelligence(ev) {
		add(ContextCodeIntelligence, "structural_query")
	}
	if contextEconomyUseful(ev) {
		add(ContextEconomy, "project_orientation")
	}
	if openspecEvidenceUseful(ev) {
		add(ContextOpenSpec, "openspec_phase_or_change")
	}
	if len(ev.ExplicitFiles) > 0 {
		add(ContextProjectFiles, "explicit_files")
	}

	out := make([]ContextNeed, 0, len(byID))
	for _, n := range byID {
		out = append(out, *n)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func inferRetrievalActions(ev TaskEvidence, ctx []ContextNeed) []RetrievalAction {
	var out []RetrievalAction
	query := strings.TrimSpace(ev.TaskText)
	has := func(id string) bool {
		for _, c := range ctx {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	if has(ContextMemory) && !isExcluded(ev.ExplicitExcludes, CapMemoryRetrieval) {
		out = append(out, RetrievalAction{
			Source: RetrievalMemory,
			Reason: "durable_project_context_likely",
			Query:  query,
		})
	}
	if has(ContextEconomy) {
		out = append(out, RetrievalAction{
			Source: RetrievalCE,
			Reason: "project_orientation",
			Query:  query,
		})
	}
	if has(ContextCodeIntelligence) && !isExcluded(ev.ExplicitExcludes, CapCodeIntelligence) {
		// Availability may be known; still emit action — execution fail-opens.
		out = append(out, RetrievalAction{
			Source: RetrievalCodeGraph,
			Reason: "structural_code_query",
			Query:  query,
		})
	}
	// OpenSpec is Tier-1 evidence, not a compose retrieval action.
	return out
}

func applyExclusions(excludes []string, caps []CapabilityNeed, ctx []ContextNeed, actions []RetrievalAction) ([]CapabilityNeed, []ContextNeed, []RetrievalAction) {
	if len(excludes) == 0 {
		return caps, ctx, actions
	}
	var nc []CapabilityNeed
	for _, c := range caps {
		if isExcluded(excludes, c.ID) {
			continue
		}
		nc = append(nc, c)
	}
	var nctx []ContextNeed
	for _, c := range ctx {
		if c.ID == ContextMemory && isExcluded(excludes, CapMemoryRetrieval) {
			continue
		}
		if c.ID == ContextCodeIntelligence && isExcluded(excludes, CapCodeIntelligence) {
			continue
		}
		if isExcluded(excludes, c.ID) {
			continue
		}
		nctx = append(nctx, c)
	}
	var na []RetrievalAction
	for _, a := range actions {
		if a.Source == RetrievalMemory && isExcluded(excludes, CapMemoryRetrieval) {
			continue
		}
		if a.Source == RetrievalCodeGraph && isExcluded(excludes, CapCodeIntelligence) {
			continue
		}
		na = append(na, a)
	}
	return nc, nctx, na
}

func resolveTaskSkills(ev TaskEvidence, caps []CapabilityNeed) ([]SelectedSkill, []string, error) {
	if len(ev.EnabledPins) == 0 && len(ev.Catalog) == 0 {
		return nil, []string{"no skill pins/catalog supplied; selected_skills empty"}, nil
	}
	enabled := ev.EnabledPins
	if len(enabled) == 0 {
		return nil, []string{"no enabled pins; selected_skills empty"}, nil
	}

	capIDs := make([]string, 0, len(caps))
	requiredSkill := map[string]bool{}
	for _, c := range caps {
		capIDs = append(capIDs, c.ID)
		if c.Strength == StrengthRequired {
			if sid := skillForCapability(c.ID); sid != "" {
				requiredSkill[sid] = true
			}
		}
	}
	// Explicit specialized skill IDs must be selected when pinned+cataloged.
	for _, sid := range ev.ExplicitSkillRequires {
		sid = normalizeSkillRequestID(sid)
		if sid == "" || isExcluded(ev.ExplicitExcludes, sid) {
			continue
		}
		requiredSkill[sid] = true
	}

	max := ev.MaxSkills
	if max <= 0 {
		max = len(enabled)
		if max == 0 {
			max = 5
		}
	}

	resolved, err := skills.Resolve(skills.ResolveInput{
		Intent:        ev.TaskText,
		Technologies:  ev.Technologies,
		RelevantFiles: ev.ExplicitFiles,
		Phase:         ev.Phase,
		Capabilities:  capIDs,
		AdapterIDs:    ev.AdapterIDs,
		Enabled:       enabled,
		Catalog:       ev.Catalog,
		MaxResults:    max,
	})
	if err != nil {
		return nil, nil, err
	}

	// Ensure required mapped skills are present even if score was edge-case zero.
	have := map[string]skills.ResolvedSkill{}
	for _, r := range resolved {
		have[r.ID] = r
	}
	var notes []string
	for skillID := range requiredSkill {
		if _, ok := have[skillID]; ok {
			continue
		}
		if !pinPresent(enabled, skillID) {
			continue
		}
		// Force-select from catalog by exact pin.
		for _, meta := range ev.Catalog {
			pin := findPin(enabled, skillID)
			if pin.ID == "" || meta.ID != pin.ID || meta.Version != pin.Version {
				continue
			}
			reason := "required-capability"
			if containsString(ev.ExplicitSkillRequires, skillID) {
				reason = "explicit-skill-require"
			}
			have[skillID] = skills.ResolvedSkill{
				ID:           meta.ID,
				Version:      meta.Version,
				Digest:       meta.Digest,
				Source:       meta.Source,
				CanonicalRel: meta.CanonicalRel,
				SkillMDRel:   meta.SkillMDRel,
				Score:        100,
				Reason:       reason,
			}
			notes = append(notes, "forced selection of required skill "+skillID)
			break
		}
	}

	out := make([]SelectedSkill, 0, len(have))
	for _, r := range have {
		if isExcluded(ev.ExplicitExcludes, r.ID) || isExcluded(ev.ExplicitExcludes, capabilityForSkill(r.ID)) {
			notes = append(notes, "excluded skill "+r.ID)
			continue
		}
		out = append(out, SelectedSkill{
			ID:           r.ID,
			Version:      r.Version,
			Digest:       r.Digest,
			Source:       r.Source,
			CanonicalRel: r.CanonicalRel,
			SkillMDRel:   r.SkillMDRel,
			Score:        r.Score,
			Reason:       r.Reason,
			Capability:   capabilityForSkill(r.ID),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out, notes, nil
}

func requiredSkillBlockers(caps []CapabilityNeed, selected []SelectedSkill, pins []skills.Pin) []string {
	var blockers []string
	for _, c := range caps {
		if c.Strength != StrengthRequired {
			continue
		}
		skillID := skillForCapability(c.ID)
		if skillID == "" {
			// Agent/core capability — no specialized skill required.
			continue
		}
		if skillSelected(selected, skillID) {
			continue
		}
		if !pinPresent(pins, skillID) {
			blockers = append(blockers, fmt.Sprintf("required skill %q for capability %q is not pinned", skillID, c.ID))
			continue
		}
		blockers = append(blockers, fmt.Sprintf("required skill %q for capability %q could not be selected", skillID, c.ID))
	}
	return blockers
}

func explicitSkillBlockers(skillIDs []string, selected []SelectedSkill, pins []skills.Pin, catalog []skills.Metadata) []string {
	var blockers []string
	for _, sid := range skillIDs {
		sid = normalizeSkillRequestID(sid)
		if sid == "" {
			continue
		}
		if skillSelected(selected, sid) {
			continue
		}
		if !pinPresent(pins, sid) {
			blockers = append(blockers, fmt.Sprintf("explicitly requested skill %q is not pinned/registered", sid))
			continue
		}
		if !catalogHasPin(catalog, findPin(pins, sid)) {
			blockers = append(blockers, fmt.Sprintf("explicitly requested skill %q is pinned but not in catalog", sid))
			continue
		}
		blockers = append(blockers, fmt.Sprintf("explicitly requested skill %q could not be selected", sid))
	}
	return blockers
}

func catalogHasPin(catalog []skills.Metadata, pin skills.Pin) bool {
	if pin.ID == "" {
		return false
	}
	for _, meta := range catalog {
		if meta.ID == pin.ID && meta.Version == pin.Version {
			return true
		}
	}
	return false
}

func filterExcludedSkills(skillsIn, excludes []string) []string {
	var out []string
	for _, s := range skillsIn {
		s = normalizeSkillRequestID(s)
		if s == "" || isExcluded(excludes, s) {
			continue
		}
		out = appendUnique(out, s)
	}
	return out
}

func containsString(list []string, v string) bool {
	v = normalizeSkillRequestID(v)
	for _, x := range list {
		if normalizeSkillRequestID(x) == v {
			return true
		}
	}
	return false
}

func skillSelected(selected []SelectedSkill, id string) bool {
	for _, s := range selected {
		if s.ID == id {
			return true
		}
	}
	return false
}

func pinPresent(pins []skills.Pin, id string) bool {
	for _, p := range pins {
		if p.ID == id {
			return true
		}
	}
	return false
}

func findPin(pins []skills.Pin, id string) skills.Pin {
	for _, p := range pins {
		if p.ID == id {
			return p
		}
	}
	return skills.Pin{}
}

func stronger(a, b string) string {
	if strengthRank(b) > strengthRank(a) {
		return b
	}
	return a
}

func strengthRank(s string) int {
	switch s {
	case StrengthRequired:
		return 3
	case StrengthRelevant:
		return 2
	case StrengthOptional:
		return 1
	default:
		return 0
	}
}
