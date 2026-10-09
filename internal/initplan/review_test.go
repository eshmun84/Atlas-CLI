package initplan_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

func TestBuildReview_NoArtifacts(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:           "Atlas-CLI",
		ProjectMode:           "new",
		ToolCursorAvailable:   true,
		ToolOpenCodeAvailable: true,
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") || !draft.ToggleMulti("adapters.selected", "opencode") {
		t.Fatal("expected adapter selection")
	}
	plan := initplan.BuildReview(initplan.ReviewInput{
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
	})

	if plan.ProjectName != "Atlas-CLI" {
		t.Fatalf("name = %q", plan.ProjectName)
	}
	if plan.ProjectModeLabel != "New project" {
		t.Fatalf("mode label = %q", plan.ProjectModeLabel)
	}
	if plan.Workflow != "SDD" {
		t.Fatalf("workflow = %q", plan.Workflow)
	}
	if plan.SpecEngine != "OpenSpec" {
		t.Fatalf("spec engine = %q", plan.SpecEngine)
	}
	if plan.Adapters != "Cursor, OpenCode" {
		t.Fatalf("adapters = %q", plan.Adapters)
	}
	if plan.DeliveryPlatform != "None" {
		t.Fatalf("delivery platform = %q", plan.DeliveryPlatform)
	}
	if plan.DeliveryAssistance != "Disabled" {
		t.Fatalf("delivery assistance = %q", plan.DeliveryAssistance)
	}
	if plan.GovernanceStorage != "Local only" {
		t.Fatalf("governance = %q", plan.GovernanceStorage)
	}
	if plan.MCPCount != 0 {
		t.Fatalf("mcp count = %d", plan.MCPCount)
	}
	if plan.PreviewOnly {
		t.Fatal("apply is enabled; plan must not be preview-only")
	}
	if plan.ConfigApplyOnly {
		t.Fatal("runtime materialization is enabled; plan must not be config-apply-only")
	}
	if len(plan.ExistingArtifacts) != 0 {
		t.Fatalf("artifacts = %#v", plan.ExistingArtifacts)
	}
	if len(plan.Backups) != 0 || len(plan.Replacements) != 0 {
		t.Fatalf("unexpected backups/replacements: %#v %#v", plan.Backups, plan.Replacements)
	}
	if !strings.Contains(plan.GitSafetyStatement, "No repository, branch, commit, push, pull request, merge or remote operation") {
		t.Fatalf("git safety = %q", plan.GitSafetyStatement)
	}
	if len(plan.DeliveryPolicy) < 6 {
		t.Fatalf("delivery policy = %#v", plan.DeliveryPolicy)
	}
	if !stringsContainsAll(plan, []string{
		"Existing project source files are preserved.",
		plan.GitSafetyStatement,
		"Secrets and credentials are not stored.",
		"Claude Code and Codex adapters are not materialized.",
		"Apply is the only mutation step. Status and Doctor remain read-only.",
		"Apply writes Atlas configuration under .atlas/ and materializes compact runtime gateway files.",
		"Apply creates/updates Atlas Home (ATLAS_HOME or ~/.atlas) and mirrors bundled Atlas-owned assets.",
		"After Init, Configure Apply saves .atlas/config.yaml and reconciles MCP projections",
		"MCP desired state is stored in Atlas config and materialized into selected agent MCP configs",
		"Runtime conflicts block Init and require manual cleanup in this slice.",
		"Context Economy v0 is a separate explicit flow. CodeGraph is an optional externally installed Code Intelligence provider",
		"Atlas Context Graph is NOT IMPLEMENTED.",
		"Init performs no Git operations.",
	}) {
		t.Fatalf("missing preserve/warning copy: %#v %#v", plan.Preservations, plan.Warnings)
	}
	for _, banned := range []string{
		"Configure Apply is config-only",
		"preference/config only",
	} {
		if stringsContainsAll(plan, []string{banned}) {
			t.Fatalf("stale MCP/Configure wording still present (%q): %#v %#v", banned, plan.Preservations, plan.Warnings)
		}
	}
	if !strings.Contains(plan.GovernanceNote, "Local only") {
		t.Fatalf("governance note = %q", plan.GovernanceNote)
	}
	if len(plan.HomeWrites) == 0 {
		t.Fatal("expected Atlas Home writes in plan")
	}

	wantCreates := []string{
		".atlas/config.yaml",
		".atlas/local.yaml",
		".atlas/state.yaml",
		".atlas/assets.lock.yaml",
		".atlas/agent-registry.md",
		".atlas/runtime-manifest.yaml",
		".atlas/contracts/sdd-openspec.md",
		"AGENTS.md",
		".cursor/rules/atlas.mdc",
		".opencode/atlas.md",
		".cursor/agents/atlas-orchestrator.md",
		".opencode/agents/atlas-orchestrator.md",
	}
	assertCreatePaths(t, plan, wantCreates)
	for _, file := range plan.Creates {
		if file.Status != "create/update on Apply" {
			t.Fatalf("status for %s = %q", file.Path, file.Status)
		}
	}
}

