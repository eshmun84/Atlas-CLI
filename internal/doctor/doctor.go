package doctor

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

// Report is the full doctor diagnostics result.
type Report struct {
	Checks []Check
}

// Evaluate builds diagnostics from a successful workspace discovery.
// Runtime checks are read-only and never repair or rematerialize.
func Evaluate(result workspace.DiscoveryResult) Report {
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

	return Report{Checks: checks}
}

func evaluateRuntime(h workspace.RuntimeHealth) []Check {
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
		for _, agent := range h.ExpectedAgents {
			switch {
			case !agent.Present:
				missingAgents++
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
		if h.DependsOnSDDContract {
			switch {
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
				Message:  "preference readable (" + pref + "); engine absence expected",
			})
		}
	}

	checks = append(checks, evaluateContextEconomy(h)...)

	switch {
	case h.Initialized && !h.BackupsDirExists:
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "backups directory",
			Message:  ".atlas/backups missing for initialized project",
		})
	case h.Initialized && h.BackupsDirExists:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "backups directory",
			Message:  ".atlas/backups present",
		})
	case !h.Initialized && h.BackupsDirExists:
		checks = append(checks, Check{
			Severity: SeverityPass,
			Name:     "backups directory",
			Message:  ".atlas/backups present",
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
			Message:  "CLAUDE.md, GEMINI.md, .agents/, .claude/ absent",
		})
	}

	return checks
}

func evaluateContextEconomy(h workspace.RuntimeHealth) []Check {
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
			Message:  "present",
		}}
	default:
		return []Check{{
			Severity: SeverityPass,
			Name:     "context economy",
			Message:  "n/a",
		}}
	}
}

func evaluateHome(h workspace.RuntimeHealth) []Check {
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

	return checks
}

func runtimeMaterializationMessage(h workspace.RuntimeHealth) string {
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
