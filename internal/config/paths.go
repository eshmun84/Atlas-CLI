package config

// Atlas project-local path constants.
// Config persistence may create Atlas-owned files under DirAtlas.
// Init Apply may also materialize allowlisted runtime entrypoints.
// Memory SQLite/capsule paths are not written in this slice.
const (
	DirAtlas = ".atlas"

	FileConfig          = ".atlas/config.yaml"
	FileLocal           = ".atlas/local.yaml"
	FileState           = ".atlas/state.yaml"
	FileMemoryDB        = ".atlas/memory/atlas.sqlite"
	FileCapsule         = ".atlas/context/memory-capsule.md"
	FileAssetsLock      = ".atlas/assets.lock.yaml"
	FileAgentRegistry   = ".atlas/agent-registry.md"
	FileRuntimeManifest = ".atlas/runtime-manifest.yaml"
	FileSkillRegistry   = ".atlas/skill-registry.md"
	// FileSDDOpenSpecContract is the project-local SDD/OpenSpec operational contract.
	FileSDDOpenSpecContract = ".atlas/contracts/sdd-openspec.md"
	// DirBackups is the transitional project-local backup root.
	// New backups/quarantine write under $ATLAS_HOME/projects/<project-id>/backups/.
	DirBackups = ".atlas/backups"

	// EmbedPathSDDOpenSpecContract is the bundled/Home embed-relative path.
	EmbedPathSDDOpenSpecContract = "contracts/sdd-openspec.md"

	// Runtime entrypoints materialized by Init Apply.
	FileAgentsMD       = "AGENTS.md"
	FileCursorAtlasMDC = ".cursor/rules/atlas.mdc"
	// FileOpenCodeAtlas is the project-local OpenCode adapter convention for Atlas.
	FileOpenCodeAtlas = ".opencode/atlas.md"

	DirCursorAgents   = ".cursor/agents"
	DirOpenCodeAgents = ".opencode/agents"
)
