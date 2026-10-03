package doctor_test

import (
	"errors"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestEvaluate_WarnsForMissingAtlasMarkers(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "develop",
			Remotes: []workspace.GitRemote{
				{Name: "origin", URL: "git@example.com:demo.git"},
			},
		},
		Tools: []workspace.ToolInfo{
			{Name: "git", Available: true},
			{Name: "openspec", Available: false},
		},
	})

	assertHas(t, report, doctor.SeverityPass, "workspace", "discovery completed")
	assertHas(t, report, doctor.SeverityPass, "git", "repository detected")
	assertHas(t, report, doctor.SeverityPass, "branch", "develop")
	assertHas(t, report, doctor.SeverityWarn, "atlas config", ".atlas/config.yaml not found")
	assertHas(t, report, doctor.SeverityWarn, "agents file", "AGENTS.md not found")
	assertHas(t, report, doctor.SeverityPass, "tool git", "available")
	assertHas(t, report, doctor.SeverityWarn, "tool openspec", "unavailable")

	passed, warnings, failed := report.Counts()
	if failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
	if warnings < 3 {
		t.Fatalf("warnings = %d, want >= 3", warnings)
	}
	if passed < 3 {
		t.Fatalf("passed = %d, want >= 3", passed)
	}
	if report.Failed() {
		t.Fatal("report should not be failed")
	}
	if report.ResultLabel() != "ready with warnings" {
		t.Fatalf("result = %q, want ready with warnings", report.ResultLabel())
	}
}

func TestEvaluate_GitAndBranchWarnings(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo: false,
		},
		Files: workspace.FileInfo{
			HasAtlasConfig: true,
			HasAgentsFile:  true,
		},
	})

	assertHas(t, report, doctor.SeverityWarn, "git", "repository not detected")
	assertHas(t, report, doctor.SeverityWarn, "branch", "not detected")
	if hasCheck(report, "remotes") {
		t.Fatal("remotes check should be skipped when not a git repo")
	}
}

func TestEvaluate_WarnsWhenRepoHasNoRemotes(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "main",
		},
		Files: workspace.FileInfo{
			HasAtlasConfig: true,
			HasAgentsFile:  true,
		},
	})

	assertHas(t, report, doctor.SeverityWarn, "remotes", "none detected")
}

func TestEvaluate_ReadyWhenClean(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "develop",
			Remotes: []workspace.GitRemote{
				{Name: "origin", URL: "git@example.com:demo.git"},
			},
		},
		Files: workspace.FileInfo{
			HasAtlasConfig: true,
			HasAgentsFile:  true,
		},
		Tools: []workspace.ToolInfo{
			{Name: "git", Available: true},
		},
	})

	passed, warnings, failed := report.Counts()
	if warnings != 0 || failed != 0 {
		t.Fatalf("counts pass=%d warn=%d fail=%d", passed, warnings, failed)
	}
	if report.ResultLabel() != "ready" {
		t.Fatalf("result = %q, want ready", report.ResultLabel())
	}
}

func TestEvaluateWorkingDirectoryError(t *testing.T) {
	t.Parallel()

	report := doctor.EvaluateWorkingDirectoryError(errors.New("permission denied"))
	assertHas(t, report, doctor.SeverityFail, "workspace", "current directory cannot be resolved: permission denied")
	if !report.Failed() {
		t.Fatal("expected failed report")
	}
	if report.ResultLabel() != "not ready" {
		t.Fatalf("result = %q, want not ready", report.ResultLabel())
	}
}

func TestEvaluateDiscoveryError(t *testing.T) {
	t.Parallel()

	report := doctor.EvaluateDiscoveryError(errors.New("boom"))
	assertHas(t, report, doctor.SeverityFail, "workspace", "discovery failed: boom")
	if !report.Failed() {
		t.Fatal("expected failed report")
	}
}

func assertHas(t *testing.T, report doctor.Report, severity doctor.Severity, name, message string) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Severity == severity && check.Name == name && check.Message == message {
			return
		}
	}
	t.Fatalf("missing check %s %s: %s in %#v", severity, name, message, report.Checks)
}

func hasCheck(report doctor.Report, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name {
			return true
		}
	}
	return false
}