func TestBuildReview_WithArtifactsAndMCP(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:    "demo",
		ProjectMode:    "existing",
		CursorDetected: true,
	})
	if !draft.SelectOption("source_control.mode", "git_github") {
		t.Fatal("set github")
	}
	if !draft.SelectOption("source_control.governance_storage", "versioned") {
		t.Fatal("set versioned")
	}
	mcp := config.EmptyMCPDraft()
	if !mcp.EnableBuiltin(config.MCPBuiltinJira) {
		t.Fatal("select jira")
	}
	if _, err := mcp.AddCustom("Jira Main", config.MCPTransportStdio, "npx", "-y demo-mcp", ""); err != nil {
		t.Fatalf("add mcp: %v", err)
	}
	mcp.ToggleCustom(0)

	plan := initplan.BuildReview(initplan.ReviewInput{
		Root:      root,
		Draft:     draft,
		MCP:       mcp,
		Artifacts: []string{"AGENTS.md", ".cursor"},
	})

	if len(plan.ExistingArtifacts) != 2 {
		t.Fatalf("artifacts = %#v", plan.ExistingArtifacts)
	}
	if len(plan.MCPEntries) != 2 {
		t.Fatalf("mcp = %#v", plan.MCPEntries)
	}
	if plan.MCPEntries[0].Name != "Jira" || plan.MCPEntries[0].Kind != "built-in" {
		t.Fatalf("builtin mcp = %#v", plan.MCPEntries[0])
	}
	if plan.MCPEntries[1].Name != "Jira Main" || plan.MCPEntries[1].Kind != "custom" || plan.MCPEntries[1].Transport != "stdio" {
		t.Fatalf("custom mcp = %#v", plan.MCPEntries[1])
	}
	if plan.MCPEntries[0].Status == "" || plan.MCPEntries[1].Status == "" {
		t.Fatalf("mcp status = %#v", plan.MCPEntries)
	}
	if plan.GovernanceNote == "" || plan.GovernanceStorage != "Versioned" {
		t.Fatalf("governance note=%q storage=%q", plan.GovernanceNote, plan.GovernanceStorage)
	}

	foundAgents, foundCursor, foundManifest, foundReplaceAgents, foundReplaceCursor := false, false, false, false, false
	for _, backup := range plan.Backups {
		if backup.Path == "projects/<project-id>/backups/<timestamp>/AGENTS.md" {
			foundAgents = true
		}
		if backup.Path == "projects/<project-id>/backups/<timestamp>/.cursor/rules/atlas.mdc" {
			foundCursor = true
		}
		if backup.Path == "projects/<project-id>/backups/<timestamp>/manifest.json" {
			foundManifest = true
		}
	}
	for _, repl := range plan.Replacements {
		if repl.Path == "AGENTS.md" {
			foundReplaceAgents = true
		}
		if repl.Path == ".cursor/rules/atlas.mdc" {
			foundReplaceCursor = true
		}
	}
	if !foundAgents || !foundCursor || !foundManifest || !foundReplaceAgents || !foundReplaceCursor {
		t.Fatalf("backup/replace missing: backups=%#v replacements=%#v", plan.Backups, plan.Replacements)
	}
}

