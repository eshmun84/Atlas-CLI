// Package adapters defines the neutral AdapterCapabilities contract.
// Core domains (skills, status, doctor) must query capabilities by adapter ID
// without provider-name branching.
package adapters

// ID is a canonical adapter identifier.
type ID string

const (
	Cursor   ID = "cursor"
	OpenCode ID = "opencode"
)

// Class is a canonical capability class owned by Atlas Core.
type Class string

const (
	ClassSkills Class = "skills"
	ClassRules  Class = "rules"
	ClassHooks  Class = "hooks"
	ClassAgents Class = "agents"
	ClassMCP    Class = "mcp"
)

// Capabilities describes what a runtime adapter can host.
// Projection path semantics live here so Core stays provider-neutral.
type Capabilities struct {
	Skills bool
	Rules  bool
	Hooks  bool
	Agents bool
	MCP    bool

	// SkillsRootRel is the project-relative skills projection root when Skills is true.
	// Empty when Skills is false. Core must not invent this path.
	SkillsRootRel string
}

// table is the closed set of adapters known to Atlas today.
// Adding Claude later extends this table without changing internal/skills.
var table = map[ID]Capabilities{
	Cursor: {
		Skills:        true,
		Rules:         true,
		Hooks:         false,
		Agents:        true,
		MCP:           true,
		SkillsRootRel: ".cursor/skills",
	},
	OpenCode: {
		Skills:        true,
		Rules:         false,
		Hooks:         false,
		Agents:        true,
		MCP:           true,
		SkillsRootRel: ".opencode/skills",
	},
}

// For returns capabilities for an adapter ID.
func For(id ID) (Capabilities, bool) {
	caps, ok := table[id]
	return caps, ok
}

// Supports reports whether the adapter supports a capability class.
func Supports(id ID, class Class) bool {
	caps, ok := For(id)
	if !ok {
		return false
	}
	switch class {
	case ClassSkills:
		return caps.Skills
	case ClassRules:
		return caps.Rules
	case ClassHooks:
		return caps.Hooks
	case ClassAgents:
		return caps.Agents
	case ClassMCP:
		return caps.MCP
	default:
		return false
	}
}

// SkillsRootRel returns the adapter-owned skills projection root, or empty.
func SkillsRootRel(id ID) string {
	caps, ok := For(id)
	if !ok || !caps.Skills {
		return ""
	}
	return caps.SkillsRootRel
}

// KnownIDs returns stable ordered adapter IDs with a capability table entry.
func KnownIDs() []ID {
	return []ID{Cursor, OpenCode}
}
