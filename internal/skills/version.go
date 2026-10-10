package skills

import (
	"fmt"
	"regexp"
	"strings"
)

// exactVersionRE is Slice 33 exact pinning only: MAJOR.MINOR.PATCH digits.
// No latest, *, ^, >=, or other semver ranges.
var exactVersionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// ValidateExactVersion accepts only exact MAJOR.MINOR.PATCH versions.
func ValidateExactVersion(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return fmt.Errorf("skills: version is required")
	}
	lower := strings.ToLower(v)
	switch lower {
	case "latest", "*", "x":
		return fmt.Errorf("skills: version %q is not an exact pin", v)
	}
	if strings.ContainsAny(v, "^~><= ") || strings.Contains(v, "||") {
		return fmt.Errorf("skills: version ranges are not supported: %q", v)
	}
	if !exactVersionRE.MatchString(v) {
		return fmt.Errorf("skills: version %q must be exact MAJOR.MINOR.PATCH", v)
	}
	return nil
}

// ValidateSkillID checks a stable skill id (folder name).
func ValidateSkillID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skills: id is required")
	}
	if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return fmt.Errorf("skills: id %q is invalid", id)
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("skills: id %q must be lowercase kebab-case", id)
	}
	return nil
}

// ValidatePin validates an exact pin.
func ValidatePin(p Pin) error {
	if err := ValidateSkillID(p.ID); err != nil {
		return err
	}
	if err := ValidateExactVersion(p.Version); err != nil {
		return err
	}
	return nil
}
