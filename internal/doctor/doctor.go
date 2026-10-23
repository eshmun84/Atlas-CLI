package doctor

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/memoryhealth"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/sdd"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// Report is the full doctor diagnostics result.
type Report struct {
	Checks []Check
}

// Evaluate builds diagnostics from the canonical inspect.Inspection.
// It assigns INFO/PASS/WARN/FAIL from existing evidence only — no second
// discovery, Git probe, tool LookPath, or Code Intelligence process.
// Runtime checks are read-only and never repair or rematerialize.
func Evaluate(result inspect.Inspection) Report {
	var checks []Check

	checks = append(checks, Check{
		Severity: SeverityPass,
		Name:     "workspace",
		Message:  "discovery completed",
	})

	checks = append(checks, evaluateGit(result)...)
	checks = append(checks, evaluateSigning(result)...)
	checks = append(checks, evaluateGitignore(result)...)
	checks = append(checks, evaluateRequiredOpenSpec(result)...)

	checks = append(checks, evaluateRuntime(result.Runtime)...)
	checks = append(checks, evaluateSDD(result.SDD)...)
	checks = append(checks, evaluateHome(result.Runtime)...)

	hasGo := false
	for _, tech := range result.Technologies {
		name := strings.ToLower(tech.Name)
		if name == "go" || strings.Contains(name, "go module") {
			hasGo = true
			break
		}
	}
	for _, tool := range result.Tools {
		if tool.Name == "go" && !hasGo {
			continue
		}
		// OpenSpec tool health is owned by evaluateRequiredOpenSpec when SDD requires it.
		if tool.Name == "openspec" && result.Runtime.ConfigLoads && result.Runtime.DependsOnSDDContract {
			continue
		}
		if tool.Available {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "tool " + tool.Name,
				Message:  "available",
			})
			continue
		}
		checks = append(checks, warn("tool "+tool.Name, "unavailable"))
	}

	checks = append(checks, evaluateMCP(result)...)
	checks = append(checks, evaluateHooks(result)...)

	return Report{Checks: checks}
}

// evaluateMCP is read-only: no writes, auth, installs, or network calls.
func evaluateMCP(result inspect.Inspection) []Check {
	if !result.Runtime.ConfigLoads {
		return []Check{{
			Severity: SeverityInfo,
			Name:     "mcp",
			Message:  "n/a (Atlas not configured)",
		}}
	}
	health, err := config.InspectMCPHealth(result.RootPath, result.Runtime.Document)
	if err != nil {
		return []Check{fail("mcp", err.Error(), RepairManual,
			"Fix MCP configuration manually; Runtime Repair does not rewrite MCP definitions")}
	}
	var checks []Check
	checks = append(checks, Check{
		Severity: SeverityInfo,
		Name:     "mcp selected",
		Message:  fmt.Sprintf("%d selected", health.SelectedCount),
	})
	for _, msg := range health.DefinitionErr {
		if strings.Contains(msg, "invalid") {
			checks = append(checks, fail("mcp definition", msg, RepairManual,
				"Fix invalid MCP definitions in project config"))
			continue
		}
		checks = append(checks, warnAction("mcp definition", msg,
			"Review MCP selections in Configure"))
	}
	for _, adapter := range health.Adapters {
		name := "mcp " + string(adapter.Adapter)
		hasOwnershipConflict := len(adapter.OwnershipConflict) > 0
		switch adapter.Status {
		case "materialized":
			if !hasOwnershipConflict {
				checks = append(checks, Check{
					Severity: SeverityPass,
					Name:     name,
					Message:  fmt.Sprintf("%d/%d materialized", adapter.Materialized, adapter.Selected),
				})
			}
		case "blocked":
			checks = append(checks, fail(name, "projection blocked", RepairManual,
				"Resolve MCP projection blockers, then re-run Configure Apply"))
		case "empty":
			checks = append(checks, Check{
				Severity: SeverityInfo,
				Name:     name,
				Message:  "no materializable MCP selected",
			})
		case "malformed":
			checks = append(checks, fail(name, "native MCP config malformed: "+adapter.MalformError, RepairManual,
				"Fix malformed native MCP config manually; Atlas will not overwrite ambiguous MCP files"))
		case "drifted":
			checks = append(checks, warnAction(name, "projection drifted",
				"Run Configure Apply to reconcile Atlas-owned MCP projections"))
		case "missing":
			checks = append(checks, warnAction(name, "projection missing",
				"Run Configure Apply to materialize selected MCP projections"))
		case "unsupported":
			checks = append(checks, fail(name, "adapter does not support MCP", RepairManual,
				"Deselect MCP on unsupported adapters or choose an MCP-capable adapter"))
		default:
			checks = append(checks, warn(name, string(adapter.Status)))
		}
		for _, w := range adapter.Warnings {
			checks = append(checks, warn(name+" prerequisite", w))
		}
		for _, key := range adapter.OwnershipConflict {
			checks = append(checks, fail(name+" ownership", "ownership conflict on "+key, RepairManual,
				"Resolve project-owned MCP conflicts manually"))
		}
	}
	return checks
}

