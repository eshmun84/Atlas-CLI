package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
)

var (
	docHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	docPass = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	docWarn = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	docFail = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
)

// Doctor renders the diagnostics screen with color-coded badges.
func Doctor(report doctor.Report) string {
	var b strings.Builder
	fmt.Fprintln(&b, docHead.Render("Atlas Doctor"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, docHead.Render("Checks"))
	if len(report.Checks) == 0 {
		fmt.Fprintln(&b, "  none")
	} else {
		for _, check := range report.Checks {
			fmt.Fprintf(&b, "  %s %s: %s\n", badge(check.Severity), check.Name, check.Message)
		}
	}

	passed, warnings, failed := report.Counts()
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, docHead.Render("Summary"))
	fmt.Fprintf(&b, "  Passed:   %s\n", docPass.Render(fmt.Sprintf("%d", passed)))
	fmt.Fprintf(&b, "  Warnings: %s\n", docWarn.Render(fmt.Sprintf("%d", warnings)))
	fmt.Fprintf(&b, "  Failed:   %s\n", docFail.Render(fmt.Sprintf("%d", failed)))
	fmt.Fprintf(&b, "  Result:   %s\n", resultStyle(report).Render(report.ResultLabel()))
	return strings.TrimRight(b.String(), "\n")
}

func badge(sev doctor.Severity) string {
	switch sev {
	case doctor.SeverityPass:
		return docPass.Render("PASS")
	case doctor.SeverityWarn:
		return docWarn.Render("WARN")
	case doctor.SeverityFail:
		return docFail.Render("FAIL")
	default:
		return string(sev)
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
