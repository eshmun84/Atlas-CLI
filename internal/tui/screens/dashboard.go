package screens

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

var (
	dashTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	dashSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dashMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	dashAction  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
)

// Dashboard renders the default Overview screen.
func Dashboard(result workspace.DiscoveryResult) string {
	var b strings.Builder

	name := filepath.Base(result.RootPath)
	if result.Atlas.Initialized() && result.Atlas.Config.Project.Name != "" {
		name = result.Atlas.Config.Project.Name
	}
	if name == "" || name == "." {
		name = "—"
	}

	fmt.Fprintln(&b, dashTitle.Render("Dashboard"))
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "  Project: %s\n", name)
	fmt.Fprintf(&b, "  Atlas state: %s\n", result.Atlas.State)

	mode := detectedModeLabel(result)
	fmt.Fprintf(&b, "  Project mode: %s\n\n", mode)

	fmt.Fprintln(&b, dashSection.Render("Git"))
	fmt.Fprintf(&b, "  Repository: %s\n", yesNoPlain(result.Git.IsRepo))
	fmt.Fprintf(&b, "  Current branch: %s\n", orUnknown(result.Git.CurrentBranch))
	fmt.Fprintf(&b, "  Default remote: %s\n", orUnknown(result.Git.DefaultRemote))
	fmt.Fprintf(&b, "  Default branch: %s\n\n", orUnknown(result.Git.DefaultBranch))

	fmt.Fprintln(&b, dashSection.Render("Technologies"))
	writeNameList(&b, technologyNames(result.Technologies))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, dashSection.Render("Libraries"))
	writeNameList(&b, libraryNames(result.Libraries))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, dashSection.Render("Suggested next action"))
	fmt.Fprintf(&b, "  %s\n", dashAction.Render(suggestedAction(result)))

	return strings.TrimRight(b.String(), "\n")
}

func detectedModeLabel(result workspace.DiscoveryResult) string {
	if result.Atlas.Initialized() && result.Atlas.Config.Project.Mode != "" {
		return result.Atlas.Config.Project.Mode
	}
	if result.Files.HasAtlasConfig {
		return "existing (recommended: Existing project)"
	}
	if result.RootPath == "" {
		return "unknown"
	}
	entries, err := os.ReadDir(result.RootPath)
	if err != nil {
		return "unknown"
	}
	if len(entries) == 0 {
		return "greenfield (recommended: New project)"
	}
	return "existing (recommended: Existing project)"
}

func suggestedAction(result workspace.DiscoveryResult) string {
	switch result.Atlas.State {
	case workspace.AtlasStateNotInitialized, workspace.AtlasStatePartialSetup:
		return "Run Init / Setup"
	case workspace.AtlasStateInvalidConfig:
		return "Run Doctor"
	case workspace.AtlasStateInitialized:
		if len(result.Warnings) > 0 {
			return "Run Doctor"
		}
		return "Open Configure or Status"
	default:
		if len(result.Warnings) > 0 {
			return "Run Doctor"
		}
		return "Open Status"
	}
}

func technologyNames(techs []workspace.Technology) []string {
	names := make([]string, 0, len(techs))
	for _, tech := range techs {
		names = append(names, tech.Name)
	}
	return names
}

func libraryNames(libs []workspace.Library) []string {
	names := make([]string, 0, len(libs))
	for _, lib := range libs {
		names = append(names, lib.Name)
	}
	return names
}

func writeNameList(b *strings.Builder, names []string) {
	if len(names) == 0 {
		fmt.Fprintln(b, "  "+dashMuted.Render("none detected"))
		return
	}
	for _, name := range names {
		fmt.Fprintf(b, "  - %s\n", name)
	}
}

func orUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func yesNoPlain(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
