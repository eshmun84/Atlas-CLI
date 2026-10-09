package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Runtime repair action kinds.
const (
	RepairActionCreate     = "create"
	RepairActionReplace    = "replace"
	RepairActionQuarantine = "quarantine"
)

// Runtime repair target kinds.
const (
	RepairKindAgents   = "agents"
	RepairKindAdapter  = "adapter"
	RepairKindAgent    = "agent"
	RepairKindAtlas    = "atlas"
	RepairKindHome     = "home"
	RepairKindConflict = "conflict"
)

// RepairHomePath is the plan path marker for Atlas Home refresh actions.
const RepairHomePath = "ATLAS_HOME"

// RuntimeRepairTarget is one planned create, replace, or quarantine.
// Quarantine applies only to explicit Atlas-owned paths (e.g. unselected
// adapter Atlas artifacts). Developer/external surfaces such as CLAUDE.md,
// GEMINI.md, AGENT.md, .agents/, and .claude/ are never mutated by Apply.
type RuntimeRepairTarget struct {
	Path    string
	Action  string
	Kind    string
	Reason  string
	Adapter string
	Backup  bool
}

// RuntimeRepairPlan is a read-only repair/quarantine preview.
type RuntimeRepairPlan struct {
	Healthy     bool
	Blocked     bool
	Blockers    []string
	Warnings    []string
	Drift       []string
	Targets     []RuntimeRepairTarget
	Creates     []string
	Replaces    []string
	Quarantines []string
	Backups     []string
}

// NeedsApply reports whether explicit Apply would mutate the workspace.
func (p RuntimeRepairPlan) NeedsApply() bool {
	return !p.Blocked && !p.Healthy && len(p.Targets) > 0
}

// Signature returns a stable hash of planned actions for stale-plan detection.
func (p RuntimeRepairPlan) Signature() string {
	targets := append([]RuntimeRepairTarget(nil), p.Targets...)
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Path != targets[j].Path {
			return targets[i].Path < targets[j].Path
		}
		if targets[i].Action != targets[j].Action {
			return targets[i].Action < targets[j].Action
		}
		return targets[i].Kind < targets[j].Kind
	})
	h := sha256.New()
	for _, target := range targets {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%t\n",
			target.Action,
			target.Path,
			target.Kind,
			target.Reason,
			target.Adapter,
			target.Backup,
		)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// BuildRuntimeRepairPlan inspects Health and the filesystem. Read-only.
