package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillFrontmatter is the authoritative SKILL.md YAML header subset.
type skillFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Triggers    []string `yaml:"triggers"`
}

// ParseSkillMD validates SKILL.md frontmatter and returns metadata fields.
// Atlas does not replace SKILL.md with an LLM-generated summary.
func ParseSkillMD(body string, expectID string) (skillFrontmatter, error) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if !strings.HasPrefix(body, "---\n") {
		return skillFrontmatter{}, fmt.Errorf("skills: SKILL.md missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(body, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		// allow EOF terminator
		if idx := strings.Index(rest, "\n---"); idx >= 0 && strings.TrimSpace(rest[idx+4:]) == "" {
			end = idx
		} else {
			return skillFrontmatter{}, fmt.Errorf("skills: SKILL.md frontmatter not terminated")
		}
	}
	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return skillFrontmatter{}, fmt.Errorf("skills: SKILL.md frontmatter: %w", err)
	}
	fm.Name = strings.TrimSpace(fm.Name)
	fm.Description = strings.TrimSpace(fm.Description)
	if fm.Name == "" {
		return skillFrontmatter{}, fmt.Errorf("skills: SKILL.md name is required")
	}
	if fm.Description == "" {
		return skillFrontmatter{}, fmt.Errorf("skills: SKILL.md description is required")
	}
	if expectID != "" && fm.Name != expectID {
		return skillFrontmatter{}, fmt.Errorf("skills: SKILL.md name %q must match skill id %q", fm.Name, expectID)
	}
	if err := ValidateSkillID(fm.Name); err != nil {
		return skillFrontmatter{}, err
	}
	outTriggers := make([]string, 0, len(fm.Triggers))
	for _, t := range fm.Triggers {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			outTriggers = append(outTriggers, t)
		}
	}
	fm.Triggers = outTriggers
	return fm, nil
}
