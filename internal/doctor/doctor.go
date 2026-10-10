package doctor

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
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

	if result.Git.IsRepo {
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "git",
			Message:  "repository detected",
		})
	} else {
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "git",
			Message:  "repository not detected",
		})
	}

	branch := strings.TrimSpace(result.Git.CurrentBranch)
	if branch != "" {
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "branch",
			Message:  branch,
		})
	} else {
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "branch",
			Message:  "not detected",
		})
	}

	if result.Git.IsRepo && len(result.Git.Remotes) == 0 {
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "remotes",
			Message:  "none detected",
		})
	}

	checks = append(checks, evaluateRemoteDefaultBranch(result.Git)...)

	checks = append(checks, evaluateRuntime(result.Runtime)...)
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
		if tool.Available {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "tool " + tool.Name,
				Message:  "available",
			})
			continue
		}
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "tool " + tool.Name,
			Message:  "unavailable",
		})
	}

	checks = append(checks, evaluateMCP(result)...)

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
		return []Check{{
			Severity: SeverityFail,
			Name:     "mcp",
			Message:  err.Error(),
		}}
	}
	var checks []Check
	checks = append(checks, Check{
		Severity: SeverityInfo,
		Name:     "mcp selected",
		Message:  fmt.Sprintf("%d selected", health.SelectedCount),
	})
	for _, msg := range health.DefinitionErr {
		sev := SeverityWarn
		if strings.Contains(msg, "invalid") {
			sev = SeverityFail
		}
		checks = append(checks, Check{
			Severity: sev,
			Name:     "mcp definition",
			Message:  msg,
		})
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
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     name,
				Message:  "projection blocked",
			})
		case "empty":
			checks = append(checks, Check{
				Severity: SeverityInfo,
				Name:     name,
				Message:  "no materializable MCP selected",
			})
		case "malformed":
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     name,
				Message:  "native MCP config malformed: " + adapter.MalformError,
			})
		case "drifted":
			checks = append(checks, Check{
				Severity: SeverityWarn,
				Name:     name,
				Message:  "projection drifted",
			})
		case "missing":
			checks = append(checks, Check{
				Severity: SeverityWarn,
				Name:     name,
				Message:  "projection missing",
			})
		case "unsupported":
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     name,
				Message:  "adapter does not support MCP",
			})
		default:
			checks = append(checks, Check{
				Severity: SeverityWarn,
				Name:     name,
				Message:  string(adapter.Status),
			})
		}
		for _, w := range adapter.Warnings {
			checks = append(checks, Check{
				Severity: SeverityWarn,
				Name:     name + " prerequisite",
				Message:  w,
			})
		}
		for _, key := range adapter.OwnershipConflict {
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     name + " ownership",
				Message:  "ownership conflict on " + key,
			})
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
	return []Check{{
		Severity: SeverityWarn,
		Name:     "remote default branch",
		Message: fmt.Sprintf(
			"unknown locally (refs/remotes/%s/HEAD missing); run: git remote set-head %s -a",
			remote, remote,
		),
	}}
}