func BuildRuntimeRepairPlan(root string, health Health) RuntimeRepairPlan {
	plan := RuntimeRepairPlan{
		Blockers:    []string{},
		Warnings:    []string{},
		Drift:       []string{},
		Targets:     []RuntimeRepairTarget{},
		Creates:     []string{},
		Replaces:    []string{},
		Quarantines: []string{},
		Backups:     []string{},
	}

	if !health.ConfigExists {
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "project is not initialized")
		return plan
	}
	if !health.ConfigLoads {
		plan.Blocked = true
		msg := "atlas config failed to load"
		if health.ConfigError != "" {
			msg += ": " + health.ConfigError
		}
		plan.Blockers = append(plan.Blockers, msg)
		return plan
	}
	if !health.Initialized {
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "project is not initialized")
		return plan
	}

	if !health.AgentsExists {
		addRepairTarget(&plan, RuntimeRepairTarget{
			Path:   config.FileAgentsMD,
			Action: RepairActionCreate,
			Kind:   RepairKindAgents,
			Reason: "AGENTS.md missing from Atlas runtime surface",
		})
		plan.Drift = append(plan.Drift, "AGENTS.md is missing")
	} else {
		agentsData, readErr := fsafety.ReadFileContained(root, config.FileAgentsMD)
		markers := health.AgentsMarkers
		if health.AgentsError != "" && readErr == nil {
			readErr = fmt.Errorf("%s", health.AgentsError)
		}
		switch {
		case readErr != nil:
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:   config.FileAgentsMD,
				Action: RepairActionReplace,
				Kind:   RepairKindAgents,
				Reason: "AGENTS.md unreadable; rewrite Atlas contract",
				Backup: true,
			})
			plan.Drift = append(plan.Drift, "AGENTS.md unreadable")
		case !markers.Complete():
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:   config.FileAgentsMD,
				Action: RepairActionReplace,
				Kind:   RepairKindAgents,
				Reason: "AGENTS.md markers are invalid; treat as non-Atlas content",
				Backup: true,
			})
			plan.Drift = append(plan.Drift, "AGENTS.md markers are incomplete")
		case !markers.ContractSatisfied(health.SelectedAdapters):
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:   config.FileAgentsMD,
				Action: RepairActionReplace,
				Kind:   RepairKindAgents,
				Reason: "AGENTS.md missing selected adapter block(s)",
				Backup: true,
			})
			plan.Drift = append(plan.Drift, "AGENTS.md missing selected adapter block")
		case len(markers.UnselectedAdapters(health.SelectedAdapters)) > 0:
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:   config.FileAgentsMD,
				Action: RepairActionReplace,
				Kind:   RepairKindAgents,
				Reason: "AGENTS.md contains unselected adapter block(s)",
				Backup: true,
			})
			plan.Drift = append(plan.Drift, "AGENTS.md has unselected adapter block")
		default:
			expected, renderErr := config.RenderAgentsMD(
				health.Document.Project.Name,
				health.Document.ContextGraphEnabled(),
				health.SelectedAdapters,
				agentsData,
			)
			if renderErr != nil {
				plan.Blocked = true
				plan.Blockers = append(plan.Blockers, renderErr.Error())
				break
			}
			if string(agentsData) != expected {
				addRepairTarget(&plan, RuntimeRepairTarget{
					Path:   config.FileAgentsMD,
					Action: RepairActionReplace,
					Kind:   RepairKindAgents,
					Reason: "AGENTS.md Atlas-managed content drifted from rendered contract",
					Backup: true,
				})
				plan.Drift = append(plan.Drift, "AGENTS.md managed content drifted")
			}
		}
	}

	selected := map[string]bool{}
	for _, adapter := range health.SelectedAdapters {
		selected[adapter] = true
	}

	for _, proj := range health.ExpectedProjections {
		expected, renderErr := expectedAdapterContent(proj.Adapter, health.Document.Project.Name)
		if renderErr != nil {
			plan.Blocked = true
			plan.Blockers = append(plan.Blockers, renderErr.Error())
			continue
		}
		switch {
		case !proj.Present:
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:    proj.Path,
				Action:  RepairActionCreate,
				Kind:    RepairKindAdapter,
				Reason:  "selected adapter projection missing",
				Adapter: proj.Adapter,
			})
			plan.Drift = append(plan.Drift, "missing adapter projection: "+proj.Path)
		case expected != "" && !adapterProjectionMatches(root, proj.Path, expected):
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:    proj.Path,
				Action:  RepairActionReplace,
				Kind:    RepairKindAdapter,
				Reason:  "selected adapter projection content does not match Atlas contract",
				Adapter: proj.Adapter,
				Backup:  true,
			})
			plan.Drift = append(plan.Drift, "non-Atlas adapter projection content: "+proj.Path)
		}
		// Runtime Repair never walks adapter trees to quarantine developer MCP,
		// settings, or non-Atlas rules. Only Atlas-owned projection/agent paths
		// are created/replaced above / removed when unselected below.
	}

	for _, agent := range health.ExpectedAgents {
		switch {
		case !agent.Present:
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:    agent.Path,
				Action:  RepairActionCreate,
				Kind:    RepairKindAgent,
				Reason:  "Atlas agent missing",
				Adapter: agent.Adapter,
			})
			plan.Drift = append(plan.Drift, "missing Atlas agent: "+agent.Path)
		case !agent.Matches:
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:    agent.Path,
				Action:  RepairActionReplace,
				Kind:    RepairKindAgent,
				Reason:  "Atlas agent content drifted from embedded contract",
				Adapter: agent.Adapter,
				Backup:  true,
			})
			plan.Drift = append(plan.Drift, "drifted Atlas agent: "+agent.Path)
		}
	}

	addAtlasSurfaceRepair(&plan, health.AgentRegistryPresent, health.AgentRegistryMatches, config.FileAgentRegistry, "agent registry")
	addAtlasSurfaceRepair(&plan, health.RuntimeManifestPresent, health.RuntimeManifestMatches, config.FileRuntimeManifest, "runtime manifest")
	addAtlasSurfaceRepair(&plan, health.AssetsLockPresent, health.AssetsLockMatches, config.FileAssetsLock, "assets lock")
	if health.DependsOnSDDContract {
		addAtlasSurfaceRepair(&plan, health.SDDContractPresent, health.SDDContractMatches, config.FileSDDOpenSpecContract, "SDD/OpenSpec contract")
	}

	if health.Initialized || health.RuntimeMaterialized {
		switch {
		case !health.Home.Exists:
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:   RepairHomePath,
				Action: RepairActionCreate,
				Kind:   RepairKindHome,
				Reason: "Atlas Home missing",
			})
			plan.Drift = append(plan.Drift, "Atlas Home missing: "+health.Home.Path)
		case !health.Home.LayoutComplete || len(health.Home.MissingAssets) > 0 || len(health.Home.DriftedAssets) > 0:
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:   RepairHomePath,
				Action: RepairActionReplace,
				Kind:   RepairKindHome,
				Reason: "Atlas Home assets missing or drifted",
			})
			plan.Drift = append(plan.Drift, "Atlas Home needs refresh: "+health.Home.Path)
		}
	}

	// When an adapter is unselected, quarantine only Atlas-owned artifacts under
	// that surface. Never remove the whole tree (MCP configs / user settings stay).
	if !selected["cursor"] {
		for _, path := range atlasOwnedPathsUnderAdapter("cursor") {
			present, err := config.AtlasOwnedFileExists(root, path)
			if err != nil {
				plan.Blocked = true
				plan.Blockers = append(plan.Blockers, "unsafe Atlas-owned Cursor runtime path: "+path+": "+err.Error())
				continue
			}
			if present {
				addRepairTarget(&plan, RuntimeRepairTarget{
					Path:    path,
					Action:  RepairActionQuarantine,
					Kind:    RepairKindConflict,
					Reason:  "unselected Atlas-owned Cursor runtime artifact",
					Adapter: "cursor",
					Backup:  true,
				})
				plan.Drift = append(plan.Drift, "unselected Atlas artifact present: "+path)
			}
		}
	}
	if !selected["opencode"] {
		for _, path := range atlasOwnedPathsUnderAdapter("opencode") {
			present, err := config.AtlasOwnedFileExists(root, path)
			if err != nil {
				plan.Blocked = true
				plan.Blockers = append(plan.Blockers, "unsafe Atlas-owned OpenCode runtime path: "+path+": "+err.Error())
				continue
			}
			if present {
				addRepairTarget(&plan, RuntimeRepairTarget{
					Path:    path,
					Action:  RepairActionQuarantine,
					Kind:    RepairKindConflict,
					Reason:  "unselected Atlas-owned OpenCode runtime artifact",
					Adapter: "opencode",
					Backup:  true,
				})
				plan.Drift = append(plan.Drift, "unselected Atlas artifact present: "+path)
			}
		}
	}

	// Developer/external runtime surfaces (CLAUDE.md, GEMINI.md, AGENT.md,
	// .agents/, .claude/) are reported by Status/Doctor as coexistence only.
	// Runtime Repair Apply never quarantines or deletes them.

	if health.Initialized && !health.StateExists {
		plan.Warnings = append(plan.Warnings, ".atlas/state.yaml missing; Apply will refresh state metadata")
	}

	if len(plan.Targets) == 0 {
		plan.Healthy = true
	}
	return plan
}

