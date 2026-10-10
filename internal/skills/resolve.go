package skills

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
)

const defaultMaxResolve = 3

// Resolve selects the smallest useful skill set from compact metadata.
// Full SKILL.md content is not loaded here — only exact references are returned.
func Resolve(in ResolveInput) ([]ResolvedSkill, error) {
	if err := requireSkillsCapable(in.AdapterIDs); err != nil {
		return nil, err
	}
	max := in.MaxResults
	if max <= 0 {
		max = defaultMaxResolve
	}
	enabled := map[string]Pin{}
	for _, p := range in.Enabled {
		if err := ValidatePin(p); err != nil {
			return nil, err
		}
		enabled[p.ID] = p
	}
	if len(enabled) == 0 {
		return nil, nil
	}

	intent := strings.ToLower(strings.TrimSpace(in.Intent))
	tech := lowerJoin(in.Technologies)
	files := lowerJoin(in.RelevantFiles)
	phase := strings.ToLower(strings.TrimSpace(in.Phase))
	hay := strings.TrimSpace(intent + " " + tech + " " + files + " " + phase)

	type scored struct {
		ResolvedSkill
	}
	var hits []scored
	for _, meta := range in.Catalog {
		pin, ok := enabled[meta.ID]
		if !ok {
			continue
		}
		if pin.Version != meta.Version {
			// Exact pin only — ignore other catalog versions of the same id.
			continue
		}
		score, reason := scoreMetadata(meta, hay, intent, phase)
		if score <= 0 {
			continue
		}
		hits = append(hits, scored{ResolvedSkill{
			ID:           meta.ID,
			Version:      meta.Version,
			Digest:       meta.Digest,
			Source:       meta.Source,
			CanonicalRel: meta.CanonicalRel,
			SkillMDRel:   meta.SkillMDRel,
			Score:        score,
			Reason:       reason,
		}})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].ID != hits[j].ID {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Version < hits[j].Version
	})
	if len(hits) > max {
		hits = hits[:max]
	}
	out := make([]ResolvedSkill, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.ResolvedSkill)
	}
	return out, nil
}

func requireSkillsCapable(adapterIDs []string) error {
	if len(adapterIDs) == 0 {
		return nil
	}
	for _, raw := range adapterIDs {
		id := adapters.ID(strings.TrimSpace(raw))
		if !adapters.Supports(id, adapters.ClassSkills) {
			return fmt.Errorf("skills: adapter %q does not support Skills", raw)
		}
	}
	return nil
}

func scoreMetadata(meta Metadata, hay, intent, phase string) (int, string) {
	score := 0
	var reasons []string
	name := strings.ToLower(meta.Name)
	desc := strings.ToLower(meta.Description)
	if intent != "" && (strings.Contains(intent, name) || strings.Contains(name, intent)) {
		score += 5
		reasons = append(reasons, "intent-name")
	}
	for _, t := range meta.Triggers {
		t = strings.ToLower(t)
		if t == "" {
			continue
		}
		if intent != "" && strings.Contains(intent, t) {
			score += 4
			reasons = append(reasons, "trigger:"+t)
		} else if hay != "" && strings.Contains(hay, t) {
			score += 2
			reasons = append(reasons, "context:"+t)
		}
	}
	if hay != "" && strings.Contains(desc, intent) && intent != "" {
		score += 2
		reasons = append(reasons, "description")
	}
	if phase != "" && strings.Contains(desc, phase) {
		score += 1
		reasons = append(reasons, "phase")
	}
	if score == 0 {
		return 0, ""
	}
	return score, strings.Join(reasons, ",")
}

func lowerJoin(vals []string) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}