// evaluateRemoteDefaultBranch reports local knowledge of refs/remotes/<remote>/HEAD.
// Read-only: never contacts remotes or runs git remote set-head.
func evaluateRemoteDefaultBranch(git project.GitInfo) []Check {
	if !git.IsRepo {
		return nil
	}
	remote := strings.TrimSpace(git.DefaultRemote)
	if remote == "" {
		return nil
	}
	if branch := strings.TrimSpace(git.DefaultBranch); branch != "" {
		return []Check{{
			Severity: SeverityPass,
			Name:     "remote default branch",
			Message:  branch,
		}}
	}
	return []Check{warnAction("remote default branch", fmt.Sprintf(
		"unknown locally (refs/remotes/%s/HEAD missing); run: git remote set-head %s -a",
		remote, remote,
	), "Run git remote set-head locally; Atlas will not mutate remotes")}
}

func evaluateRuntime(h runtime.Health) []Check {
	var checks []Check

	if !h.ConfigExists {
		checks = append(checks, warn("atlas config", ".atlas/config.yaml not found"))
	} else {
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "atlas config",
			Message:  ".atlas/config.yaml found",
		})
		if h.ConfigLoads {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas config loads",
				Message:  "valid",
			})
			if h.ProjectSchemaError != "" {
				checks = append(checks, fail("atlas project schema", h.ProjectSchemaError, RepairManual,
					"Use an Atlas version that supports this project schema; Runtime Repair does not migrate project schema"))
			} else {
				checks = append(checks, Check{
					Severity: SeverityPass,
					Name:     "atlas project schema",
					Message:  fmt.Sprintf("compatible (schema %d)", config.CurrentProjectSchemaVersion),
				})
			}
		} else {
			msg := "failed to load"
			if h.ConfigError != "" {
				msg = msg + ": " + h.ConfigError
			}
			checks = append(checks, fail("atlas config loads", msg, RepairManual,
				"Fix .atlas/config.yaml manually; Runtime Repair does not rewrite project config"))
		}
	}

	switch {
	case !h.Initialized && !h.ConfigExists:
		// Non-Atlas project: missing state is neutral.
	case !h.StateExists:
		msg := ".atlas/state.yaml not found"
		if h.Initialized {
			checks = append(checks, fail("atlas state", ".atlas/state.yaml missing for initialized project", RepairAtlas,
				"Run Runtime Repair to restore Atlas project state"))
		} else if h.ConfigExists {
			checks = append(checks, warn("atlas state", msg))
		} else {
			checks = append(checks, warn("atlas state", msg))
		}
	default:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "atlas state",
			Message:  ".atlas/state.yaml found",
		})
		if h.StateLoads {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas state loads",
				Message:  "valid",
			})
		} else {
			msg := "failed to load"
			if h.StateError != "" {
				msg = msg + ": " + h.StateError
			}
			checks = append(checks, fail("atlas state loads", msg, RepairManual,
				"Fix .atlas/state.yaml manually"))
		}
	}

	if h.RuntimeMaterialized && !h.AgentsExists {
		checks = append(checks, fail("runtime materialization", "runtime_materialized=true but AGENTS.md is missing", RepairAtlas,
			"Run Runtime Repair to restore AGENTS.md"))
	} else {
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "runtime materialization",
			Message:  runtimeMaterializationMessage(h),
		})
	}

	switch {
	case h.RuntimeMaterialized && !h.AgentsExists:
		checks = append(checks, fail("agents file", "AGENTS.md required when runtime_materialized=true", RepairAtlas,
			"Run Runtime Repair to restore AGENTS.md"))
	case !h.AgentsExists:
		checks = append(checks, warn("agents file", "AGENTS.md not found"))
	default:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "agents file",
			Message:  "AGENTS.md found",
		})
	}

	switch {
	case h.RuntimeMaterialized && h.AgentsExists && !h.AgentsMarkers.Complete():
		checks = append(checks, fail("agents markers", "AGENTS.md markers incomplete", RepairAtlas,
			"Run Runtime Repair to restore the Atlas AGENTS.md contract"))
	case h.AgentsExists && h.AgentsMarkers.Complete() && !h.AgentsMarkers.ContractSatisfied(h.SelectedAdapters):
		checks = append(checks, fail("agents markers", "AGENTS.md missing selected adapter block", RepairAtlas,
			"Run Runtime Repair to restore missing adapter blocks"))
	case h.AgentsExists && h.AgentsMarkers.Complete() && len(h.AgentsMarkers.UnselectedAdapters(h.SelectedAdapters)) > 0:
		checks = append(checks, Check{
			Severity:          SeverityWarn,
			Name:              "agents markers",
			Message:           "AGENTS.md contains unselected adapter block",
			Repairability:     RepairAtlas,
			RecommendedAction: "Run Runtime Repair to restore AGENTS.md adapter blocks",
		})
	case h.AgentsExists && h.AgentsMarkers.Complete():
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "agents markers",
			Message:  "Atlas contract markers present",
		})
	case h.AgentsExists && !h.AgentsMarkers.Complete() && !h.RuntimeMaterialized:
		checks = append(checks, warn("agents markers", "AGENTS.md markers incomplete"))
	}

	if h.ConfigLoads {
		for _, proj := range h.ExpectedProjections {
			switch {
			case !proj.Present:
				checks = append(checks, fail("adapter projection "+proj.Adapter, proj.Path+" missing", RepairAtlas,
					"Run Runtime Repair to restore Atlas-owned adapter entrypoints"))
			case !proj.Matches:
				checks = append(checks, fail("adapter projection "+proj.Adapter, proj.Path+" content drifted from Atlas contract", RepairAtlas,
					"Run Runtime Repair to restore Atlas-owned adapter entrypoints"))
			default:
				checks = append(checks, Check{
					Severity: SeverityPass,
					Name:     "adapter projection " + proj.Adapter,
					Message:  proj.Path + " present",
				})
			}
		}
		missingAgents := 0
		driftedAgents := 0
		renderFailAgents := 0
		for _, agent := range h.ExpectedAgents {
			switch {
			case !agent.Present:
				missingAgents++
			case agent.RenderError != "":
				renderFailAgents++
			case !agent.Matches:
				driftedAgents++
			}
		}
		switch {
		case missingAgents > 0:
			checks = append(checks, fail("atlas agents", fmt.Sprintf("%d Atlas agent file(s) missing", missingAgents), RepairAtlas,
				"Run Runtime Repair to rematerialize Atlas agents"))
		case renderFailAgents > 0:
			checks = append(checks, fail("atlas agents", fmt.Sprintf("%d Atlas agent canonical render failure(s)", renderFailAgents), RepairManual, ""))
		case driftedAgents > 0:
			checks = append(checks, fail("atlas agents", fmt.Sprintf("%d Atlas agent file(s) drifted", driftedAgents), RepairAtlas,
				"Run Runtime Repair to restore drifted Atlas agents"))
		case len(h.ExpectedAgents) > 0:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas agents",
				Message:  fmt.Sprintf("%d Atlas agent file(s) present", len(h.ExpectedAgents)),
			})
		}
		switch {
		case h.AgentRegistryRenderError != "":
			checks = append(checks, fail("agent registry", "canonical render failed", RepairManual, ""))
		case !h.AgentRegistryPresent:
			checks = append(checks, fail("agent registry", config.FileAgentRegistry+" missing", RepairAtlas,
				"Run Runtime Repair to rematerialize the agent registry"))
		case !h.AgentRegistryMatches:
			checks = append(checks, fail("agent registry", "content drifted", RepairAtlas,
				"Run Runtime Repair to restore the agent registry"))
		default:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "agent registry",
				Message:  config.FileAgentRegistry + " present",
			})
		}
		switch {
		case h.SkillRegistryRenderError != "":
			checks = append(checks, fail("skill registry", "canonical render failed", RepairManual, ""))
		case !h.SkillRegistryPresent:
			checks = append(checks, fail("skill registry", config.FileSkillRegistry+" missing", RepairAtlas,
				"Run Runtime Repair to rematerialize the skill registry"))
		case !h.SkillRegistryMatches:
			checks = append(checks, fail("skill registry", "content drifted", RepairAtlas,
				"Run Runtime Repair to restore the skill registry"))
		default:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "skill registry",
				Message:  config.FileSkillRegistry + " present",
			})
		}
		if !h.Skills.CatalogReadable {
			msg := "catalog unreadable"
			if h.Skills.CatalogError != "" {
				msg = h.Skills.CatalogError
			}
			checks = append(checks, fail("skills catalog", msg, RepairManual,
				"Fix Atlas Home skills catalog readability, then re-run Doctor"))
		} else {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "skills catalog",
				Message:  fmt.Sprintf("%d package(s) available", len(h.Skills.Available)),
			})
		}
		counts := h.Skills.SummaryCounts()
		repairableSkills := countOwnedSkillRepairables(h.Skills)
		unownedStaleOrDrifted := countUnownedSkillNonMissing(h.Skills)
		switch {
		case h.Skills.OwnershipError != "":
			checks = append(checks, fail("skills ownership", "unreadable: "+h.Skills.OwnershipError, RepairManual,
				"Fix Atlas Home skills ownership manually; Runtime Repair does not reconcile skill projections while ownership is unreadable"))
		case counts["conflict"] > 0:
			checks = append(checks, fail("skills projections", fmt.Sprintf("%d conflict(s) preserved", counts["conflict"]), RepairManual,
				"Resolve skill projection conflicts manually; Atlas preserves project-owned conflicts"))
		case repairableSkills > 0:
			checks = append(checks, fail("skills projections", fmt.Sprintf("not ready (missing=%d stale=%d drifted=%d)",
				counts["missing"], counts["stale"], counts["drifted"]), RepairAtlas,
				"Run Runtime Repair to reconcile Atlas-owned skill projections"))
		case counts["invalid"]+counts["unsupported"] > 0:
			checks = append(checks, fail("skills projections", fmt.Sprintf("not repairable automatically (invalid=%d unsupported=%d)",
				counts["invalid"], counts["unsupported"]), RepairManual,
				"Fix or replace invalid/unsupported skill packages manually; Runtime Repair does not rewrite them"))
		case unownedStaleOrDrifted > 0:
			checks = append(checks, fail("skills projections", fmt.Sprintf("%d unowned stale/drifted projection(s) preserved", unownedStaleOrDrifted), RepairManual,
				"Resolve unowned skill projections manually; Atlas preserves non-owned surfaces"))
		case len(h.Skills.Projections) > 0:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "skills projections",
				Message:  fmt.Sprintf("%d ready", counts["ready"]),
			})
		}
		switch {
		case h.RuntimeManifestRenderError != "":
			checks = append(checks, fail("runtime manifest", "canonical render failed", RepairManual, ""))
		case !h.RuntimeManifestPresent:
			checks = append(checks, fail("runtime manifest", config.FileRuntimeManifest+" missing", RepairAtlas,
				"Run Runtime Repair to rematerialize the runtime manifest"))
		case !h.RuntimeManifestMatches:
			checks = append(checks, fail("runtime manifest", "content drifted", RepairAtlas,
				"Run Runtime Repair to restore the runtime manifest"))
		default:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "runtime manifest",
				Message:  config.FileRuntimeManifest + " present",
			})
		}
		switch {
		case h.AssetsLockRenderError != "":
			checks = append(checks, fail("assets lock", "canonical render failed", RepairManual, ""))
		case !h.AssetsLockPresent:
			checks = append(checks, fail("assets lock", config.FileAssetsLock+" missing", RepairAtlas,
				"Run Runtime Repair to rematerialize assets.lock.yaml"))
		case !h.AssetsLockMatches:
			checks = append(checks, fail("assets lock", "content drifted", RepairAtlas,
				"Run Runtime Repair to restore assets.lock.yaml"))
		default:
			if h.ConfigLoads {
				checks = append(checks, Check{
					Severity: SeverityPass,
					Name:     "assets lock",
					Message:  config.FileAssetsLock + " present",
				})
			}
		}
		// SDD/OpenSpec contract + layout readiness are reported by evaluateRequiredOpenSpec.
		if len(h.SelectedAdapters) == 0 {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "adapter selection",
				Message:  "no adapters selected",
			})
		} else {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "adapter selection",
				Message:  "selected adapters validated",
			})
		}
	}

	if h.ConfigLoads {
		if h.ContextGraphReadable {
			pref := "disabled"
			if h.ContextGraphEnabled {
				pref = "enabled"
			}
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "context graph",
				Message:  "Atlas Context Graph preference (" + pref + "); engine NOT IMPLEMENTED",
			})
		}
	}

	// Code Intelligence / Memory / Context Economy use the shared discovery
	// snapshot only (no second probe).
	checks = append(checks, evaluateCodeIntelligence(h)...)
	checks = append(checks, evaluateMemory(h)...)
	checks = append(checks, evaluateContextEconomy(h)...)

	switch {
	case h.Initialized && h.HomeProject.BackupsPresent:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "backups directory",
			Message:  "Home project backups present",
		})
	case h.Initialized && h.LegacyBackupsDirExists:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "backups directory",
			Message:  "transitional .atlas/backups present",
		})
	case h.Initialized && !h.BackupsDirExists:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "backups directory",
			Message:  "no backups yet (created on repair/init backup)",
		})
	case !h.Initialized && h.BackupsDirExists:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "backups directory",
			Message:  "backups present",
		})
	}

	presentForbidden := false
	for _, art := range h.ForbiddenArtifacts {
		if art.Present {
			presentForbidden = true
			checks = append(checks, warn("forbidden artifact "+art.Path, "present but not required or expected"))
		}
	}
	if !presentForbidden {
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "forbidden artifacts",
			Message:  "AGENT.md, CLAUDE.md, GEMINI.md, .agents/, .claude/ absent (Atlas does not mutate these)",
		})
	}

	return checks
}

