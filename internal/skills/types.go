// Package skills implements the Atlas Skills v1 canonical domain.
// It is adapter-neutral: provider projection lives behind Projector implementations.
package skills

import "time"

const (
	FileSkillMD = "SKILL.md"

	DirReferences = "references"
	DirScripts    = "scripts"
	DirAssets     = "assets"

	SourceBundled = "bundled"
	SourceHome    = "home"

	OwnershipSchemaVersion = 1
)

// Pin is an exact version pin in project desired state.
type Pin struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version" json:"version"`
}

// Metadata is compact catalog metadata (not a rewrite of SKILL.md).
type Metadata struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Version     string   `yaml:"version" json:"version"`
	Triggers    []string `yaml:"triggers,omitempty" json:"triggers,omitempty"`
	Digest      string   `yaml:"digest" json:"digest"`
	Source      string   `yaml:"source" json:"source"`
	// CanonicalRel is Home-relative path to the package root under assets/skills/.
	CanonicalRel string `yaml:"canonical_rel" json:"canonical_rel"`
	SkillMDRel   string `yaml:"skill_md_rel" json:"skill_md_rel"`
}

// Package is a validated skill package on disk.
type Package struct {
	Meta    Metadata
	RootAbs string
	SkillMD string // full SKILL.md body (loaded only when needed)
	Files   []string
}

// ResolvedSkill is a resolver hit with exact references (not rewritten content).
type ResolvedSkill struct {
	ID           string
	Version      string
	Digest       string
	Source       string
	CanonicalRel string
	SkillMDRel   string
	Score        int
	Reason       string
}

// ResolveInput drives progressive disclosure selection.
type ResolveInput struct {
	Intent        string
	Technologies  []string
	RelevantFiles []string
	Phase         string
	AdapterIDs    []string // adapters that must support Skills
	Enabled       []Pin
	Catalog       []Metadata
	MaxResults    int
}

// OwnershipDocument is machine-local Atlas ownership for skill projections.
type OwnershipDocument struct {
	SchemaVersion int                `yaml:"schema_version"`
	ProjectID     string             `yaml:"project_id"`
	UpdatedAt     string             `yaml:"updated_at,omitempty"`
	Adapters      []AdapterOwnership `yaml:"adapters"`
}

// AdapterOwnership lists Atlas-owned skill projections for one adapter.
type AdapterOwnership struct {
	Adapter string       `yaml:"adapter"`
	Entries []OwnedEntry `yaml:"entries"`
}

// OwnedEntry proves Atlas ownership of one skill projection package.
type OwnedEntry struct {
	SkillID   string `yaml:"skill_id"`
	Version   string `yaml:"version"`
	Digest    string `yaml:"digest"`
	RootRel   string `yaml:"root_rel"` // adapter-owned package root (from Projector)
	Source    string `yaml:"source"`
	UpdatedAt string `yaml:"updated_at,omitempty"`
}

// ProjectionStatus is read-only skill projection health for one pin+adapter.
type ProjectionStatus struct {
	Adapter string
	SkillID string
	Version string
	Digest  string
	RootRel string
	State   string // ready|missing|stale|drifted|invalid|conflict|unsupported
	Owned   bool
	Message string
}

// SkillHealth is read-only project skill health.
type SkillHealth struct {
	CatalogReadable bool
	CatalogError    string
	Available       []Metadata
	Enabled         []Pin
	Projections     []ProjectionStatus
	RegistryPresent bool
	RegistryMatches bool
	CheckedAt       time.Time
}