func evaluateRuntime(h runtime.Health) []Check {
	var checks []Check

	if !h.ConfigExists {
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "atlas config",
			Message:  ".atlas/config.yaml not found",
		})
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
		} else {
			msg := "failed to load"
			if h.ConfigError != "" {
				msg = msg + ": " + h.ConfigError
			}
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas config loads",
				Message:  msg,
			})
		}
	}

	switch {
	case !h.Initialized && !h.ConfigExists:
		// Non-Atlas project: missing state is neutral.
	case !h.StateExists:
		sev := SeverityWarn
		msg := ".atlas/state.yaml not found"
		if h.Initialized {
			sev = SeverityFail
			msg = ".atlas/state.yaml missing for initialized project"
		} else if h.ConfigExists {
			sev = SeverityWarn
			msg = ".atlas/state.yaml not found"
		}
		checks = append(checks, Check{
			Severity: sev,
			Name:     "atlas state",
			Message:  msg,
		})
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
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas state loads",
				Message:  msg,
			})
		}
	}

	if h.RuntimeMaterialized && !h.AgentsExists {
		checks = append(checks, Check{
			Severity: SeverityFail,
			Name:     "runtime materialization",
			Message:  "runtime_materialized=true but AGENTS.md is missing",
		})
	} else {
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "runtime materialization",
			Message:  runtimeMaterializationMessage(h),
		})
	}

	switch {
	case h.RuntimeMaterialized && !h.AgentsExists:
		checks = append(checks, Check{
			Severity: SeverityFail,
			Name:     "agents file",
			Message:  "AGENTS.md required when runtime_materialized=true",
		})
	case !h.AgentsExists:
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "agents file",
			Message:  "AGENTS.md not found",
		})
	default:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "agents file",
			Message:  "AGENTS.md found",
		})
	}

	switch {
	case h.RuntimeMaterialized && h.AgentsExists && !h.AgentsMarkers.Complete():
		checks = append(checks, Check{
			Severity: SeverityFail,
			Name:     "agents markers",
			Message:  "AGENTS.md markers incomplete",
		})
	case h.AgentsExists && h.AgentsMarkers.Complete() && !h.AgentsMarkers.ContractSatisfied(h.SelectedAdapters):
		checks = append(checks, Check{
			Severity: SeverityFail,
			Name:     "agents markers",
			Message:  "AGENTS.md missing selected adapter block",
		})
	case h.AgentsExists && h.AgentsMarkers.Complete() && len(h.AgentsMarkers.UnselectedAdapters(h.SelectedAdapters)) > 0:
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "agents markers",
			Message:  "AGENTS.md contains unselected adapter block",
		})
	case h.AgentsExists && h.AgentsMarkers.Complete():
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "agents markers",
			Message:  "Atlas contract markers present",
		})
	case h.AgentsExists && !h.AgentsMarkers.Complete() && !h.RuntimeMaterialized:
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "agents markers",
			Message:  "AGENTS.md markers incomplete",
		})
	}

	if h.ConfigLoads {
		for _, proj := range h.ExpectedProjections {
			if proj.Present {
				checks = append(checks, Check{
					Severity: SeverityPass,
					Name:     "adapter projection " + proj.Adapter,
					Message:  proj.Path + " present",
				})
				continue
			}
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "adapter projection " + proj.Adapter,
				Message:  proj.Path + " missing",
			})
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
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas agents",
				Message:  fmt.Sprintf("%d Atlas agent file(s) missing", missingAgents),
			})
		case renderFailAgents > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas agents",
				Message:  fmt.Sprintf("%d Atlas agent canonical render failure(s)", renderFailAgents),
			})
		case driftedAgents > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas agents",
				Message:  fmt.Sprintf("%d Atlas agent file(s) drifted", driftedAgents),
			})
		case len(h.ExpectedAgents) > 0:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas agents",
				Message:  fmt.Sprintf("%d Atlas agent file(s) present", len(h.ExpectedAgents)),
			})
		}
		switch {
		case h.AgentRegistryRenderError != "":
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "agent registry",
				Message:  "canonical render failed",
			})
		case !h.AgentRegistryPresent:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "agent registry",
				Message:  config.FileAgentRegistry + " missing",
			})
		case !h.AgentRegistryMatches:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "agent registry",
				Message:  "content drifted",
			})
		default:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "agent registry",
				Message:  config.FileAgentRegistry + " present",
			})
		}
		switch {
		case h.SkillRegistryRenderError != "":
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "skill registry",
				Message:  "canonical render failed",
			})
		case !h.SkillRegistryPresent:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "skill registry",
				Message:  config.FileSkillRegistry + " missing",
			})
		case !h.SkillRegistryMatches:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "skill registry",
				Message:  "content drifted",
			})
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
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "skills catalog",
				Message:  msg,
			})
		} else {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "skills catalog",
				Message:  fmt.Sprintf("%d package(s) available", len(h.Skills.Available)),
			})
		}
		counts := h.Skills.SummaryCounts()
		switch {
		case counts["conflict"] > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "skills projections",
				Message:  fmt.Sprintf("%d conflict(s) preserved", counts["conflict"]),
			})
		case counts["missing"]+counts["stale"]+counts["drifted"]+counts["invalid"]+counts["unsupported"] > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "skills projections",
				Message: fmt.Sprintf("not ready (missing=%d stale=%d drifted=%d invalid=%d unsupported=%d)",
					counts["missing"], counts["stale"], counts["drifted"], counts["invalid"], counts["unsupported"]),
			})
		case len(h.Skills.Projections) > 0:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "skills projections",
				Message:  fmt.Sprintf("%d ready", counts["ready"]),
			})
		}
		switch {
		case h.RuntimeManifestRenderError != "":
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "runtime manifest",
				Message:  "canonical render failed",
			})
		case !h.RuntimeManifestPresent:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "runtime manifest",
				Message:  config.FileRuntimeManifest + " missing",
			})
		case !h.RuntimeManifestMatches:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "runtime manifest",
				Message:  "content drifted",
			})
		default:
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "runtime manifest",
				Message:  config.FileRuntimeManifest + " present",
			})
		}
		switch {
		case h.AssetsLockRenderError != "":
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "assets lock",
				Message:  "canonical render failed",
			})
		case !h.AssetsLockPresent:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "assets lock",
				Message:  config.FileAssetsLock + " missing",
			})
		case !h.AssetsLockMatches:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "assets lock",
				Message:  "content drifted",
			})
		default:
			if h.ConfigLoads {
				checks = append(checks, Check{
					Severity: SeverityPass,
					Name:     "assets lock",
					Message:  config.FileAssetsLock + " present",
				})
			}
		}
		if h.DependsOnSDDContract {
			switch {
			case h.SDDContractRenderError != "":
				checks = append(checks, Check{
					Severity: SeverityFail,
					Name:     "sdd openspec contract",
					Message:  "canonical render failed",
				})
			case !h.SDDContractPresent:
				checks = append(checks, Check{
					Severity: SeverityFail,
					Name:     "sdd openspec contract",
					Message:  config.FileSDDOpenSpecContract + " missing",
				})
			case !h.SDDContractMatches:
				checks = append(checks, Check{
					Severity: SeverityFail,
					Name:     "sdd openspec contract",
					Message:  "content drifted",
				})
			default:
				checks = append(checks, Check{
					Severity: SeverityPass,
					Name:     "sdd openspec contract",
					Message:  config.FileSDDOpenSpecContract + " present",
				})
			}
		}
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

	// Code Intelligence uses the shared discovery snapshot only (no second probe).
	checks = append(checks, evaluateCodeIntelligence(h)...)
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
			checks = append(checks, Check{
				Severity: SeverityWarn,
				Name:     "forbidden artifact " + art.Path,
				Message:  "present but not required or expected",
			})
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
		return []Check{{Severity: SeverityWarn, Name: provider, Message: msg}}
	case "incompatible":
		msg := "incompatible"
		if snap.Message != "" {
			msg += " · " + snap.Message
		}
		return []Check{{Severity: SeverityWarn, Name: provider, Message: msg}}
	case "error":
		msg := "error"
		if snap.Message != "" {
			msg += " · " + snap.Message
		}
		return []Check{{Severity: SeverityWarn, Name: provider, Message: msg}}
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
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     provider + " metadata",
			Message:  "metadata.json invalid",
		})
	case snap.GraphPresent:
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     provider + " metadata",
			Message:  "metadata.json missing",
		})
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
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     provider + " freshness",
			Message:  "stale · " + reason,
		})
	case "missing":
		checks = append(checks, Check{
			Severity: SeverityInfo,
			Name:     provider + " freshness",
			Message:  "graph missing",
		})
	case "error":
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     provider + " freshness",
			Message:  snap.FreshnessReason,
		})
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
		return []Check{{
			Severity: SeverityWarn,
			Name:     "context economy",
			Message:  "missing under Atlas Home",
		}}
	case ce.State == atlascontext.StatusStale:
		return []Check{{
			Severity: SeverityWarn,
			Name:     "context economy",
			Message:  "stale",
		}}
	case ce.State == atlascontext.StatusUnreadable:
		return []Check{{
			Severity: SeverityWarn,
			Name:     "context economy",
			Message:  "unreadable",
		}}
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

