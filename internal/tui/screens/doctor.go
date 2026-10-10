package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
)

var (
	docHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	docPass = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	docWarn = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	docFail = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	docInfo = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	docMute = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

var doctorSectionOrder = []string{
	"Workspace",
	"Git",
	"Atlas Configuration",
	"Atlas Runtime",
	"SDD",
	"Adapters",
	"Atlas Home",
	"Context",
	"Code Intelligence",
	"MCP / External Context",
}

// Doctor renders the deep diagnostic screen with grouped sections.
// Read-only: never repairs, rematerializes, or mutates Atlas Home.
func Doctor(report doctor.Report, result inspect.Inspection) string {
	var b strings.Builder
	fmt.Fprintln(&b, docHead.Render("Atlas Doctor"))
	fmt.Fprintln(&b, docMute.Render("Deep diagnostics · read-only"))
	fmt.Fprintln(&b)

	passed, warnings, failed := report.Counts()
	fmt.Fprintln(&b, docHead.Render("Overall Health"))
	fmt.Fprintf(&b, "  PASS: %s\n", docPass.Render(fmt.Sprintf("%d", passed)))
	fmt.Fprintf(&b, "  WARNING: %s\n", docWarn.Render(fmt.Sprintf("%d", warnings)))
	fmt.Fprintf(&b, "  ERROR: %s\n", docFail.Render(fmt.Sprintf("%d", failed)))
	fmt.Fprintf(&b, "  Result: %s\n\n", resultStyle(report).Render(report.ResultLabel()))

	grouped := map[string][]doctor.Check{}
	for _, check := range report.Checks {
		sec := doctorSectionFor(check.Name)
		grouped[sec] = append(grouped[sec], check)
	}

	for _, section := range doctorSectionOrder {
		checks := grouped[section]
		fmt.Fprintln(&b, docHead.Render(section))
		if section == "MCP / External Context" {
			writeDoctorMCP(&b, checks)
			fmt.Fprintln(&b)
			continue
		}
		if len(checks) == 0 {
			fmt.Fprintln(&b, "  "+docMute.Render("none"))
			fmt.Fprintln(&b)
			continue
		}
		for _, check := range checks {
			fmt.Fprintf(&b, "  %s %s: %s\n", doctorBadge(check.Severity), check.Name, check.Message)
		}
		fmt.Fprintln(&b)
	}

	return strings.TrimRight(b.String(), "\n")
}

func writeDoctorMCP(b *strings.Builder, checks []doctor.Check) {
	if len(checks) == 0 {
		fmt.Fprintln(b, "  "+docInfo.Render("INFO")+" mcp: n/a")
		return
	}
	for _, check := range checks {
		fmt.Fprintf(b, "  %s %s: %s\n", doctorBadge(check.Severity), check.Name, check.Message)
	}
}

func doctorSectionFor(name string) string {
	switch {
	case name == "workspace":
		return "Workspace"
	case name == "git" || name == "branch" || name == "remotes" || name == "remote default branch" ||
		name == "tool git" || name == "tool gh":
		return "Git"
	case strings.HasPrefix(name, "atlas config") || strings.HasPrefix(name, "atlas state") || name == "tool openspec":
		return "Atlas Configuration"
	case strings.HasPrefix(name, "atlas home"):
		return "Atlas Home"
	case name == "context graph" || name == "context economy":
		return "Context"
	case name == "code intelligence" || name == "codegraph" || strings.HasPrefix(name, "codegraph "):
		return "Code Intelligence"
	case strings.HasPrefix(name, "adapter") || name == "atlas agents" || name == "agent registry" ||
		name == "tool cursor" || name == "tool opencode":
		return "Adapters"
	case strings.HasPrefix(name, "mcp"):
		return "MCP / External Context"
	case name == "tool go":
		return "Workspace"
	case name == "sdd openspec contract":
		return "Atlas Runtime"
	case strings.HasPrefix(name, "sdd"):
		return "SDD"
	default:
		// runtime materialization, agents*, registry/manifest/lock, backups, forbidden
		return "Atlas Runtime"
	}
}

func doctorBadge(sev doctor.Severity) string {
	switch sev {
	case doctor.SeverityPass:
		return docPass.Render("PASS")
	case doctor.SeverityInfo:
		return docInfo.Render("INFO")
	case doctor.SeverityWarn:
		return docWarn.Render("WARNING")
	case doctor.SeverityFail:
		return docFail.Render("ERROR")
	default:
		return docInfo.Render("INFO")
	}
}

func resultStyle(report doctor.Report) lipgloss.Style {
	_, warnings, failed := report.Counts()
	switch {
	case failed > 0:
		return docFail
	case warnings > 0:
		return docWarn
	default:
		return docPass
	}
}
