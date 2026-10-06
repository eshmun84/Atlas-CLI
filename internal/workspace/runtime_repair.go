package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/eshmun84/Atlas-CLI/internal/config"
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

var competingRuntimeRoots = []string{
	"AGENT.md",
	"CLAUDE.md",
	"GEMINI.md",
	".agents",
	".claude",
}

// RuntimeRepairTarget is one planned create, replace, or quarantine.
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

// BuildRuntimeRepairPlan inspects RuntimeHealth and the filesystem. Read-only.
func BuildRuntimeRepairPlan(root string, health RuntimeHealth) RuntimeRepairPlan {
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
		agentsPath := filepath.Join(root, config.FileAgentsMD)
		agentsData, readErr := os.ReadFile(agentsPath)
		markers := health.AgentsMarkers
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
			expected := config.RenderAgentsMD(
				health.Document.Project.Name,
				health.Document.ContextGraphEnabled(),
				health.SelectedAdapters,
				agentsData,
			)
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
		rootDir := adapterRoot(proj.Adapter)
		keep := atlasOwnedAdapterKeepSet(proj.Adapter)
		extras := extraAdapterFiles(root, rootDir, keep)
		expected := expectedAdapterContent(proj.Adapter, health.Document.Project.Name)
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
		for _, extra := range extras {
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:    extra,
				Action:  RepairActionQuarantine,
				Kind:    RepairKindConflict,
				Reason:  "non-Atlas content under selected adapter path " + rootDir,
				Adapter: proj.Adapter,
				Backup:  true,
			})
			plan.Drift = append(plan.Drift, "competing adapter file: "+extra)
		}
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

	if !selected["cursor"] && exists(root, ".cursor") {
		addRepairTarget(&plan, RuntimeRepairTarget{
			Path:   ".cursor",
			Action: RepairActionQuarantine,
			Kind:   RepairKindConflict,
			Reason: "unselected adapter runtime surface .cursor",
			Backup: true,
		})
		plan.Drift = append(plan.Drift, "unselected adapter path present: .cursor")
	}
	if !selected["opencode"] && exists(root, ".opencode") {
		addRepairTarget(&plan, RuntimeRepairTarget{
			Path:   ".opencode",
			Action: RepairActionQuarantine,
			Kind:   RepairKindConflict,
			Reason: "unselected adapter runtime surface .opencode",
			Backup: true,
		})
		plan.Drift = append(plan.Drift, "unselected adapter path present: .opencode")
	}

	for _, path := range competingRuntimeRoots {
		if exists(root, path) {
			addRepairTarget(&plan, RuntimeRepairTarget{
				Path:   path,
				Action: RepairActionQuarantine,
				Kind:   RepairKindConflict,
				Reason: "competing runtime artifact " + path,
				Backup: true,
			})
			plan.Drift = append(plan.Drift, "competing runtime artifact: "+path)
		}
	}

	if health.Initialized && !health.StateExists {
		plan.Warnings = append(plan.Warnings, ".atlas/state.yaml missing; Apply will refresh state metadata")
	}

	if len(plan.Targets) == 0 {
		plan.Healthy = true
	}
	return plan
}

// ShowRuntimeRepair reports whether the Runtime Repair sidebar entry should appear.
func ShowRuntimeRepair(result DiscoveryResult) bool {
	if result.Atlas.Initialized() || result.Runtime.RuntimeMaterialized {
		return true
	}
	if !result.Runtime.ConfigExists {
		return false
	}
	plan := BuildRuntimeRepairPlan(result.RootPath, result.Runtime)
	return plan.NeedsApply() || plan.Blocked
}

func expectedAdapterContent(adapter, projectName string) string {
	switch adapter {
	case "cursor":
		return config.RenderCursorAtlasMDC(projectName)
	case "opencode":
		return config.RenderOpenCodeAtlas(projectName)
	default:
		return ""
	}
}

func adapterProjectionMatches(root, rel, expected string) bool {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
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

func adapterRoot(adapter string) string {
	switch adapter {
	case "cursor":
		return ".cursor"
	case "opencode":
		return ".opencode"
	default:
		return ""
	}
}

func atlasOwnedAdapterKeepSet(adapter string) map[string]bool {
	keep := map[string]bool{}
	switch adapter {
	case "cursor":
		keep[config.FileCursorAtlasMDC] = true
	case "opencode":
		keep[config.FileOpenCodeAtlas] = true
	}
	for _, path := range config.AtlasAgentRuntimePaths([]string{adapter}) {
		keep[path] = true
	}
	return keep
}

func extraAdapterFiles(root, dir string, keep map[string]bool) []string {
	if dir == "" || !exists(root, dir) {
		return nil
	}
	var extras []string
	base := filepath.Join(root, filepath.FromSlash(dir))
	_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if keep[rel] {
			return nil
		}
		// Developer-owned non-Atlas agents are never quarantined or rewritten.
		if config.IsDeveloperAgentRuntimePath(rel) {
			return nil
		}
		extras = append(extras, rel)
		return nil
	})
	sort.Strings(extras)
	return extras
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