func evaluateHome(h runtime.Health) []Check {
	var checks []Check
	homePath := strings.TrimSpace(h.Home.Path)
	if homePath == "" {
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "atlas home path",
			Message:  "unresolved",
		})
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
		checks = append(checks, Check{
			Severity: SeverityFail,
			Name:     "atlas home",
			Message:  "missing",
		})
	case !h.Home.Exists:
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "atlas home",
			Message:  "not created yet",
		})
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
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas home writable",
				Message:  "parent path not writable",
			})
		} else if h.Home.Writable {
			checks = append(checks, Check{
				Severity: SeverityPass,
				Name:     "atlas home writable",
				Message:  "parent path writable",
			})
		}
	case !h.Home.Writable:
		sev := SeverityWarn
		if depends {
			sev = SeverityFail
		}
		checks = append(checks, Check{
			Severity: sev,
			Name:     "atlas home writable",
			Message:  "not writable",
		})
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
		} else {
			sev := SeverityWarn
			if depends {
				sev = SeverityFail
			}
			checks = append(checks, Check{
				Severity: sev,
				Name:     "atlas home layout",
				Message:  "incomplete",
			})
		}
	}

	if depends && h.Home.Exists {
		switch {
		case len(h.Home.AssetErrors) > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas home assets",
				Message:  fmt.Sprintf("%d embedded integrity error(s)", len(h.Home.AssetErrors)),
			})
		case len(h.Home.MissingAssets) > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas home assets",
				Message:  fmt.Sprintf("%d missing", len(h.Home.MissingAssets)),
			})
		case len(h.Home.DriftedAssets) > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas home assets",
				Message:  fmt.Sprintf("%d drifted", len(h.Home.DriftedAssets)),
			})
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
			checks = append(checks, Check{
				Severity: SeverityWarn,
				Name:     "atlas home project",
				Message:  "project id unresolved",
			})
		case !h.Home.Exists:
			checks = append(checks, Check{
				Severity: SeverityWarn,
				Name:     "atlas home project",
				Message:  "Home not created yet",
			})
		case len(h.HomeProject.IntegrityErrors) > 0:
			checks = append(checks, Check{
				Severity: SeverityFail,
				Name:     "atlas home project",
				Message:  "integrity: " + strings.Join(h.HomeProject.IntegrityErrors, "; "),
			})
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
		Checks: []Check{{
			Severity: SeverityFail,
			Name:     "workspace",
			Message:  msg,
		}},
	}
}

// EvaluateDiscoveryError builds a FAIL report when workspace discovery fails.
func EvaluateDiscoveryError(err error) Report {
	msg := "discovery failed"
	if err != nil {
		msg = msg + ": " + err.Error()
	}
	return Report{
		Checks: []Check{{
			Severity: SeverityFail,
			Name:     "workspace",
			Message:  msg,
		}},
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
