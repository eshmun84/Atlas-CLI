package doctor_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func TestEvaluate_WarnsForMissingAtlasMarkers(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(inspect.Inspection{
		RootPath: "/tmp/demo",
		Git: project.GitInfo{
			IsRepo:        true,
			CurrentBranch: "develop",
			Remotes: []project.GitRemote{
				{Name: "origin", URL: "git@example.com:demo.git"},
			},
		},
		Runtime: runtime.Health{},
		Tools: []project.ToolInfo{
			{Name: "git", Available: true},
			{Name: "openspec", Available: false},
		},
	})

	assertHas(t, report, doctor.SeverityPass, "workspace", "discovery completed")
	assertHas(t, report, doctor.SeverityPass, "git", "repository detected")
	assertHas(t, report, doctor.SeverityPass, "branch", "develop")
	assertHas(t, report, doctor.SeverityWarn, "atlas config", ".atlas/config.yaml not found")
	assertHas(t, report, doctor.SeverityWarn, "agents file", "AGENTS.md not found")
	assertHas(t, report, doctor.SeverityPass, "forbidden artifacts", "AGENT.md, CLAUDE.md, GEMINI.md, .agents/, .claude/ absent (Atlas does not mutate these)")
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

	report := doctor.Evaluate(inspect.Inspection{
		RootPath: "/tmp/demo",
		Git: project.GitInfo{
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

	report := doctor.Evaluate(inspect.Inspection{
		RootPath: "/tmp/demo",
		Git: project.GitInfo{
			IsRepo:        true,
			CurrentBranch: "main",
		},
		Runtime: healthyRuntime(),
	})

	assertHas(t, report, doctor.SeverityWarn, "remotes", "none detected")
	if hasCheck(report, "remote default branch") {
		t.Fatal("remote default branch check should be skipped when no default remote")
	}
}

func TestEvaluate_WarnsWhenRemoteDefaultBranchUnknownLocally(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(inspect.Inspection{
		RootPath: "/tmp/demo",
		Git: project.GitInfo{
			IsRepo:        true,
			CurrentBranch: "feature/x",
			DefaultRemote: "origin",
			Remotes: []project.GitRemote{
				{Name: "origin", URL: "https://example.com/demo.git"},
			},
		},
		Runtime: healthyRuntime(),
	})

	assertHas(t, report, doctor.SeverityWarn, "remote default branch",
		"unknown locally (refs/remotes/origin/HEAD missing); run: git remote set-head origin -a")
}

func TestEvaluate_PassesWhenRemoteDefaultBranchKnown(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(inspect.Inspection{
		RootPath: "/tmp/demo",
		Git: project.GitInfo{
			IsRepo:        true,
			CurrentBranch: "feature/x",
			DefaultRemote: "origin",
			DefaultBranch: "main",
			Remotes: []project.GitRemote{
				{Name: "origin", URL: "https://example.com/demo.git"},
			},
		},
		Runtime: healthyRuntime(),
	})

	assertHas(t, report, doctor.SeverityPass, "remote default branch", "main")
}

func TestEvaluate_ReadyWhenClean(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(inspect.Inspection{
		RootPath: "/tmp/demo",
		Git: project.GitInfo{
			IsRepo:        true,
			CurrentBranch: "develop",
			DefaultRemote: "origin",
			DefaultBranch: "main",
			Remotes: []project.GitRemote{
				{Name: "origin", URL: "git@example.com:demo.git"},
			},
		},
		Runtime: healthyRuntime(),
		Tools: []project.ToolInfo{
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
	rt.ExpectedProjections = []runtime.ProjectionStatus{
		{Adapter: "cursor", Path: config.FileCursorAtlasMDC, Present: false},
	}

	report := doctor.Evaluate(inspect.Inspection{
		RootPath: "/tmp/demo",
		Git: project.GitInfo{
			IsRepo:        true,
			CurrentBranch: "develop",
			Remotes:       []project.GitRemote{{Name: "origin", URL: "x"}},
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

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "agents markers", "AGENTS.md markers incomplete")
}

func TestEvaluate_MissingBackupsPassWhenInitialized(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.BackupsDirExists = false
	rt.LegacyBackupsDirExists = false
	rt.HomeProject.BackupsPresent = false

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityPass, "backups directory", "no backups yet (created on repair/init backup)")
	if report.Failed() {
		t.Fatal("missing backups must not fail")
	}
}

func TestEvaluate_ForbiddenArtifactsWarnWhenPresent(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.ForbiddenArtifacts = []runtime.ForbiddenArtifactStatus{
		{Path: "CLAUDE.md", Present: true},
		{Path: "GEMINI.md", Present: false},
		{Path: ".agents", Present: false},
		{Path: ".claude", Present: false},
	}

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityWarn, "forbidden artifact CLAUDE.md", "present but not required or expected")
	if hasNamed(report, "forbidden artifacts") {
		t.Fatal("aggregate pass should be omitted when any forbidden artifact is present")
	}
}

func TestEvaluate_InvalidConfigFailsWithoutPanic(t *testing.T) {
	t.Parallel()

	rt := runtime.Health{
		Initialized:  false,
		ConfigExists: true,
		ConfigLoads:  false,
		ConfigError:  "invalid config: adapters.selected[0] \"Cursor\" is invalid",
		StateExists:  true,
		StateLoads:   true,
		State:        config.StateDocument{RuntimeMaterialized: false},
		ForbiddenArtifacts: []runtime.ForbiddenArtifactStatus{
			{Path: "CLAUDE.md", Present: false},
			{Path: "GEMINI.md", Present: false},
			{Path: ".agents", Present: false},
			{Path: ".claude", Present: false},
		},
	}

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "atlas config loads", "failed to load: invalid config: adapters.selected[0] \"Cursor\" is invalid")
	if !report.Failed() {
		t.Fatal("invalid config must fail")
	}
}

func TestEvaluate_MissingStateNeutralForNonAtlas(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(inspect.Inspection{
		Runtime: runtime.Health{
			ForbiddenArtifacts: []runtime.ForbiddenArtifactStatus{
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

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "atlas state", ".atlas/state.yaml missing for initialized project")
}

func TestEvaluate_ContextGraphEnabledWithoutEngineIsPass(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.ContextGraphEnabled = true

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityPass, "context graph", "Atlas Context Graph preference (enabled); engine NOT IMPLEMENTED")
	assertHas(t, report, doctor.SeverityInfo, "codegraph", "optional provider unavailable")
	for _, check := range report.Checks {
		if check.Name == "context graph" && check.Severity != doctor.SeverityPass {
			t.Fatalf("context graph must not warn/fail: %#v", check)
		}
		if check.Name == "codegraph" && check.Severity == doctor.SeverityFail {
			t.Fatalf("optional Code Intelligence must not fail Doctor: %#v", check)
		}
	}
}

func TestEvaluate_CodeIntelligenceAvailablePass(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.CodeIntelligence = codeintel.Snapshot{
		Applicable: true,
		Provider:   codeintel.ProviderCodeGraph,
		State:      codeintel.StateAvailable,
		Version:    "3.17.0",
	}

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityPass, "codegraph", "available · 3.17.0 · graph missing")
	if report.Failed() {
		t.Fatal("available Code Intelligence must not fail Doctor")
	}
}

func TestEvaluate_CodeIntelligenceGraphAbsentStillPass(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.CodeIntelligence = codeintel.Snapshot{
		Provider:     codeintel.ProviderCodeGraph,
		State:        codeintel.StateAvailable,
		Version:      "3.17.0",
		GraphPresent: false,
	}

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityPass, "codegraph", "available · 3.17.0 · graph missing")
	for _, check := range report.Checks {
		if check.Name == "codegraph" && check.Severity != doctor.SeverityPass {
			t.Fatalf("graph missing must stay PASS, got %#v", check)
		}
	}
}

func TestEvaluate_CodeIntelligenceOptionalUnavailableInfo(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(inspect.Inspection{
		Runtime: runtime.Health{
			CodeIntelligence: codeintel.Snapshot{
				Provider: codeintel.ProviderCodeGraph,
				State:    codeintel.StateUnavailable,
				Message:  "CodeGraph executable not found on PATH",
			},
			ForbiddenArtifacts: []runtime.ForbiddenArtifactStatus{
				{Path: "CLAUDE.md", Present: false},
				{Path: "GEMINI.md", Present: false},
				{Path: ".agents", Present: false},
				{Path: ".claude", Present: false},
			},
		},
	})
	assertHas(t, report, doctor.SeverityInfo, "codegraph", "CodeGraph executable not found on PATH")
	if report.Failed() {
		t.Fatal("optional unavailable provider must not fail Doctor")
	}
	_, warnings, failed := report.Counts()
	if failed != 0 {
		t.Fatalf("failed=%d want 0", failed)
	}
	if warnings != 0 && hasNamedSeverity(report, "codegraph", doctor.SeverityWarn) {
		t.Fatal("optional unavailable must not be WARNING")
	}
}

func TestEvaluate_CodeIntelligenceWithoutConfigStillVisible(t *testing.T) {
	t.Parallel()

	report := doctor.Evaluate(inspect.Inspection{
		Runtime: runtime.Health{
			ConfigExists: false,
			ConfigLoads:  false,
			CodeIntelligence: codeintel.Snapshot{
				Provider: codeintel.ProviderCodeGraph,
				State:    codeintel.StateAvailable,
				Version:  "3.17.0",
			},
			ForbiddenArtifacts: []runtime.ForbiddenArtifactStatus{
				{Path: "CLAUDE.md", Present: false},
				{Path: "GEMINI.md", Present: false},
				{Path: ".agents", Present: false},
				{Path: ".claude", Present: false},
			},
		},
	})
	assertHas(t, report, doctor.SeverityPass, "codegraph", "available · 3.17.0 · graph missing")
}

func TestEvaluate_CodeIntelligenceIncompatibleWarn(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.CodeIntelligence = codeintel.Snapshot{
		Provider: codeintel.ProviderCodeGraph,
		State:    codeintel.StateIncompatible,
		Version:  "2.9.0",
		Message:  "CodeGraph 2.9.0 is outside the supported major 3 range",
	}

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityWarn, "codegraph",
		"incompatible · CodeGraph 2.9.0 is outside the supported major 3 range")
}

func TestEvaluate_CodeIntelligenceMissingExpectedWarn(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.CodeIntelligence = codeintel.Snapshot{
		Provider: codeintel.ProviderCodeGraph,
		State:    codeintel.StateMissing,
		Message:  "provider missing after prior configuration",
	}

	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityWarn, "codegraph", "provider missing after prior configuration")
}

func TestEvaluate_CodeIntelligenceUsesSnapshotOnlyNoFSMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	marker := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, root)

	rt := healthyRuntime()
	rt.CodeIntelligence = codeintel.Snapshot{
		Provider: codeintel.ProviderCodeGraph,
		State:    codeintel.StateAvailable,
		Version:  "3.17.0",
	}
	report := doctor.Evaluate(inspect.Inspection{
		RootPath: root,
		Runtime:  rt,
	})
	assertHas(t, report, doctor.SeverityPass, "codegraph", "available · 3.17.0 · graph missing")
	assertDirUnchanged(t, root, before)

	if _, err := os.Stat(filepath.Join(root, ".codegraph")); !os.IsNotExist(err) {
		t.Fatal("Doctor must not create .codegraph")
	}
	if _, err := os.Stat(filepath.Join(root, "graph.db")); !os.IsNotExist(err) {
		t.Fatal("Doctor must not create graph.db")
	}
}

func TestDoctorScreen_RendersCodeIntelligenceSection(t *testing.T) {
	t.Parallel()

	rt := healthyRuntime()
	rt.CodeIntelligence = codeintel.Snapshot{
		Provider: codeintel.ProviderCodeGraph,
		State:    codeintel.StateAvailable,
		Version:  "3.17.0",
	}
	result := inspect.Inspection{Runtime: rt}
	report := doctor.Evaluate(result)
	view := stripANSIDoctor(screens.Doctor(report, result))

	if !strings.Contains(view, "Code Intelligence") {
		t.Fatalf("doctor view missing Code Intelligence section:\n%s", view)
	}
	if !strings.Contains(view, "PASS codegraph: available · 3.17.0 · graph missing") {
		t.Fatalf("doctor view missing expected Code Intelligence line:\n%s", view)
	}
	// Must not bury the check under Context as a silent omission.
	idxCI := strings.Index(view, "Code Intelligence")
	idxMCP := strings.Index(view, "MCP / External Context")
	if idxMCP < 0 {
		idxMCP = strings.Index(view, "\nMCP\n")
	}
	if idxCI < 0 || idxMCP < 0 || idxCI > idxMCP {
		t.Fatalf("Code Intelligence section order unexpected:\n%s", view)
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

func healthyRuntime() runtime.Health {
	agents := make([]runtime.AgentFileStatus, 0, len(config.AtlasAgentRuntimePaths([]string{"cursor"})))
	for _, path := range config.AtlasAgentRuntimePaths([]string{"cursor"}) {
		agents = append(agents, runtime.AgentFileStatus{
			Adapter: "cursor",
			Path:    path,
			Present: true,
			Matches: true,
		})
	}
	return runtime.Health{
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
		ExpectedProjections:    []runtime.ProjectionStatus{{Adapter: "cursor", Path: config.FileCursorAtlasMDC, Present: true}},
		ExpectedAgents:         agents,
		AgentRegistryPresent:   true,
		AgentRegistryMatches:   true,
		RuntimeManifestPresent: true,
		RuntimeManifestMatches: true,
		AssetsLockPresent:      true,
		AssetsLockMatches:      true,
		DependsOnSDDContract:   true,
		SDDContractPresent:     true,
		SDDContractMatches:     true,
		Home: home.Status{
			Path:           "/tmp/atlas-home-test",
			Exists:         true,
			Writable:       true,
			LayoutComplete: true,
			StatePresent:   true,
			StateLoads:     true,
			AssetCount:     len(home.BundledAssets()),
		},
		HomeProject: home.ProjectStatus{
			ProjectID:      "demo-test",
			Present:        true,
			BackupsPresent: true,
		},
		ContextGraphEnabled:  true,
		ContextGraphReadable: true,
		BackupsDirExists:     true,
		ForbiddenArtifacts: []runtime.ForbiddenArtifactStatus{
			{Path: "CLAUDE.md", Present: false},
			{Path: "GEMINI.md", Present: false},
			{Path: ".agents", Present: false},
			{Path: ".claude", Present: false},
		},
	}
}

func TestEvaluate_HomePathReported(t *testing.T) {
	t.Parallel()
	rt := healthyRuntime()
	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityPass, "atlas home path", rt.Home.Path)
	assertHas(t, report, doctor.SeverityPass, "atlas home assets", fmt.Sprintf("%d mirrored", len(home.BundledAssets())))
}

func TestEvaluate_HomeAssetErrorsFail(t *testing.T) {
	t.Parallel()
	rt := healthyRuntime()
	rt.Home.AssetErrors = []string{"agents/runtime/atlas-worker.md: injected read failure"}
	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "atlas home assets", "1 embedded integrity error(s)")
	if !report.Failed() {
		t.Fatal("doctor must not report healthy with asset integrity errors")
	}
}

func TestEvaluate_RuntimeRenderErrorNotDrift(t *testing.T) {
	t.Parallel()
	rt := healthyRuntime()
	rt.RuntimeManifestMatches = false
	rt.RuntimeManifestRenderError = "injected canonical render failure"
	rt.AssetsLockMatches = false
	rt.AssetsLockRenderError = "injected lock render failure"
	if len(rt.ExpectedAgents) > 0 {
		rt.ExpectedAgents[0].Matches = false
		rt.ExpectedAgents[0].RenderError = "injected agent render failure"
	}
	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "runtime manifest", "canonical render failed")
	assertHas(t, report, doctor.SeverityFail, "assets lock", "canonical render failed")
	assertHas(t, report, doctor.SeverityFail, "atlas agents", "1 Atlas agent canonical render failure(s)")
	for _, c := range report.Checks {
		if c.Name == "runtime manifest" && c.Message == "content drifted" {
			t.Fatal("render failure must not be labeled only as drift")
		}
	}
	if !report.Failed() {
		t.Fatal("doctor must FAIL on render integrity errors")
	}
}

func TestEvaluate_RuntimeDriftStillDrift(t *testing.T) {
	t.Parallel()
	rt := healthyRuntime()
	rt.RuntimeManifestMatches = false
	rt.RuntimeManifestRenderError = ""
	report := doctor.Evaluate(inspect.Inspection{Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "runtime manifest", "content drifted")
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

func hasNamedSeverity(report doctor.Report, name string, severity doctor.Severity) bool {
	for _, check := range report.Checks {
		if check.Name == name && check.Severity == severity {
			return true
		}
	}
	return false
}

func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertDirUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotDir(t, root)
	if len(before) != len(after) {
		t.Fatalf("directory tree size changed: before=%d after=%d", len(before), len(after))
	}
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("path mutated: %s", path)
		}
	}
}

func stripANSIDoctor(s string) string {
	var b strings.Builder
	inESC := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			inESC = true
			continue
		}
		if inESC {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inESC = false
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
