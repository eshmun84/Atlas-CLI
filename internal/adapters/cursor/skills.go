// Package cursor hosts Cursor-specific adapter projection implementations.
package cursor

import (
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// SkillsProjector implements skills.Projector for Cursor.
type SkillsProjector struct{}

// NewSkillsProjector returns a Cursor skills projector.
func NewSkillsProjector() SkillsProjector { return SkillsProjector{} }

func (SkillsProjector) ID() adapters.ID       { return adapters.Cursor }
func (SkillsProjector) SupportsSkills() bool  { return true }
func (SkillsProjector) SkillsRootRel() string { return adapters.SkillsRootRel(adapters.Cursor) }

func (p SkillsProjector) PackageRootRel(skillID string) string {
	return filepath.ToSlash(filepath.Join(p.SkillsRootRel(), skillID))
}

var _ skills.Projector = SkillsProjector{}
