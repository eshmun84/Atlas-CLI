package doctor_test

import (
	"errors"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
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
		Runtime: workspace.RuntimeHealth{},
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
	assertHas(t, report, doctor.SeverityPass, "forbidden artifacts", "CLAUDE.md, GEMINI.md, .agents/, .claude/ absent")
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
		Runtime: healthyRuntime(),
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
		Runtime: healthyRuntime(),
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
		Runtime: healthyRuntime(),
		Tools: []workspace.ToolInfo{
			{Name: "git", Available: true},
		},
	})

	passed, warnings, failed := report.Counts()
	if warnings != 0 || failed != 0 {
		t.Fatalf("counts pass=%d warn=%d fail=%d checks=%#v", passed, warnings, failed, report.Checks)
	}
	if report.ResultLabel() != "ready" {
		t.Fatalf("result = %q, want ready", report.ResultLabel())
	}
}

func TestEvaluate_RuntimeDriftFailures(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.AgentsExists = false
	rt.AgentsMarkers = config.AgentsMarkers{}
	rt.Warnings = []string{"runtime_materialized=true but AGENTS.md is missing"}
	rt.ExpectedProjections = []workspace.ProjectionStatus{
		{Adapter: "cursor", Path: config.FileCursorAtlasMDC, Present: false},
	}

	report := doctor.Evaluate(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "develop",
			Remotes:       []workspace.GitRemote{{Name: "origin", URL: "x"}},
		},
		Runtime: rt,
	})

	assertHas(t, report, doctor.SeverityFail, "runtime materialization", "runtime_materialized=true but AGENTS.md is missing")
	assertHas(t, report, doctor.SeverityFail, "agents file", "AGENTS.md required when runtime_materialized=true")
	assertHas(t, report, doctor.SeverityFail, "adapter projection cursor", ".cursor/rules/atlas.mdc missing")
	if !report.Failed() {
		t.Fatal("expected failed report")
	}
}

