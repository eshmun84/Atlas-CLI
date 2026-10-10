package skills

import (
	"fmt"
	"strings"
)

// RenderSkillRegistry builds .atlas/skill-registry.md from catalog metadata.
// It indexes enabled skills; it does not duplicate full SKILL.md bodies.
func RenderSkillRegistry(projectName string, enabled []Metadata, adapters []string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Atlas Skill Registry\n\n")
	fmt.Fprintf(&b, "Project: **%s**\n\n", name)
	fmt.Fprintf(&b, "Skills are canonical under Atlas Home (`$ATLAS_HOME/assets/skills/<id>/<version>/`).\n")
	fmt.Fprintf(&b, "Runtime copies under adapter skill folders are regenerable projections only.\n")
	fmt.Fprintf(&b, "Atlas never follows projection symlinks into Home and never installs skills from the network.\n\n")

	if len(adapters) > 0 {
		fmt.Fprintf(&b, "## Selected adapters\n\n")
		for _, a := range adapters {
			fmt.Fprintf(&b, "- `%s`\n", a)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(enabled) == 0 {
		fmt.Fprintf(&b, "## Status\n\n")
		fmt.Fprintf(&b, "No skills are enabled for this project.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "## Enabled skills\n\n")
	for _, m := range enabled {
		fmt.Fprintf(&b, "### `%s` `%s`\n\n", m.ID, m.Version)
		fmt.Fprintf(&b, "- Description: %s\n", m.Description)
		fmt.Fprintf(&b, "- Digest: `%s`\n", m.Digest)
		fmt.Fprintf(&b, "- Source: `%s`\n", m.Source)
		fmt.Fprintf(&b, "- Canonical: `$ATLAS_HOME/%s`\n", m.CanonicalRel)
		fmt.Fprintf(&b, "- SKILL.md: `$ATLAS_HOME/%s`\n", m.SkillMDRel)
		if len(m.Triggers) > 0 {
			fmt.Fprintf(&b, "- Triggers: `%s`\n", strings.Join(m.Triggers, "`, `"))
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Policy\n\n")
	fmt.Fprintf(&b, "- Exact versions only (no `latest`, ranges, or automatic upgrades).\n")
	fmt.Fprintf(&b, "- Content equality is not ownership; unknown projections are preserved as conflicts.\n")
	fmt.Fprintf(&b, "- Runtime Repair may restore Atlas-owned projections; Status/Doctor never mutate.\n")
	return b.String()
}