func TestBuildReview_DoesNotWriteFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "new",
	})
	_ = initplan.BuildReview(initplan.ReviewInput{Draft: draft, MCP: config.EmptyMCPDraft()})
	assertNoAtlasOrAgents(t, root)
}

func assertCreatePaths(t *testing.T, plan initplan.MaterializationPlan, want []string) {
	t.Helper()
	got := make(map[string]bool, len(plan.Creates))
	for _, file := range plan.Creates {
		got[file.Path] = true
	}
	for _, path := range want {
		if !got[path] {
			t.Fatalf("missing create %q in %#v", path, plan.Creates)
		}
	}
}

func assertNoAtlasOrAgents(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not exist, stat err = %v", err)
	}
}

func TestBuildReview_DocsScaffoldProjectWrite(t *testing.T) {
	t.Parallel()

	off := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	offPlan := initplan.BuildReview(initplan.ReviewInput{Draft: off, MCP: config.EmptyMCPDraft()})
	for _, file := range offPlan.Creates {
		if file.Path == config.FileProjectDocsREADME {
			t.Fatal("docs scaffold must be off by default in Review")
		}
	}

	on := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:  "demo",
		ProjectMode:  "existing",
		DocsScaffold: true,
	})
	onPlan := initplan.BuildReview(initplan.ReviewInput{Draft: on, MCP: config.EmptyMCPDraft()})
	found := false
	for _, file := range onPlan.Creates {
		if file.Path == config.FileProjectDocsREADME {
			found = true
			if file.Kind != "project-docs" {
				t.Fatalf("docs kind = %q, want project-docs", file.Kind)
			}
			if !strings.Contains(file.Status, "create once") {
				t.Fatalf("docs status = %q", file.Status)
			}
		}
	}
	if !found {
		t.Fatalf("expected docs scaffold in project writes: %#v", onPlan.Creates)
	}
}

func TestBuildReview_HomeDataPresentAbsentUnknown(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "presence-demo",
		ProjectMode: "new",
	})

	rootAbsent := t.TempDir()
	planAbsent := initplan.BuildReview(initplan.ReviewInput{Root: rootAbsent, Draft: draft})
	if planAbsent.HomeDataPresence != "absent" || planAbsent.HomeDataDetected {
		t.Fatalf("absent: %#v", planAbsent)
	}

	rootPresent := t.TempDir()
	id, err := home.ProjectID(rootPresent, "presence-demo")
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(homeDir, "projects", id)
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "marker"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	planPresent := initplan.BuildReview(initplan.ReviewInput{Root: rootPresent, Draft: draft})
	if planPresent.HomeDataPresence != "present" || !planPresent.HomeDataDetected {
		t.Fatalf("present: %#v", planPresent)
	}

	rootUnknown := t.TempDir()
	idUnknown, err := home.ProjectID(rootUnknown, "presence-demo")
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(homeDir, "projects", idUnknown)
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	planUnknown := initplan.BuildReview(initplan.ReviewInput{Root: rootUnknown, Draft: draft})
	if planUnknown.HomeDataPresence != "unknown" || planUnknown.HomeDataDetected {
		t.Fatalf("unknown: %#v", planUnknown)
	}
	if planUnknown.HomeDataError == "" {
		t.Fatal("expected HomeDataError for inspection failure")
	}
}

func stringsContainsAll(plan initplan.MaterializationPlan, want []string) bool {
	var b strings.Builder
	for _, item := range plan.Preservations {
		b.WriteString(item.Statement)
		b.WriteByte('\n')
	}
	for _, item := range plan.Warnings {
		b.WriteString(item.Message)
		b.WriteByte('\n')
	}
	text := b.String()
	for _, item := range want {
		if !strings.Contains(text, item) {
			return false
		}
	}
	return true
}
