package config

// Atlas project-local path constants.
// Config persistence may create Atlas-owned files under DirAtlas.
// Runtime paths such as FileMemoryDB and FileCapsule are not written in this slice.
const (
	DirAtlas = ".atlas"

	FileConfig     = ".atlas/config.yaml"
	FileLocal      = ".atlas/local.yaml"
	FileState      = ".atlas/state.yaml"
	FileMemoryDB   = ".atlas/memory/atlas.sqlite"
	FileCapsule    = ".atlas/context/memory-capsule.md"
	FileAssetsLock = ".atlas/assets.lock.yaml"
	DirBackups     = ".atlas/backups"
)
