package workspace

import (
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

// Compatibility aliases — prefer inspect.Inspection, project.*, and runtime.* for
// new code. This package must not gain new productive consumers.
type (
	GitInfo     = project.GitInfo
	GitRemote   = project.GitRemote
	FileInfo    = project.FileInfo
	Technology  = project.Technology
	Library     = project.Library
	ToolInfo    = project.ToolInfo
	AtlasStatus = project.AtlasStatus

	RuntimeHealth           = runtime.Health
	ProjectionStatus        = runtime.ProjectionStatus
	ForbiddenArtifactStatus = runtime.ForbiddenArtifactStatus
	AgentFileStatus         = runtime.AgentFileStatus
	RuntimeRepairPlan       = runtime.RuntimeRepairPlan
	RuntimeRepairTarget     = runtime.RuntimeRepairTarget
	RuntimeRepairResult     = runtime.RuntimeRepairResult

	// DiscoveryResult is an alias of the canonical inspect.Inspection.
	DiscoveryResult = inspect.Inspection
)

const (
	AtlasStateNotInitialized = project.AtlasStateNotInitialized
	AtlasStateInitialized    = project.AtlasStateInitialized
	AtlasStatePartialSetup   = project.AtlasStatePartialSetup
	AtlasStateInvalidConfig  = project.AtlasStateInvalidConfig

	ConfidenceHigh   = project.ConfidenceHigh
	ConfidenceMedium = project.ConfidenceMedium
	ConfidenceLow    = project.ConfidenceLow

	RepairActionCreate     = runtime.RepairActionCreate
	RepairActionReplace    = runtime.RepairActionReplace
	RepairActionQuarantine = runtime.RepairActionQuarantine
	RepairKindAgents       = runtime.RepairKindAgents
	RepairKindAdapter      = runtime.RepairKindAdapter
	RepairKindAgent        = runtime.RepairKindAgent
	RepairKindAtlas        = runtime.RepairKindAtlas
	RepairKindHome         = runtime.RepairKindHome
	RepairKindConflict     = runtime.RepairKindConflict
	RepairHomePath         = runtime.RepairHomePath
	RepairSuccessTitle     = runtime.RepairSuccessTitle
	RepairSuccessBody      = runtime.RepairSuccessBody
	RepairNoopTitle        = runtime.RepairNoopTitle
	RepairNoopBody         = runtime.RepairNoopBody
	RepairStaleMessage     = runtime.RepairStaleMessage
)

// RequiredTools re-exports project.RequiredTools.
var RequiredTools = project.RequiredTools

// Discover forwards to inspect.Inspect (canonical composition).
// Deprecated for new productive call sites — use inspect.Inspect.
func Discover(root string) (DiscoveryResult, error) {
	return inspect.Inspect(root)
}

// DiscoverFiles re-exports project.DiscoverFiles.
func DiscoverFiles(root string) (FileInfo, error) { return project.DiscoverFiles(root) }

// DiscoverGit re-exports project.DiscoverGit.
func DiscoverGit(root string) (GitInfo, error) { return project.DiscoverGit(root) }

// DiscoverTools re-exports project.DiscoverTools.
func DiscoverTools() []ToolInfo { return project.DiscoverTools() }

// DiscoverTechnologies re-exports project.DiscoverTechnologies.
func DiscoverTechnologies(files FileInfo) []Technology {
	return project.DiscoverTechnologies(files)
}

// DiscoverLibraries re-exports project.DiscoverLibraries.
func DiscoverLibraries(root string, files FileInfo) []Library {
	return project.DiscoverLibraries(root, files)
}

// DiscoverRuntimeArtifacts re-exports project.DiscoverRuntimeArtifacts.
func DiscoverRuntimeArtifacts(root string) []string {
	return project.DiscoverRuntimeArtifacts(root)
}

// EvaluateAtlasStatus re-exports project.EvaluateAtlasStatus.
func EvaluateAtlasStatus(root string, files FileInfo) AtlasStatus {
	return project.EvaluateAtlasStatus(root, files)
}

// EvaluateRuntimeHealth re-exports runtime.EvaluateHealth.
func EvaluateRuntimeHealth(root string, atlas AtlasStatus, files FileInfo) RuntimeHealth {
	return runtime.EvaluateHealth(root, atlas, files)
}

// BuildRuntimeRepairPlan re-exports runtime.BuildRuntimeRepairPlan.
func BuildRuntimeRepairPlan(root string, health RuntimeHealth) RuntimeRepairPlan {
	return runtime.BuildRuntimeRepairPlan(root, health)
}

// ApplyRuntimeRepair re-exports runtime.ApplyRuntimeRepair.
func ApplyRuntimeRepair(root, expectedSignature string, nowFn func() time.Time) (RuntimeRepairResult, error) {
	return runtime.ApplyRuntimeRepair(root, expectedSignature, nowFn)
}

// ShowRuntimeRepair re-exports runtime.ShowRuntimeRepair for a composed result.
func ShowRuntimeRepair(result DiscoveryResult) bool {
	return runtime.ShowRuntimeRepair(result.RootPath, result.Atlas, result.Runtime)
}
