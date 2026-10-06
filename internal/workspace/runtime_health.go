package workspace

import (
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
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

// AgentFileStatus is one expected Atlas-owned runtime agent file.
type AgentFileStatus struct {
	Adapter string
	Path    string
	Present bool
	Matches bool
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
	ExpectedAgents      []AgentFileStatus

	AgentRegistryPresent   bool
	AgentRegistryMatches   bool
	RuntimeManifestPresent bool
	RuntimeManifestMatches bool
	AssetsLockPresent      bool
	AssetsLockMatches      bool

	Home home.Status

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
		ExpectedAgents:      []AgentFileStatus{},
		Home:                home.Inspect(), // read-only; never creates Atlas Home
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
			for _, path := range config.AtlasAgentRuntimePaths(doc.Adapters.Selected) {
				adapter := agentAdapterFromPath(path)
				present := exists(root, path)
				matches := false
				if present {
					expected, err := config.RenderAtlasAgent(filepath.Base(path))
					if err == nil {
						matches = fileMatches(root, path, expected)
					}
				}
				health.ExpectedAgents = append(health.ExpectedAgents, AgentFileStatus{
					Adapter: adapter,
					Path:    path,
					Present: present,
					Matches: matches,
				})
			}

			homePath := health.Home.Path
			expectedRegistry := config.RenderAgentRegistry(doc.Project.Name, doc.Adapters.Selected, homePath)
			health.AgentRegistryPresent = exists(root, config.FileAgentRegistry)
			health.AgentRegistryMatches = health.AgentRegistryPresent && fileMatches(root, config.FileAgentRegistry, expectedRegistry)

			expectedManifest, err := config.RenderRuntimeManifestYAML(doc.Project.Name, doc.Adapters.Selected)
			health.RuntimeManifestPresent = exists(root, config.FileRuntimeManifest)
			if err == nil && health.RuntimeManifestPresent {
				health.RuntimeManifestMatches = fileMatches(root, config.FileRuntimeManifest, expectedManifest)
			}

			expectedLock, err := config.RenderAssetsLockYAMLFor(homePath, doc)
			health.AssetsLockPresent = exists(root, config.FileAssetsLock)
			if err == nil && health.AssetsLockPresent {
				health.AssetsLockMatches = fileMatches(root, config.FileAssetsLock, expectedLock)
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
	for _, agent := range h.ExpectedAgents {
		switch {
		case !agent.Present:
			warnings = append(warnings, "expected Atlas agent missing: "+agent.Path)
		case !agent.Matches:
			warnings = append(warnings, "Atlas agent content drifted: "+agent.Path)
		}
	}
	if h.ConfigLoads && !h.AgentRegistryPresent {
		warnings = append(warnings, "expected agent registry missing: "+config.FileAgentRegistry)
	} else if h.ConfigLoads && h.AgentRegistryPresent && !h.AgentRegistryMatches {
		warnings = append(warnings, "agent registry content drifted")
	}
	if h.ConfigLoads && !h.RuntimeManifestPresent {
		warnings = append(warnings, "expected runtime manifest missing: "+config.FileRuntimeManifest)
	} else if h.ConfigLoads && h.RuntimeManifestPresent && !h.RuntimeManifestMatches {
		warnings = append(warnings, "runtime manifest content drifted")
	}
	if h.ConfigLoads && !h.AssetsLockPresent {
		warnings = append(warnings, "expected assets lock missing: "+config.FileAssetsLock)
	} else if h.ConfigLoads && h.AssetsLockPresent && !h.AssetsLockMatches {
		warnings = append(warnings, "assets lock content drifted")
	}
	if h.Initialized || h.RuntimeMaterialized {
		if !h.Home.Exists {
			warnings = append(warnings, "Atlas Home missing: "+h.Home.Path)
		} else {
			if !h.Home.Writable {
				warnings = append(warnings, "Atlas Home not writable: "+h.Home.Path)
			}
			if !h.Home.LayoutComplete {
				warnings = append(warnings, "Atlas Home layout incomplete")
			}
			if len(h.Home.MissingAssets) > 0 {
				warnings = append(warnings, "Atlas Home assets missing")
			}
			if len(h.Home.DriftedAssets) > 0 {
				warnings = append(warnings, "Atlas Home assets drifted")
			}
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

func fileMatches(root, rel, expected string) bool {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	return string(data) == expected
}

func agentAdapterFromPath(rel string) string {
	switch {
	case filepath.ToSlash(filepath.Dir(rel)) == config.DirCursorAgents:
		return "cursor"
	case filepath.ToSlash(filepath.Dir(rel)) == config.DirOpenCodeAgents:
		return "opencode"
	default:
		return ""
	}
}