// ShowRuntimeRepair reports whether the Runtime Repair sidebar entry should appear.
func ShowRuntimeRepair(root string, atlas project.AtlasStatus, health Health) bool {
	if atlas.Initialized() || health.RuntimeMaterialized {
		return true
	}
	if !health.ConfigExists {
		return false
	}
	plan := BuildRuntimeRepairPlan(root, health)
	return plan.NeedsApply() || plan.Blocked
}

func expectedAdapterContent(adapter, projectName string) (string, error) {
	switch adapter {
	case "cursor":
		return config.RenderCursorAtlasMDC(projectName)
	case "opencode":
		return config.RenderOpenCodeAtlas(projectName)
	default:
		return "", nil
	}
}

func adapterProjectionMatches(root, rel, expected string) bool {
	data, err := fsafety.ReadFileContained(root, rel)
	if err != nil {
		return false
	}
	return string(data) == expected
}

func addRepairTarget(plan *RuntimeRepairPlan, target RuntimeRepairTarget) {
	for _, existing := range plan.Targets {
		if existing.Path == target.Path && existing.Action == target.Action {
			return
		}
	}
	plan.Targets = append(plan.Targets, target)
	switch target.Action {
	case RepairActionCreate:
		plan.Creates = append(plan.Creates, target.Path)
	case RepairActionReplace:
		plan.Replaces = append(plan.Replaces, target.Path)
	case RepairActionQuarantine:
		plan.Quarantines = append(plan.Quarantines, target.Path)
	}
	if target.Backup {
		plan.Backups = append(plan.Backups, target.Path)
	}
}

func atlasOwnedPathsUnderAdapter(adapter string) []string {
	var out []string
	switch adapter {
	case "cursor":
		out = append(out, config.FileCursorAtlasMDC)
	case "opencode":
		out = append(out, config.FileOpenCodeAtlas)
	}
	out = append(out, config.AtlasAgentRuntimePaths([]string{adapter})...)
	sort.Strings(out)
	return out
}

func addAtlasSurfaceRepair(plan *RuntimeRepairPlan, present, matches bool, path, label string) {
	switch {
	case !present:
		addRepairTarget(plan, RuntimeRepairTarget{
			Path:   path,
			Action: RepairActionCreate,
			Kind:   RepairKindAtlas,
			Reason: label + " missing",
		})
		plan.Drift = append(plan.Drift, "missing "+label+": "+path)
	case !matches:
		addRepairTarget(plan, RuntimeRepairTarget{
			Path:   path,
			Action: RepairActionReplace,
			Kind:   RepairKindAtlas,
			Reason: label + " content drifted",
			Backup: true,
		})
		plan.Drift = append(plan.Drift, "drifted "+label+": "+path)
	}
}
