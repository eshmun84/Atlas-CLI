package doctor

import (
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func evaluateGit(result inspect.Inspection) []Check {
	mode := sourceControlMode(result)
	gitTool := toolAvailable(result.Tools, "git")

	switch mode {
	case config.SourceControlNone, "":
		if !result.Runtime.ConfigLoads {
			return evaluateGitAdvisory(result, gitTool)
		}
		var checks []Check
		checks = append(checks, pass("source control mode", "none (Git repository not required)"))
		if result.Git.IsRepo {
			checks = append(checks, info("git", "repository present (optional under mode=none)"))
			checks = append(checks, evaluateGitDetails(result.Git)...)
		} else {
			checks = append(checks, pass("git", "repository not required"))
		}
		return checks

	case config.SourceControlGitLocal, config.SourceControlGitGitHub:
		var checks []Check
		checks = append(checks, pass("source control mode", mode))
		if !gitTool {
			checks = append(checks, fail("git executable", "git not found on PATH", RepairManual,
				"Install Git and ensure it is on PATH"))
			return checks
		}
		checks = append(checks, pass("git executable", "available"))
		if !result.Git.IsRepo {
			checks = append(checks, fail("git", "repository not detected but source_control requires Git", RepairManual,
				"Run Init with Git bootstrap when creating a new project, or initialize Git manually"))
			return checks
		}
		checks = append(checks, pass("git", "repository detected"))
		checks = append(checks, evaluateGitDetails(result.Git)...)
		return checks

	default:
		return evaluateGitAdvisory(result, gitTool)
	}
}

func evaluateGitAdvisory(result inspect.Inspection, gitTool bool) []Check {
	var checks []Check
	if !gitTool {
		checks = append(checks, warn("git executable", "git not found on PATH"))
	}
	if result.Git.IsRepo {
		checks = append(checks, pass("git", "repository detected"))
		checks = append(checks, evaluateGitDetails(result.Git)...)
	} else {
		checks = append(checks, warn("git", "repository not detected"))
		checks = append(checks, warn("branch", "not detected"))
	}
	return checks
}

func evaluateGitDetails(git project.GitInfo) []Check {
	var checks []Check
	branch := strings.TrimSpace(git.CurrentBranch)
	if branch != "" {
		checks = append(checks, pass("branch", branch))
	} else {
		checks = append(checks, warn("branch", "not detected"))
	}
	if len(git.Remotes) == 0 {
		checks = append(checks, warn("remotes", "none detected"))
	}
	checks = append(checks, evaluateRemoteDefaultBranch(git)...)
	return checks
}

func evaluateSigning(result inspect.Inspection) []Check {
	if !result.Runtime.ConfigLoads {
		return nil
	}
	required := result.Runtime.Document.SourceControl.SigningRequired
	var checks []Check
	if required {
		checks = append(checks, pass("signing policy", "Atlas signing_required=true"))
	} else {
		checks = append(checks, info("signing policy", "Atlas signing_required=false"))
	}

	if !result.Git.IsRepo {
		return checks
	}

	switch result.Git.CommitGPGSign {
	case "true":
		checks = append(checks, pass("git commit.gpgsign", "true"))
	case "false":
		if required {
			checks = append(checks, Check{
				Severity:          SeverityWarn,
				Name:              "git commit.gpgsign",
				Message:           "false while Atlas signing_required=true (explicit commit -S still required)",
				Repairability:     RepairDiagnostic,
				RecommendedAction: "Ensure commits use signing (-S); do not use --no-gpg-sign",
			})
		} else {
			checks = append(checks, info("git commit.gpgsign", "false"))
		}
	default:
		checks = append(checks, info("git commit.gpgsign", "unset"))
	}
	return checks
}

func sourceControlMode(result inspect.Inspection) string {
	if !result.Runtime.ConfigLoads {
		return ""
	}
	return strings.TrimSpace(result.Runtime.Document.SourceControl.Mode)
}

func toolAvailable(tools []project.ToolInfo, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return tool.Available
		}
	}
	return false
}
