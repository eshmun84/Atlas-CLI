package project

// Snapshot is the canonical read-only project evidence assembled by Inspect.
//
// It answers: "what project and environment am I looking at right now?"
// Runtime health lives in internal/runtime; internal/inspect composes
// project.Snapshot + runtime.Health into inspect.Inspection. project does not
// import runtime (avoids an import cycle).
//
// Contract: Inspect once → normalize once → consume many times
// (Init, Status, Doctor, and future surfaces).
type Snapshot struct {
	RootPath string

	Git              GitInfo
	Files            FileInfo
	Technologies     []Technology
	Libraries        []Library
	RuntimeArtifacts []string
	Atlas            AtlasStatus
	Tools            []ToolInfo

	Warnings []string
}
