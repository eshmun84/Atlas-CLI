package capability

import "github.com/eshmun84/Atlas-CLI/internal/skills"

// Schema and status vocabulary for CapabilityPlan JSON envelopes.
const (
	SchemaVersion = 1

	StatusSuccess             = "success"
	StatusBlocked             = "blocked"
	StatusInteractionRequired = "interaction_required"
	StatusInvalidRequest      = "invalid_request"

	StrengthRequired = "required"
	StrengthRelevant = "relevant"
	StrengthOptional = "optional"

	ContextMemory           = "memory"
	ContextEconomy          = "context-economy"
	ContextCodeIntelligence = "code-intelligence"
	ContextOpenSpec         = "openspec"
	ContextProjectFiles     = "project-files"

	RetrievalMemory    = "memory"
	RetrievalCE        = "context_economy"
	RetrievalCodeGraph = "codegraph"
)

// TaskEvidence is bounded Tier-0/1 input for DiscoverPlan.
// It is not a universal TaskDescriptor — only fields the planner consumes.
type TaskEvidence struct {
	TaskText              string
	Classification        string
	Workflow              string
	Phase                 string
	ExplicitFiles         []string
	Technologies          []string
	ExplicitRequires      []string // closed capability IDs (core or skill-mapped)
	ExplicitSkillRequires []string // specialized skill IDs from "use X skill" (not vocab expansion)
	ExplicitExcludes      []string
	ActiveChange          string
	// Governance flags from .atlas/config.yaml (reuse — not a second policy engine).
	TestingRequired bool
	ReviewRequired  bool
	// Registry inputs for skill resolution (caller-supplied; planner does not discover Home).
	EnabledPins []skills.Pin
	Catalog     []skills.Metadata
	AdapterIDs  []string
	MaxSkills   int // 0 → len(EnabledPins) or skills default
}

// CapabilityNeed is one closed-vocabulary capability with strength and reasons.
type CapabilityNeed struct {
	ID       string   `json:"id"`
	Strength string   `json:"strength"`
	Reasons  []string `json:"reasons,omitempty"`
}

// ContextNeed is a context source the agent should consult (not a skill).
type ContextNeed struct {
	ID      string   `json:"id"`
	Reasons []string `json:"reasons,omitempty"`
}

// RetrievalAction is a Tier-2 governed retrieval the execution layer may run.
// DiscoverPlan emits actions; it does not execute them.
type RetrievalAction struct {
	Source string `json:"source"`
	Reason string `json:"reason,omitempty"`
	Query  string `json:"query,omitempty"`
}

// SelectedSkill is a task-active skill reference (not projection state).
type SelectedSkill struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Digest       string `json:"digest,omitempty"`
	Source       string `json:"source,omitempty"`
	CanonicalRel string `json:"canonical_rel,omitempty"`
	SkillMDRel   string `json:"skill_md_rel,omitempty"`
	Score        int    `json:"score"`
	Reason       string `json:"reason,omitempty"`
	Capability   string `json:"capability,omitempty"`
}

// CapabilityPlan is the deterministic, inspectable result of DiscoverPlan.
type CapabilityPlan struct {
	SchemaVersion    int               `json:"schema_version"`
	Status           string            `json:"status"`
	Task             string            `json:"task,omitempty"`
	Capabilities     []CapabilityNeed  `json:"capabilities,omitempty"`
	ContextNeeds     []ContextNeed     `json:"context_needs,omitempty"`
	SelectedSkills   []SelectedSkill   `json:"selected_skills,omitempty"`
	RequiredSkills   []string          `json:"required_skills,omitempty"` // explicit specialized skill IDs
	RetrievalActions []RetrievalAction `json:"retrieval_actions,omitempty"`
	Exclusions       []string          `json:"exclusions,omitempty"`
	Notes            []string          `json:"notes,omitempty"`
	Blockers         []string          `json:"blockers,omitempty"`
	ProjectedPins    []string          `json:"projected_pins,omitempty"` // enabled pins (≠ selected)
	Error            *ErrorBody        `json:"error,omitempty"`
}

// ErrorBody is a non-sensitive structured error.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
