package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

var (
	statusHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	statusYes  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusNo   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	statusWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusFail = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// Status renders the read-only workspace status screen.
func Status(result workspace.DiscoveryResult) string {
	var b strings.Builder
	rt := result.Runtime

	fmt.Fprintln(&b, statusHead.Render("Atlas Status"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Workspace"))
	fmt.Fprintf(&b, "  Root: %s\n", result.RootPath)
	fmt.Fprintf(&b, "  Atlas state: %s\n\n", result.Atlas.State)

	fmt.Fprintln(&b, statusHead.Render("Atlas Runtime"))
	fmt.Fprintf(&b, "  Initialized: %s\n", yesNo(rt.Initialized))
	fmt.Fprintf(&b, "  .atlas/config.yaml: %s\n", configStatus(rt))
	fmt.Fprintf(&b, "  .atlas/state.yaml: %s\n", stateStatus(rt))
	fmt.Fprintf(&b, "  runtime_materialized: %s\n", boolBadge(rt.RuntimeMaterialized))
	fmt.Fprintf(&b, "  AGENTS.md: %s\n", agentsStatus(rt))
	fmt.Fprintf(&b, "  AGENTS markers: %s\n", markersStatus(rt))
	fmt.Fprintf(&b, "  Adapters: %s\n", adaptersLabel(rt.SelectedAdapters))
	fmt.Fprintln(&b, "  Adapter projections:")
	if len(rt.ExpectedProjections) == 0 {
		fmt.Fprintln(&b, "    none expected")
	} else {
		for _, proj := range rt.ExpectedProjections {
			fmt.Fprintf(&b, "    - %s (%s): %s\n", proj.Path, proj.Adapter, presentMissing(proj.Present))
		}
	}
	fmt.Fprintf(&b, "  Context Graph: %s\n", contextGraphStatus(rt))
	fmt.Fprintf(&b, "  .atlas/backups: %s\n", backupsStatus(rt))

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Git"))
	fmt.Fprintf(&b, "  Repository: %s\n", yesNo(result.Git.IsRepo))
	fmt.Fprintf(&b, "  Current branch: %s\n", displayOrUnknown(result.Git.CurrentBranch))
	fmt.Fprintf(&b, "  Default remote: %s\n", displayOrUnknown(result.Git.DefaultRemote))
	fmt.Fprintf(&b, "  Remote URL: %s\n", displayOrUnknown(result.Git.DefaultRemoteURL))
	fmt.Fprintf(&b, "  Default branch: %s\n", displayOrUnknown(result.Git.DefaultBranch))
	fmt.Fprintln(&b, "  Remotes:")
	if len(result.Git.Remotes) == 0 {
		fmt.Fprintln(&b, "    none detected")
	} else {
		for _, remote := range result.Git.Remotes {
			fmt.Fprintf(&b, "    - %s %s\n", remote.Name, remote.URL)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Files"))
	fmt.Fprintf(&b, "  README.md: %s\n", yesNo(result.Files.HasReadme))
	fmt.Fprintf(&b, "  .gitignore: %s\n", yesNo(result.Files.HasGitignore))
	fmt.Fprintf(&b, "  Makefile: %s\n", yesNo(result.Files.HasMakefile))
	fmt.Fprintf(&b, "  go.mod: %s\n", yesNo(result.Files.HasGoMod))
	fmt.Fprintf(&b, "  AGENTS.md: %s\n", yesNo(result.Files.HasAgentsFile))
	fmt.Fprintf(&b, "  .atlas/: %s\n", yesNo(result.Files.HasAtlasDir))
	fmt.Fprintf(&b, "  .atlas/config.yaml: %s\n", yesNo(result.Files.HasAtlasConfig))

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Technologies"))
	if len(result.Technologies) == 0 {
		fmt.Fprintln(&b, "  none detected")
	} else {
		for _, tech := range result.Technologies {
			fmt.Fprintf(&b, "  - %s (%s, %s)\n", tech.Name, tech.Source, tech.Confidence)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Libraries"))
	if len(result.Libraries) == 0 {
		fmt.Fprintln(&b, "  none detected")
	} else {
		for _, lib := range result.Libraries {
			fmt.Fprintf(&b, "  - %s (%s)\n", lib.Name, lib.Module)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Runtime Artifacts"))
	if len(result.RuntimeArtifacts) == 0 {
		fmt.Fprintln(&b, "  none detected")
	} else {
		for _, path := range result.RuntimeArtifacts {
			fmt.Fprintf(&b, "  - %s\n", path)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Tools"))
	for _, tool := range result.Tools {
		fmt.Fprintf(&b, "  %s: %s\n", tool.Name, availability(tool.Available))
	}

	if len(rt.Warnings) > 0 || len(result.Warnings) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, statusHead.Render("Warnings"))
		seen := map[string]struct{}{}
		for _, warning := range rt.Warnings {
			if _, ok := seen[warning]; ok {
				continue
			}
			seen[warning] = struct{}{}
			fmt.Fprintf(&b, "  - %s\n", statusWarn.Render(warning))
		}
		for _, warning := range result.Warnings {
			if _, ok := seen[warning]; ok {
				continue
			}
			seen[warning] = struct{}{}
			fmt.Fprintf(&b, "  - %s\n", statusWarn.Render(warning))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func displayOrUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func yesNo(v bool) string {
	if v {
		return statusYes.Render("yes")
	}
	return statusNo.Render("no")
}

func boolBadge(v bool) string {
	if v {
		return statusYes.Render("true")
	}
	return statusNo.Render("false")
}

func presentMissing(present bool) string {
	if present {
		return statusYes.Render("present")
	}
	return statusFail.Render("missing")
}

func availability(available bool) string {
	if available {
		return statusYes.Render("available")
	}
	return statusNo.Render("unavailable")
}

func configStatus(rt workspace.RuntimeHealth) string {
	switch {
	case !rt.ConfigExists:
		return statusNo.Render("missing")
	case rt.ConfigLoads:
		return statusYes.Render("ok")
	default:
		return statusFail.Render("invalid")
	}
}

func stateStatus(rt workspace.RuntimeHealth) string {
	switch {
	case !rt.StateExists:
		if rt.Initialized {
			return statusFail.Render("missing")
		}
		return statusNo.Render("missing")
	case rt.StateLoads:
		return statusYes.Render("ok")
	default:
		return statusFail.Render("invalid")
	}
}

func agentsStatus(rt workspace.RuntimeHealth) string {
	if rt.AgentsExists {
		return statusYes.Render("present")
	}
	if rt.RuntimeMaterialized {
		return statusFail.Render("missing")
	}
	return statusNo.Render("missing")
}

func markersStatus(rt workspace.RuntimeHealth) string {
	if !rt.AgentsExists {
		return statusNo.Render("n/a")
	}
	m := rt.AgentsMarkers
	parts := []string{
		markerFlag("BASE:BEGIN", m.BaseBegin),
		markerFlag("BASE:END", m.BaseEnd),
		markerFlag("USER:BEGIN", m.UserBegin),
		markerFlag("USER:END", m.UserEnd),
	}
	label := strings.Join(parts, " ")
	if m.Complete() {
		return statusYes.Render(label)
	}
	if rt.RuntimeMaterialized {
		return statusFail.Render(label)
	}
	return statusWarn.Render(label)
}

func markerFlag(name string, ok bool) string {
	if ok {
		return name + "=yes"
	}
	return name + "=no"
}

func adaptersLabel(adapters []string) string {
	if len(adapters) == 0 {
		return statusNo.Render("none")
	}
	return statusYes.Render(strings.Join(adapters, ", "))
}

func contextGraphStatus(rt workspace.RuntimeHealth) string {
	if !rt.ContextGraphReadable {
		if rt.ConfigExists && !rt.ConfigLoads {
			return statusWarn.Render("unreadable (invalid config)")
		}
		return statusNo.Render("n/a")
	}
	if rt.ContextGraphEnabled {
		return statusYes.Render("enabled")
	}
	return statusNo.Render("disabled")
}

func backupsStatus(rt workspace.RuntimeHealth) string {
	if rt.BackupsDirExists {
		return statusYes.Render("present")
	}
	if rt.Initialized {
		return statusWarn.Render("missing")
	}
	return statusNo.Render("missing")
}
