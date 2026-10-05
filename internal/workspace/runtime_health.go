package workspace

import (
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// Forbidden runtime artifact paths that Atlas must not require or expect.
var forbiddenRuntimeArtifacts = []string{
	"CLAUDE.md",
	"GEMINI.md",
	".agents",
	".claude",
}

// ProjectionStatus is one expected adapter projection path.
type ProjectionStatus struct {
	Adapter string
	Path    string
	Present bool
}

// ForbiddenArtifactStatus is presence of a non-required generated artifact.
type ForbiddenArtifactStatus struct {
	Path    string
	Present bool
}

// RuntimeHealth is a read-only snapshot of Atlas runtime materialization.
// Safe for Status, Doctor, tests, and future Repair; never mutates the workspace.
type RuntimeHealth struct {
	Initialized bool

	ConfigExists bool
	ConfigLoads  bool
	ConfigError  string
	Document     config.ProjectDocument

	StateExists bool
	StateLoads  bool
	StateError  string
	State       config.StateDocument

	RuntimeMaterialized bool

	AgentsExists  bool
	AgentsMarkers config.AgentsMarkers

	SelectedAdapters    []string
	ExpectedProjections []ProjectionStatus

	ContextGraphEnabled  bool
	ContextGraphReadable bool

	BackupsDirExists bool

	ForbiddenArtifacts []ForbiddenArtifactStatus

	Warnings []string
}

// EvaluateRuntimeHealth inspects Atlas runtime files without writing.
// Invalid config yields partial health (no panic, no empty-only result).
func EvaluateRuntimeHealth(root string, atlas AtlasStatus, files FileInfo) RuntimeHealth {
	health := RuntimeHealth{
		Initialized:         atlas.Initialized(),
		ConfigExists:        files.HasAtlasConfig || atlas.HasConfig,
		SelectedAdapters:    []string{},
		ExpectedProjections: []ProjectionStatus{},
		ForbiddenArtifacts:  make([]ForbiddenArtifactStatus, 0, len(forbiddenRuntimeArtifacts)),
		Warnings:            []string{},
	}

	configPath := atlas.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(root, config.FileConfig)
	}

	if health.ConfigExists {
		doc, err := config.LoadProjectDocument(configPath)
		if err != nil {
			health.ConfigLoads = false
			health.ConfigError = err.Error()
			// Fall back to AtlasStatus.Config when ProjectDocument shape fails
			// but legacy Config.Load already succeeded (Initialized=true).
			if atlas.Initialized() {
				health.SelectedAdapters = adaptersFromConfig(atlas.Config)
			}
		} else {
			health.ConfigLoads = true
			health.Document = doc
			health.SelectedAdapters = append([]string{}, doc.Adapters.Selected...)
			health.ContextGraphEnabled = doc.ContextGraphEnabled()
			health.ContextGraphReadable = true
			for _, adapter := range doc.Adapters.Selected {
				switch adapter {
				case "cursor":
					path := config.FileCursorAtlasMDC
					health.ExpectedProjections = append(health.ExpectedProjections, ProjectionStatus{
						Adapter: adapter,
						Path:    path,
						Present: exists(root, path),
					})
				case "opencode":
					path := config.FileOpenCodeAtlas
					health.ExpectedProjections = append(health.ExpectedProjections, ProjectionStatus{
						Adapter: adapter,
						Path:    path,
						Present: exists(root, path),
					})
				}
			}
		}
	}

	statePath := filepath.Join(root, config.FileState)
	if exists(root, config.FileState) {
		health.StateExists = true
		state, err := config.LoadStateDocument(statePath)
		if err != nil {
			health.StateLoads = false
			health.StateError = err.Error()
		} else {
			health.StateLoads = true
			health.State = state
			health.RuntimeMaterialized = state.RuntimeMaterialized
		}
	}

	agentsPath := filepath.Join(root, config.FileAgentsMD)
	if exists(root, config.FileAgentsMD) {
		health.AgentsExists = true
		data, err := os.ReadFile(agentsPath)
		if err == nil {
			health.AgentsMarkers = config.InspectAgentsMarkers(data)
		}
	} else {
		health.AgentsExists = files.HasAgentsFile
	}

	health.BackupsDirExists = exists(root, config.DirBackups)

	for _, path := range forbiddenRuntimeArtifacts {
		health.ForbiddenArtifacts = append(health.ForbiddenArtifacts, ForbiddenArtifactStatus{
			Path:    path,
			Present: exists(root, path),
		})
	}

	health.Warnings = append(health.Warnings, collectRuntimeWarnings(health)...)
	return health
}

func adaptersFromConfig(cfg config.Config) []string {
	var out []string
	if cfg.Adapters.Cursor {
		out = append(out, "cursor")
	}
	if cfg.Adapters.OpenCode {
		out = append(out, "opencode")
	}
	return out
}

func collectRuntimeWarnings(h RuntimeHealth) []string {
	var warnings []string

	if h.Initialized && !h.BackupsDirExists {
		warnings = append(warnings, ".atlas/backups missing for initialized project")
	}
	if h.RuntimeMaterialized && !h.AgentsExists {
		warnings = append(warnings, "runtime_materialized=true but AGENTS.md is missing")
	}
	if h.RuntimeMaterialized && h.AgentsExists && !h.AgentsMarkers.Complete() {
		warnings = append(warnings, "AGENTS.md markers are incomplete")
	}
	if h.AgentsExists && h.AgentsMarkers.Complete() && !h.AgentsMarkers.ContractSatisfied(h.SelectedAdapters) {
		warnings = append(warnings, "AGENTS.md missing selected adapter block")
	}
	if h.AgentsExists && h.AgentsMarkers.Complete() && len(h.AgentsMarkers.UnselectedAdapters(h.SelectedAdapters)) > 0 {
		warnings = append(warnings, "AGENTS.md contains unselected adapter block")
	}
	for _, proj := range h.ExpectedProjections {
		if !proj.Present {
			warnings = append(warnings, "expected adapter projection missing: "+proj.Path)
		}
	}
	for _, art := range h.ForbiddenArtifacts {
		if art.Present {
			warnings = append(warnings, "unexpected generated artifact present: "+art.Path)
		}
	}
	if h.ConfigExists && !h.ConfigLoads && h.ConfigError != "" {
		warnings = append(warnings, "atlas config failed to load")
	}
	if h.Initialized && !h.StateExists {
		warnings = append(warnings, ".atlas/state.yaml missing for initialized project")
	}
	if h.StateExists && !h.StateLoads && h.StateError != "" {
		warnings = append(warnings, "atlas state failed to load")
	}
	return warnings
}
