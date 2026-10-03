package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

var (
	cfgTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	cfgSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	cfgMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// Configure renders a read-only Atlas configuration summary.
func Configure(result workspace.DiscoveryResult) string {
	var b strings.Builder

	fmt.Fprintln(&b, cfgTitle.Render("Configure"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, cfgSection.Render("Atlas project"))
	fmt.Fprintf(&b, "  State: %s\n", result.Atlas.State)
	fmt.Fprintf(&b, "  Config path: %s\n\n", result.Atlas.ConfigPath)

	fmt.Fprintln(&b, cfgSection.Render("Current config"))
	if result.Atlas.Initialized() {
		cfg := result.Atlas.Config
		fmt.Fprintf(&b, "  Project name: %s\n", cfg.Project.Name)
		fmt.Fprintf(&b, "  Project mode: %s\n", cfg.Project.Mode)
		fmt.Fprintf(&b, "  Storage mode: %s\n", cfg.Governance.StorageMode)
		fmt.Fprintf(&b, "  Memory provider: %s\n", cfg.Memory.Provider)
		fmt.Fprintf(&b, "  Atlas version: %s\n", cfg.Atlas.Version)
	} else if result.Atlas.State == workspace.AtlasStateInvalidConfig {
		fmt.Fprintln(&b, "  "+cfgMuted.Render("Config file is present but invalid."))
	} else {
		fmt.Fprintln(&b, "  "+cfgMuted.Render("No readable Atlas config."))
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, cfgSection.Render("Editing"))
	fmt.Fprintln(&b, "  "+cfgMuted.Render("Configuration editing is out of scope for this slice."))
	fmt.Fprintln(&b, "  "+cfgMuted.Render("This screen is read-only."))

	return strings.TrimRight(b.String(), "\n")
}