// evaluateSDD emits read-only Spec-Driven Development checks from the
// Inspection.SDD overview. Never probes the filesystem again.
// Issues are always processed — including when no engine is selected
// (e.g. unsafe/symlinked Spec Engine root).
func evaluateSDD(ov sdd.Overview) []Check {
	var checks []Check
	switch {
	case ov.EngineConflict:
		msg := "multiple engines present without explicit selection"
		for _, issue := range ov.Issues {
			if issue.Kind == sdd.IssueAmbiguous && strings.TrimSpace(issue.Message) != "" {
				msg = issue.Message
				break
			}
		}
		checks = append(checks, fail("sdd engine", msg, RepairManual,
			"Resolve Spec Engine selection manually; Atlas will not guess among conflicting engines"))
	case ov.HasEngine():
		label := strings.TrimSpace(ov.Presence.Label)
		if label == "" {
			label = string(ov.Presence.Engine)
		}
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "sdd engine",
			Message:  label + " detected",
		})
		activeMsg := "none"
		switch n := len(ov.Active); {
		case n == 1:
			id := strings.TrimSpace(ov.Active[0].ChangeID)
			if id == "" {
				id = "(unnamed)"
			}
			activeMsg = id
		case n > 1:
			activeMsg = fmt.Sprintf("%d active", n)
		}
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "sdd active changes",
			Message:  activeMsg,
		})
		if n := len(ov.Archived); n > 0 {
			checks = append(checks, Check{
				Severity: SeverityInfo,
				Name:     "sdd archived changes",
				Message:  fmt.Sprintf("%d archived", n),
			})
		}
	default:
		checks = append(checks, Check{
			Severity: SeverityInfo,
			Name:     "sdd engine",
			Message:  "none detected",
		})
	}

	// Always surface detection/listing issues (unsafe must stay FAIL).
	checks = append(checks, sddIssueChecks(ov.Issues)...)

	for _, ref := range ov.Active {
		if ref.Ambiguous {
			checks = append(checks, warn("sdd ambiguous change", ref.ChangeID+": ambiguous state"))
		}
		for _, note := range ref.Issues {
			if strings.Contains(note, "symlink") {
				checks = append(checks, fail("sdd unsafe path", ref.ChangeID+": "+note, RepairManual,
					"Remove unsafe symlinks from the Spec Engine tree manually"))
			}
		}
	}
	return checks
}

