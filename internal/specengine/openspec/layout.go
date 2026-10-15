package openspec

// OpenSpec layout constants — private to this adapter.
// internal/sdd must not reference these paths.
//
// Current/temporary: these paths encode Atlas-recognized OpenSpec filesystem
// shapes for inspection/bootstrap. Block B direction: openspec/ lifecycle and
// provider filesystem semantics belong to OpenSpec; Atlas retains SDD
// governance, not long-term ownership of provider layout constants.
const (
	engineID    = "openspec"
	engineLabel = "OpenSpec"

	rootDir      = "openspec"
	configFile   = "openspec/config.yaml"
	specsDir     = "openspec/specs"
	changesDir   = "openspec/changes"
	archiveDir   = "openspec/changes/archive"
	archiveName  = "archive"
	proposalFile = "proposal.md"
	tasksFile    = "tasks.md"
	specsDirName = "specs"
	specFileName = "spec.md"
)
