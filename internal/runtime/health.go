package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// Forbidden runtime artifact paths that Atlas must not require, expect, or mutate.
// Status/Doctor may report coexistence; Runtime Repair Apply never moves them.
var forbiddenRuntimeArtifacts = []string{
	"AGENT.md",
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
	Adapter     string
	Path        string
	Present     bool
	Matches     bool
	RenderError string // canonical renderer failure (not ordinary drift)
}

// Health is a read-only snapshot of Atlas runtime materialization.
// Safe for Status, Doctor, tests, and future Repair; never mutates the workspace.
type Health struct {
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
	AgentsError   string // unsafe/unreadable AGENTS.md (e.g. symlink leaf)

	SelectedAdapters    []string
	ExpectedProjections []ProjectionStatus
	ExpectedAgents      []AgentFileStatus

	AgentRegistryPresent       bool
	AgentRegistryMatches       bool
	AgentRegistryRenderError   string
	SkillRegistryPresent       bool
	SkillRegistryMatches       bool
	SkillRegistryRenderError   string
	Skills                     skills.SkillHealth
	RuntimeManifestPresent     bool
	RuntimeManifestMatches     bool
	RuntimeManifestRenderError string
	AssetsLockPresent          bool
	AssetsLockMatches          bool
	AssetsLockRenderError      string

	DependsOnSDDContract   bool
	SDDContractPresent     bool
	SDDContractMatches     bool
	SDDContractRenderError string

	ContextEconomy atlascontext.StatusSnapshot

	// CodeIntelligence is an optional provider snapshot (read-only Probe/Status).
	// Absence or errors never fail Atlas discovery.
	CodeIntelligence codeintel.Snapshot

	Home        home.Status
	HomeProject home.ProjectStatus

	ContextGraphEnabled  bool
	ContextGraphReadable bool

	// BackupsDirExists is true when Home project backups or transitional
	// project-local .atlas/backups exist.
	BackupsDirExists bool
	// LegacyBackupsDirExists is transitional project-local .atlas/backups.
	LegacyBackupsDirExists bool

	ForbiddenArtifacts []ForbiddenArtifactStatus

	Warnings []string
}

