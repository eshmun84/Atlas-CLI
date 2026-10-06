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
	DirBackups          = ".atlas/backups"

	// Runtime entrypoints materialized by Init Apply.
	FileAgentsMD       = "AGENTS.md"
	FileCursorAtlasMDC = ".cursor/rules/atlas.mdc"
	// FileOpenCodeAtlas is the project-local OpenCode adapter convention for Atlas.
	FileOpenCodeAtlas = ".opencode/atlas.md"

	DirCursorAgents   = ".cursor/agents"
	DirOpenCodeAgents = ".opencode/agents"
)
