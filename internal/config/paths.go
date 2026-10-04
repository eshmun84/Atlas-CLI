package config

// Atlas project-local path constants.
// These identify future materialization targets; this package does not create them.
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