// EvaluateHealth inspects Atlas runtime files without writing.
// Invalid config yields partial health (no panic, no empty-only result).
func EvaluateHealth(root string, atlas project.AtlasStatus, files project.FileInfo) Health {
	health := Health{
		Initialized:         atlas.Initialized(),
		ConfigExists:        files.HasAtlasConfig || atlas.HasConfig,
		SelectedAdapters:    []string{},
		ExpectedProjections: []ProjectionStatus{},
		ExpectedAgents:      []AgentFileStatus{},
		Home:                home.Inspect(), // read-only; never creates Atlas Home
		ForbiddenArtifacts:  make([]ForbiddenArtifactStatus, 0, len(forbiddenRuntimeArtifacts)),
		Warnings:            []string{},
	}

	if health.ConfigExists {
		doc, err := config.LoadProjectDocumentAt(root)
		if err != nil {
			health.ConfigLoads = false
			health.ConfigError = err.Error()
			// Fall back to project.AtlasStatus.Config when ProjectDocument shape fails
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
						Present: atlasOwnedPresent(root, path),
					})
				case "opencode":
					path := config.FileOpenCodeAtlas
					health.ExpectedProjections = append(health.ExpectedProjections, ProjectionStatus{
						Adapter: adapter,
						Path:    path,
						Present: atlasOwnedPresent(root, path),
					})
				}
			}
			for _, path := range config.AtlasAgentRuntimePaths(doc.Adapters.Selected) {
				adapter := agentAdapterFromPath(path)
				present := atlasOwnedPresent(root, path)
				st := AgentFileStatus{Adapter: adapter, Path: path, Present: present}
				if present {
					expected, renderErr := config.RenderAtlasAgent(filepath.Base(path))
					if renderErr != nil {
						st.RenderError = renderErr.Error()
					} else {
						st.Matches = fileMatches(root, path, expected)
					}
				}
				health.ExpectedAgents = append(health.ExpectedAgents, st)
			}

			homePath := health.Home.Path
			expectedRegistry := config.RenderAgentRegistry(doc.Project.Name, doc.Adapters.Selected, homePath)
			health.AgentRegistryPresent = atlasOwnedPresent(root, config.FileAgentRegistry)
			health.AgentRegistryMatches = health.AgentRegistryPresent && fileMatches(root, config.FileAgentRegistry, expectedRegistry)

			expectedSkillRegistry, skillRegErr := config.RenderSkillRegistry(doc, homePath)
			health.SkillRegistryPresent = atlasOwnedPresent(root, config.FileSkillRegistry)
			if skillRegErr != nil {
				health.SkillRegistryRenderError = skillRegErr.Error()
			} else if health.SkillRegistryPresent {
				health.SkillRegistryMatches = fileMatches(root, config.FileSkillRegistry, expectedSkillRegistry)
			}
			health.Skills = config.InspectSkillHealth(root, doc)

			expectedManifest, err := config.RenderRuntimeManifestYAML(doc.Project.Name, doc.Adapters.Selected)
			health.RuntimeManifestPresent = atlasOwnedPresent(root, config.FileRuntimeManifest)
			if err != nil {
				health.RuntimeManifestRenderError = err.Error()
			} else if health.RuntimeManifestPresent {
				health.RuntimeManifestMatches = fileMatches(root, config.FileRuntimeManifest, expectedManifest)
			}

			expectedLock, err := config.RenderAssetsLockYAMLFor(homePath, doc)
			health.AssetsLockPresent = atlasOwnedPresent(root, config.FileAssetsLock)
			if err != nil {
				health.AssetsLockRenderError = err.Error()
			} else if health.AssetsLockPresent {
				health.AssetsLockMatches = fileMatches(root, config.FileAssetsLock, expectedLock)
			}

			health.DependsOnSDDContract = config.DependsOnSDDOpenSpecContract(doc)
			if health.DependsOnSDDContract {
				health.SDDContractPresent = atlasOwnedPresent(root, config.FileSDDOpenSpecContract)
				if health.SDDContractPresent {
					expectedContract, contractErr := config.RenderSDDOpenSpecContract()
					if contractErr != nil {
						health.SDDContractRenderError = contractErr.Error()
					} else {
						health.SDDContractMatches = fileMatches(root, config.FileSDDOpenSpecContract, expectedContract)
					}
				}
			}
		}
	}

	if present, err := config.AtlasOwnedFileExists(root, config.FileState); err != nil {
		health.StateExists = true
		health.StateLoads = false
		health.StateError = err.Error()
	} else if present {
		health.StateExists = true
		state, err := config.LoadStateDocumentAt(root)
		if err != nil {
			health.StateLoads = false
			health.StateError = err.Error()
		} else {
			health.StateLoads = true
			health.State = state
			health.RuntimeMaterialized = state.RuntimeMaterialized
		}
	}

	if data, err := readAtlasOwned(root, config.FileAgentsMD); err != nil {
		if !os.IsNotExist(err) {
			health.AgentsExists = true
			health.AgentsError = err.Error()
		} else {
			health.AgentsExists = files.HasAgentsFile
		}
	} else {
		health.AgentsExists = true
		health.AgentsMarkers = config.InspectAgentsMarkers(data)
	}

	health.LegacyBackupsDirExists = project.Exists(root, config.DirBackups)

	projectName := ""
	if health.StateLoads {
		projectName = health.State.ProjectName
	}
	if projectName == "" && health.ConfigLoads {
		projectName = health.Document.Project.Name
	}
	if projectID, idErr := home.ProjectID(root, projectName); idErr == nil && health.Home.Path != "" {
		health.HomeProject = home.InspectProject(health.Home.Path, projectID)
		health.BackupsDirExists = health.HomeProject.BackupsPresent || health.LegacyBackupsDirExists
	} else {
		health.BackupsDirExists = health.LegacyBackupsDirExists
	}
	health.ContextEconomy = atlascontext.Inspect(atlascontext.InspectInput{
		Root:           root,
		Initialized:    health.Initialized,
		ProjectName:    projectName,
		StateProjectID: health.State.ContextEconomyProjectID,
		StateUpdatedAt: health.State.ContextEconomyUpdatedAt,
	})

	projectID := ""
	if health.HomeProject.ProjectID != "" {
		projectID = health.HomeProject.ProjectID
	} else if id, idErr := home.ProjectID(root, projectName); idErr == nil {
		projectID = id
	}
	health.CodeIntelligence = probeCodeIntelligence(root, health.Home.Path, projectID)

	for _, path := range forbiddenRuntimeArtifacts {
		health.ForbiddenArtifacts = append(health.ForbiddenArtifacts, ForbiddenArtifactStatus{
			Path:    path,
			Present: project.Exists(root, path),
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

func collectRuntimeWarnings(h Health) []string {
	var warnings []string

	// Backups are created on demand under Atlas Home; absence is not a warning
	// for a healthy initialized project that has never needed quarantine.
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
		case agent.RenderError != "":
			warnings = append(warnings, "Atlas agent canonical render failed: "+agent.Path)
		case !agent.Matches:
			warnings = append(warnings, "Atlas agent content drifted: "+agent.Path)
		}
	}
	if h.ConfigLoads && h.AgentRegistryRenderError != "" {
		warnings = append(warnings, "agent registry canonical render failed")
	} else if h.ConfigLoads && !h.AgentRegistryPresent {
		warnings = append(warnings, "expected agent registry missing: "+config.FileAgentRegistry)
	} else if h.ConfigLoads && h.AgentRegistryPresent && !h.AgentRegistryMatches {
		warnings = append(warnings, "agent registry content drifted")
	}
	if h.ConfigLoads && h.RuntimeManifestRenderError != "" {
		warnings = append(warnings, "runtime manifest canonical render failed")
	} else if h.ConfigLoads && !h.RuntimeManifestPresent {
		warnings = append(warnings, "expected runtime manifest missing: "+config.FileRuntimeManifest)
	} else if h.ConfigLoads && h.RuntimeManifestPresent && !h.RuntimeManifestMatches {
		warnings = append(warnings, "runtime manifest content drifted")
	}
	if h.ConfigLoads && h.AssetsLockRenderError != "" {
		warnings = append(warnings, "assets lock canonical render failed")
	} else if h.ConfigLoads && !h.AssetsLockPresent {
		warnings = append(warnings, "expected assets lock missing: "+config.FileAssetsLock)
	} else if h.ConfigLoads && h.AssetsLockPresent && !h.AssetsLockMatches {
		warnings = append(warnings, "assets lock content drifted")
	}
	if h.ConfigLoads && h.DependsOnSDDContract {
		switch {
		case h.SDDContractRenderError != "":
			warnings = append(warnings, "SDD/OpenSpec contract canonical render failed")
		case !h.SDDContractPresent:
			warnings = append(warnings, "expected SDD/OpenSpec contract missing: "+config.FileSDDOpenSpecContract)
		case !h.SDDContractMatches:
			warnings = append(warnings, "SDD/OpenSpec contract content drifted")
		}
	}
	if h.AgentsError != "" {
		warnings = append(warnings, "AGENTS.md unsafe or unreadable")
	}
	if h.Initialized {
		switch h.ContextEconomy.State {
		case atlascontext.StatusMissing:
			warnings = append(warnings, "Context Economy index missing under Atlas Home")
		case atlascontext.StatusStale:
			warnings = append(warnings, "Context Economy index/capsule stale")
		case atlascontext.StatusUnreadable:
			warnings = append(warnings, "Context Economy index unreadable")
		}
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
			if len(h.Home.AssetErrors) > 0 {
				warnings = append(warnings, "Atlas Home embedded asset integrity errors")
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

func probeCodeIntelligence(root, homePath, projectID string) codeintel.Snapshot {
	svc := codeintel.NewService(&codegraph.Adapter{
		LookPath: exec.LookPath,
		Runner:   codegraph.ExecRunner{Timeout: 2 * time.Second},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return svc.Snapshot(ctx, codeintel.Project{
		Root:     root,
		ID:       projectID,
		HomePath: homePath,
	})
}

func fileMatches(root, rel, expected string) bool {
	data, err := readAtlasOwned(root, rel)
	if err != nil {
		return false
	}
	return string(data) == expected
}

func readAtlasOwned(root, rel string) ([]byte, error) {
	return fsafety.ReadFileContained(root, rel)
}

func atlasOwnedPresent(root, rel string) bool {
	ok, err := config.AtlasOwnedFileExists(root, rel)
	return err == nil && ok
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
