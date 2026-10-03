package doctor

import (
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

// Report is the full doctor diagnostics result.
type Report struct {
	Checks []Check
}

// Evaluate builds diagnostics from a successful workspace discovery.
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

	if !result.Files.HasAtlasConfig {
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "atlas config",
			Message:  ".atlas/config.yaml not found",
		})
	}

	if !result.Files.HasAgentsFile {
		checks = append(checks, Check{
			Severity: SeverityWarn,
			Name:     "agents file",
			Message:  "AGENTS.md not found",
		})
	}

	for _, tool := range result.Tools {
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