func TestEvaluate_IncompleteMarkersFailWhenMaterialized(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.AgentsMarkers = config.AgentsMarkers{BaseBegin: true, BaseEnd: true}

	report := doctor.Evaluate(workspace.DiscoveryResult{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "agents markers", "AGENTS.md markers incomplete")
}

func TestEvaluate_MissingBackupsWarnsWhenInitialized(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.BackupsDirExists = false

	report := doctor.Evaluate(workspace.DiscoveryResult{Runtime: rt})
	assertHas(t, report, doctor.SeverityWarn, "backups directory", ".atlas/backups missing for initialized project")
	if report.Failed() {
		t.Fatal("missing backups must not fail")
	}
}

func TestEvaluate_ForbiddenArtifactsWarnWhenPresent(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.ForbiddenArtifacts = []workspace.ForbiddenArtifactStatus{
		{Path: "CLAUDE.md", Present: true},
		{Path: "GEMINI.md", Present: false},
		{Path: ".agents", Present: false},
		{Path: ".claude", Present: false},
	}

	report := doctor.Evaluate(workspace.DiscoveryResult{Runtime: rt})
	assertHas(t, report, doctor.SeverityWarn, "forbidden artifact CLAUDE.md", "present but not required or expected")
	if hasNamed(report, "forbidden artifacts") {
		t.Fatal("aggregate pass should be omitted when any forbidden artifact is present")
	}
}

func TestEvaluate_InvalidConfigFailsWithoutPanic(t *testing.T) {
	t.Parallel()

	rt := workspace.RuntimeHealth{
		Initialized:  false,
		ConfigExists: true,
		ConfigLoads:  false,
		ConfigError:  "invalid config: adapters.selected[0] \"Cursor\" is invalid",
		StateExists:  true,
		StateLoads:   true,
		State:        config.StateDocument{RuntimeMaterialized: false},
		ForbiddenArtifacts: []workspace.ForbiddenArtifactStatus{
			{Path: "CLAUDE.md", Present: false},
			{Path: "GEMINI.md", Present: false},
			{Path: ".agents", Present: false},
			{Path: ".claude", Present: false},
		},
	}

	report := doctor.Evaluate(workspace.DiscoveryResult{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "atlas config loads", "failed to load: invalid config: adapters.selected[0] \"Cursor\" is invalid")
	if !report.Failed() {
		t.Fatal("invalid config must fail")
	}
}

func TestEvaluate_MissingStateNeutralForNonAtlas(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(workspace.DiscoveryResult{
		Runtime: workspace.RuntimeHealth{
			ForbiddenArtifacts: []workspace.ForbiddenArtifactStatus{
				{Path: "CLAUDE.md", Present: false},
				{Path: "GEMINI.md", Present: false},
				{Path: ".agents", Present: false},
				{Path: ".claude", Present: false},
			},
		},
	})
	if hasNamed(report, "atlas state") {
		t.Fatal("non-Atlas projects must not report missing state")
	}
}

func TestEvaluate_MissingStateFailsWhenInitialized(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.StateExists = false
	rt.StateLoads = false
	rt.State = config.StateDocument{}
	rt.RuntimeMaterialized = false

	report := doctor.Evaluate(workspace.DiscoveryResult{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "atlas state", ".atlas/state.yaml missing for initialized project")
}

func TestEvaluate_ContextGraphEnabledWithoutEngineIsPass(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.ContextGraphEnabled = true

	report := doctor.Evaluate(workspace.DiscoveryResult{Runtime: rt})
	assertHas(t, report, doctor.SeverityPass, "context graph", "preference readable (enabled); engine absence expected")
	for _, check := range report.Checks {
		if check.Name == "context graph" && check.Severity != doctor.SeverityPass {
			t.Fatalf("context graph must not warn/fail: %#v", check)
		}
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

func healthyRuntime() workspace.RuntimeHealth {
	agents := make([]workspace.AgentFileStatus, 0, len(config.AtlasAgentRuntimePaths([]string{"cursor"})))
	for _, path := range config.AtlasAgentRuntimePaths([]string{"cursor"}) {
		agents = append(agents, workspace.AgentFileStatus{
			Adapter: "cursor",
			Path:    path,
			Present: true,
			Matches: true,
		})
	}
	return workspace.RuntimeHealth{
		Initialized:         true,
		ConfigExists:        true,
		ConfigLoads:         true,
		StateExists:         true,
		StateLoads:          true,
		State:               config.StateDocument{RuntimeMaterialized: true, Initialized: true},
		RuntimeMaterialized: true,
		AgentsExists:        true,
		AgentsMarkers: config.AgentsMarkers{
			BaseBegin: true,
			BaseEnd:   true,
			UserBegin: true,
			UserEnd:   true,
			AdapterBlocks: map[string]bool{
				"cursor": true,
			},
			FoundAdapters: []string{"cursor"},
		},
		SelectedAdapters:       []string{"cursor"},
		ExpectedProjections:    []workspace.ProjectionStatus{{Adapter: "cursor", Path: config.FileCursorAtlasMDC, Present: true}},
		ExpectedAgents:         agents,
		AgentRegistryPresent:   true,
		AgentRegistryMatches:   true,
		RuntimeManifestPresent: true,
		RuntimeManifestMatches: true,
		AssetsLockPresent:      true,
		AssetsLockMatches:      true,
		ContextGraphEnabled:    true,
		ContextGraphReadable:   true,
		BackupsDirExists:       true,
		ForbiddenArtifacts: []workspace.ForbiddenArtifactStatus{
			{Path: "CLAUDE.md", Present: false},
			{Path: "GEMINI.md", Present: false},
			{Path: ".agents", Present: false},
			{Path: ".claude", Present: false},
		},
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
	return hasNamed(report, name)
}

func hasNamed(report doctor.Report, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name {
			return true
		}
	}
	return false
}