func sddIssueChecks(issues []sdd.Issue) []Check {
	var checks []Check
	for _, issue := range issues {
		// Conflict engine message is already summarized on "sdd engine".
		if issue.Kind == sdd.IssueAmbiguous && strings.Contains(issue.Message, "multiple spec engines") {
			continue
		}
		name := "sdd layout"
		sev := SeverityWarn
		switch issue.Kind {
		case sdd.IssueUnsafe:
			name = "sdd unsafe path"
			sev = SeverityFail
		case sdd.IssueAmbiguous:
			name = "sdd ambiguous change"
			sev = SeverityWarn
		case sdd.IssueMalformed:
			name = "sdd malformed layout"
			sev = SeverityFail
		case sdd.IssueIncomplete:
			name = "sdd incomplete state"
			sev = SeverityWarn
		case sdd.IssueInfo:
			name = "sdd"
			sev = SeverityInfo
		}
		msg := strings.TrimSpace(issue.Message)
		if issue.Change != "" {
			msg = issue.Change + ": " + msg
		}
		switch sev {
		case SeverityFail:
			checks = append(checks, fail(name, msg, RepairManual,
				"Fix Spec Engine layout issues manually; Atlas will not overwrite ambiguous change data"))
		case SeverityWarn:
			checks = append(checks, warn(name, msg))
		case SeverityInfo:
			checks = append(checks, info(name, msg))
		default:
			checks = append(checks, pass(name, msg))
		}
	}
	return checks
}

