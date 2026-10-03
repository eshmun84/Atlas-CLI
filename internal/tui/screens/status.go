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
)

// Status renders the read-only workspace status screen.
func Status(result workspace.DiscoveryResult) string {
	var b strings.Builder

	fmt.Fprintln(&b, statusHead.Render("Atlas Status"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, statusHead.Render("Workspace"))
	fmt.Fprintf(&b, "  Root: %s\n", result.RootPath)
	fmt.Fprintf(&b, "  Atlas state: %s\n\n", result.Atlas.State)

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

	if len(result.Warnings) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, statusHead.Render("Warnings"))
		for _, warning := range result.Warnings {
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

func availability(available bool) string {
	if available {
		return statusYes.Render("available")
	}
	return statusNo.Render("unavailable")
}
