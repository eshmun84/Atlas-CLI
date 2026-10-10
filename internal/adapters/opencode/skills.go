// Package opencode hosts OpenCode-specific adapter projection implementations.
package opencode

import (
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// SkillsProjector implements skills.Projector for OpenCode.
type SkillsProjector struct{}

// NewSkillsProjector returns an OpenCode skills projector.
func NewSkillsProjector() SkillsProjector { return SkillsProjector{} }

func (SkillsProjector) ID() adapters.ID       { return adapters.OpenCode }
func (SkillsProjector) SupportsSkills() bool  { return true }
func (SkillsProjector) SkillsRootRel() string { return adapters.SkillsRootRel(adapters.OpenCode) }

func (p SkillsProjector) PackageRootRel(skillID string) string {
	return filepath.ToSlash(filepath.Join(p.SkillsRootRel(), skillID))
}

var _ skills.Projector = SkillsProjector{}