func evaluateCodeIntelligence(h runtime.Health) []Check {
	snap := h.CodeIntelligence
	provider := strings.TrimSpace(string(snap.Provider))
	if provider == "" {
		provider = "codegraph"
	}

	var checks []Check
	switch snap.State {
	case "", "unavailable":
		msg := "optional provider unavailable"
		if snap.Message != "" {
			msg = snap.Message
		}
		return []Check{{Severity: SeverityInfo, Name: provider, Message: msg}}
	case "missing":
		msg := "previously expected provider missing"
		if snap.Message != "" {
			msg = snap.Message
		}
		return []Check{warnAction(provider, msg, "Install or reconfigure Code Intelligence outside Doctor; Atlas will not auto-install tools")}
	case "incompatible":
		msg := "incompatible"
		if snap.Message != "" {
			msg += " · " + snap.Message
		}
		return []Check{warnAction(provider, msg, "Upgrade/replace the Code Intelligence provider manually")}
	case "error":
		msg := "error"
		if snap.Message != "" {
			msg += " · " + snap.Message
		}
		return []Check{warnAction(provider, msg, "Inspect Code Intelligence provider logs; Doctor will not rebuild graphs")}
	}

	msg := "available"
	if snap.Version != "" {
		msg += " · " + snap.Version
	}
	if snap.GraphPresent {
		msg += " · graph present"
	} else {
		msg += " · graph missing"
	}
	checks = append(checks, Check{Severity: SeverityPass, Name: provider, Message: msg})

	if snap.Executable != "" {
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     provider + " executable",
			Message:  snap.Executable,
		})
	}
	if snap.GraphDBPath != "" {
		if snap.GraphPresent {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     provider + " storage",
				Message:  "graph.db present under Atlas Home",
			})
		} else {
			checks = append(checks, Check{
				Severity: SeverityInfo,
				Name:     provider + " storage",
				Message:  "graph.db missing (run atlas codeintel refresh)",
			})
		}
	}
	switch {
	case snap.MetadataPresent && snap.MetadataValid:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     provider + " metadata",
			Message:  "metadata.json valid",
		})
	case snap.MetadataPresent && !snap.MetadataValid:
		checks = append(checks, warnAction(provider+" metadata", "metadata.json invalid",
			"Refresh Code Intelligence via its explicit update path"))
	case snap.GraphPresent:
		checks = append(checks, warnAction(provider+" metadata", "metadata.json missing",
			"Refresh Code Intelligence via its explicit update path"))
	}
	switch snap.Freshness {
	case "ready":
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     provider + " freshness",
			Message:  "ready",
		})
	case "stale":
		reason := snap.FreshnessReason
		if reason == "" {
			reason = "source fingerprint changed"
		}
		checks = append(checks, warnAction(provider+" freshness", "stale · "+reason,
			"Refresh Code Intelligence via its explicit update path; Doctor will not rebuild"))
	case "missing":
		checks = append(checks, info(provider+" freshness", "graph missing"))
	case "error":
		checks = append(checks, warnAction(provider+" freshness", snap.FreshnessReason,
			"Inspect Code Intelligence freshness errors; Doctor will not rebuild"))
	}
	if snap.RefreshedAt != "" {
		checks = append(checks, Check{
			Severity: SeverityInfo,
			Name:     provider + " last refresh",
			Message:  snap.RefreshedAt,
		})
	}
	return checks
}

func evaluateContextEconomy(h runtime.Health) []Check {
	ce := h.ContextEconomy
	switch {
	case !ce.Applicable:
		return []Check{{
			Severity: SeverityPass,
			Name:     "context economy",
			Message:  "not configured (project uninitialized)",
		}}
	case ce.State == atlascontext.StatusMissing:
		return []Check{warnAction("context economy", "missing under Atlas Home",
			"Run Context Economy Update explicitly")}
	case ce.State == atlascontext.StatusStale:
		return []Check{warnAction("context economy", "stale",
			"Run Context Economy Update explicitly")}
	case ce.State == atlascontext.StatusUnreadable:
		return []Check{warnAction("context economy", "unreadable",
			"Inspect Context Economy files under Atlas Home; Doctor will not rewrite them")}
	case ce.State == atlascontext.StatusPresent:
		return []Check{{
			Severity: SeverityPass,
			Name:     "context economy",
			Message:  "present (v0 file-based under Atlas Home)",
		}}
	default:
		return []Check{{
			Severity: SeverityPass,
			Name:     "context economy",
			Message:  "n/a",
		}}
	}
}

// evaluateMemory maps the shared Memory diagnostics snapshot into doctor checks.
// Evidence is gathered once in runtime.EvaluateHealth — no second DB open here.
func evaluateMemory(h runtime.Health) []Check {
	d := h.Memory
	if len(d.Checks) == 0 && d.Status == "" {
		return []Check{{
			Severity: SeverityInfo,
			Name:     "memory",
			Message:  "n/a",
		}}
	}

	checks := make([]Check, 0, len(d.Checks)+1)
	checks = append(checks, memoryMappedCheck("memory", memoryStatusSeverity(d.Status), memoryStatusMessage(d)))
	for _, c := range d.Checks {
		checks = append(checks, memoryMappedCheck(
			"memory "+shortMemoryCheckName(c.ID),
			memoryCheckSeverity(c.Severity),
			memoryCheckMessage(c),
		))
	}
	return checks
}

func memoryMappedCheck(name string, sev Severity, message string) Check {
	switch sev {
	case SeverityFail:
		return fail(name, message, RepairManual,
			"Memory is diagnostic-only from Doctor; repair schema/storage outside Runtime Repair")
	case SeverityWarn:
		return warnAction(name, message,
			"Review Memory diagnostics; Doctor/Repair will not migrate or rewrite Memory")
	case SeverityPass:
		return pass(name, message)
	default:
		return info(name, message)
	}
}

func memoryStatusSeverity(st memoryhealth.Status) Severity {
	switch st {
	case memoryhealth.StatusHealthy:
		return SeverityPass
	case memoryhealth.StatusDegraded:
		return SeverityWarn
	case memoryhealth.StatusUnavailable:
		return SeverityInfo
	case memoryhealth.StatusFailed:
		return SeverityFail
	default:
		return SeverityInfo
	}
}

func memoryCheckSeverity(sev memoryhealth.CheckSeverity) Severity {
	switch sev {
	case memoryhealth.SeverityPass:
		return SeverityPass
	case memoryhealth.SeverityWarn:
		return SeverityWarn
	case memoryhealth.SeverityFail:
		return SeverityFail
	default:
		return SeverityInfo
	}
}

func memoryStatusMessage(d memoryhealth.Diagnostics) string {
	switch d.Status {
	case memoryhealth.StatusHealthy:
		return fmt.Sprintf(
			"healthy · storage %s · schema %s · retrieval %s · persistence %s · observations %d · quality %s",
			d.StorageLabel, d.SchemaLabel, d.RetrievalLabel, d.PersistenceLabel,
			d.Metrics.ObservationCount, d.QualityLabel,
		)
	case memoryhealth.StatusDegraded:
		return fmt.Sprintf(
			"degraded · storage %s · persistence %s · quality %s",
			d.StorageLabel, d.PersistenceLabel, d.QualityLabel,
		)
	case memoryhealth.StatusFailed:
		return fmt.Sprintf("failed · storage %s · schema %s", d.StorageLabel, d.SchemaLabel)
	case memoryhealth.StatusUnavailable:
		if !d.Enabled {
			return "unavailable (disabled)"
		}
		return fmt.Sprintf("unavailable · storage %s · persistence %s", d.StorageLabel, d.PersistenceLabel)
	default:
		return "n/a"
	}
}

func memoryCheckMessage(c memoryhealth.Check) string {
	msg := strings.TrimSpace(c.Message)
	if hint := strings.TrimSpace(c.RemediationHint); hint != "" {
		if msg == "" {
			return hint
		}
		return msg + " (" + hint + ")"
	}
	return msg
}

func shortMemoryCheckName(id string) string {
	id = strings.TrimPrefix(strings.TrimSpace(id), "memory.")
	id = strings.ReplaceAll(id, "_", " ")
	id = strings.ReplaceAll(id, ".", " ")
	if id == "" {
		return "check"
	}
	return id
}

func evaluateHome(h runtime.Health) []Check {
	var checks []Check
	homePath := strings.TrimSpace(h.Home.Path)
	if homePath == "" {
		checks = append(checks, warn("atlas home path", "unresolved"))
		return checks
	}
	checks = append(checks, Check{
		Severity: SeverityPass,
		Name:     "atlas home path",
		Message:  homePath,
	})

	depends := h.Initialized || h.RuntimeMaterialized
	switch {
	case !h.Home.Exists && depends:
		checks = append(checks, fail("atlas home", "missing", RepairAtlas,
			"Run Runtime Repair (Home-only) or materialize Atlas Home explicitly"))
	case !h.Home.Exists:
		checks = append(checks, warnAction("atlas home", "not created yet",
			"Materialize Atlas Home via install or Runtime Repair Home-only Apply"))
	default:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "atlas home",
			Message:  "present",
		})
	}

	switch {
	case !h.Home.Exists:
		// writability of a missing home is advisory via ancestor bits
		if !h.Home.Writable && depends {
			checks = append(checks, fail("atlas home writable", "parent path not writable", RepairManual,
				"Fix Atlas Home parent directory permissions manually"))
		} else if h.Home.Writable {
			checks = append(checks, pass("atlas home writable", "parent path writable"))
		}
	case !h.Home.Writable:
		if depends {
			checks = append(checks, fail("atlas home writable", "not writable", RepairManual,
				"Fix Atlas Home directory permissions manually"))
		} else {
			checks = append(checks, warn("atlas home writable", "not writable"))
		}
	default:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "atlas home writable",
			Message:  "writable",
		})
	}

	if h.Home.Exists {
		if h.Home.LayoutComplete {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas home layout",
				Message:  "complete",
			})
		} else if depends {
			checks = append(checks, fail("atlas home layout", "incomplete", RepairAtlas,
				"Run Runtime Repair to restore Atlas Home layout"))
		} else {
			checks = append(checks, warnAction("atlas home layout", "incomplete",
				"Run Runtime Repair (Home-only) to complete Atlas Home layout"))
		}
	}

	if h.Home.Exists && h.Home.StateLoads {
		if h.Home.SchemaError != "" {
			checks = append(checks, fail("atlas home schema", h.Home.SchemaError, RepairManual,
				"Use an Atlas version that supports this Home schema; Runtime Repair does not migrate Home schema"))
		} else if h.Home.SchemaVersion > 0 {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas home schema",
				Message:  fmt.Sprintf("compatible (schema %d)", h.Home.SchemaVersion),
			})
		}
	}

	if depends && h.Home.Exists {
		switch {
		case len(h.Home.AssetErrors) > 0:
			checks = append(checks, fail("atlas home assets", fmt.Sprintf("%d embedded integrity error(s)", len(h.Home.AssetErrors)), RepairManual,
				"Investigate Atlas Home asset integrity errors manually"))
		case len(h.Home.MissingAssets) > 0:
			checks = append(checks, fail("atlas home assets", fmt.Sprintf("%d missing", len(h.Home.MissingAssets)), RepairAtlas,
				"Run Runtime Repair to remirror Atlas Home assets"))
		case len(h.Home.DriftedAssets) > 0:
			checks = append(checks, fail("atlas home assets", fmt.Sprintf("%d drifted", len(h.Home.DriftedAssets)), RepairAtlas,
				"Run Runtime Repair to remirror Atlas Home assets"))
		default:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas home assets",
				Message:  fmt.Sprintf("%d mirrored", len(home.BundledAssets())),
			})
		}
	}

	if depends {
		switch {
		case h.HomeProject.ProjectID == "":
			checks = append(checks, warn("atlas home project", "project id unresolved"))
		case !h.Home.Exists:
			checks = append(checks, warnAction("atlas home project", "Home not created yet",
				"Materialize Atlas Home before project-local Home operations"))
		case len(h.HomeProject.IntegrityErrors) > 0:
			checks = append(checks, fail("atlas home project", "integrity: "+strings.Join(h.HomeProject.IntegrityErrors, "; "), RepairManual,
				"Resolve Atlas Home project integrity errors manually"))
		case h.HomeProject.Present:
			detail := "present"
			if h.HomeProject.ContextPresent {
				detail += "; context data"
			}
			if h.HomeProject.BackupsPresent {
				detail += "; backups dir"
			}
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas home project",
				Message:  detail,
			})
		default:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas home project",
				Message:  "no project-local Home data yet",
			})
		}
	}

	return checks
}

func runtimeMaterializationMessage(h runtime.Health) string {
	if !h.StateLoads {
		if h.Initialized {
			return "state unavailable; cannot confirm runtime_materialized"
		}
		return "not materialized"
	}
	if h.RuntimeMaterialized {
		return "runtime_materialized=true"
	}
	return "runtime_materialized=false"
}

// EvaluateWorkingDirectoryError builds a FAIL report when cwd cannot be resolved.
func EvaluateWorkingDirectoryError(err error) Report {
	msg := "current directory cannot be resolved"
	if err != nil {
		msg = msg + ": " + err.Error()
	}
	return Report{
		Checks: []Check{fail("workspace", msg, RepairManual,
			"Fix the working directory / permissions, then re-run Doctor")},
	}
}

// countOwnedSkillRepairables counts skill projections Runtime Repair will plan
// (owned missing/stale/drifted, or missing regardless of ownership).
func countOwnedSkillRepairables(h skills.SkillHealth) int {
	n := 0
	for _, p := range h.Projections {
		switch p.State {
		case "missing":
			n++
		case "stale", "drifted":
			if p.Owned {
				n++
			}
		}
	}
	return n
}

// countUnownedSkillNonMissing counts stale/drifted projections Atlas does not own
// (Repair preserves them; Doctor must not claim atlas-repair).
func countUnownedSkillNonMissing(h skills.SkillHealth) int {
	n := 0
	for _, p := range h.Projections {
		if !p.Owned && (p.State == "stale" || p.State == "drifted") {
			n++
		}
	}
	return n
}

// EvaluateDiscoveryError builds a FAIL report when workspace discovery fails.
func EvaluateDiscoveryError(err error) Report {
	msg := "discovery failed"
	if err != nil {
		msg = msg + ": " + err.Error()
	}
	return Report{
		Checks: []Check{fail("workspace", msg, RepairManual,
			"Resolve discovery failures (permissions/path), then re-run Doctor")},
	}
}

// Counts returns PASS/WARN/FAIL totals.
// INFO checks are informational and do not affect health totals.
func (r Report) Counts() (passed, warnings, failed int) {
	for _, check := range r.Checks {
		switch check.Severity {
		case SeverityPass:
			passed++
		case SeverityWarn:
			warnings++
		case SeverityFail:
			failed++
		}
	}
	return passed, warnings, failed
}

// Failed reports whether any FAIL checks exist.
func (r Report) Failed() bool {
	_, _, failed := r.Counts()
	return failed > 0
}

// ResultLabel returns the summary result string.
func (r Report) ResultLabel() string {
	passed, warnings, failed := r.Counts()
	switch {
	case failed > 0:
		return "not ready"
	case warnings > 0:
		return "ready with warnings"
	case passed > 0:
		return "ready"
	default:
		return "ready"
	}
}
