package initplan_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

func TestBuildReview_NoArtifacts(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "Atlas-CLI",
		ProjectMode:      "new",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
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
	if plan.SourceControl != "None" {
		t.Fatalf("source control = %q", plan.SourceControl)
	}
	if plan.BranchStrategy != "Manual" {
		t.Fatalf("branch = %q", plan.BranchStrategy)
	}
	if plan.GovernanceStorage != "Local only" {
		t.Fatalf("governance = %q", plan.GovernanceStorage)
	}
	if plan.MemoryStrategy != "SQLite + Context Capsule" {
		t.Fatalf("memory = %q", plan.MemoryStrategy)
	}
	if plan.MCPCount != 0 {
		t.Fatalf("mcp count = %d", plan.MCPCount)
	}
	if !plan.PreviewOnly {
		t.Fatal("plan must be preview-only")
	}
	if len(plan.ExistingArtifacts) != 0 {
		t.Fatalf("artifacts = %#v", plan.ExistingArtifacts)
	}
	if len(plan.Backups) != 0 || len(plan.Replacements) != 0 {
		t.Fatalf("unexpected backups/replacements: %#v %#v", plan.Backups, plan.Replacements)
	}
	if !stringsContainsAll(plan, []string{
		"Existing project source files are preserved.",
		"README.md is preserved unless future explicit README integration is enabled.",
		"Git history is not modified.",
		"No commits are created.",
		"No branches are created.",
		"No remote operations are performed.",
		"Secrets and credentials are not stored.",
		"Materialization is not implemented yet.",
		"Review is preview-only.",
		"Configuration is in-memory only.",
		"MCP entries are not persisted.",
	}) {
		t.Fatalf("missing preserve/warning copy: %#v %#v", plan.Preservations, plan.Warnings)
	}
	if !strings.Contains(plan.GovernanceNote, "stay local") {
		t.Fatalf("governance note = %q", plan.GovernanceNote)
	}

	wantCreates := []string{
		".atlas/config.yaml",
		".atlas/local.yaml",
		".atlas/state.yaml",
		".atlas/assets.lock.yaml",
		".atlas/backups/",
		"AGENTS.md",
		".cursor/rules/atlas.mdc",
		"OpenCode runtime adapter files",
	}
	assertCreatePaths(t, plan, wantCreates)
}

func TestBuildReview_WithArtifactsAndMCP(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	if !draft.SelectOption("source_control.governance_storage", "versioned") {
		t.Fatal("set versioned")
	}
	mcp := config.EmptyMCPDraft()
	if !mcp.ToggleBuiltin(0) {
		t.Fatal("select jira")
	}
	if _, err := mcp.AddCustom("Jira Main", config.MCPTransportStdio, "", "", ""); err != nil {
		t.Fatalf("add mcp: %v", err)
	}
	mcp.ToggleCustom(0)

	plan := initplan.BuildReview(initplan.ReviewInput{
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
	if plan.MCPEntries[0].Status != "in memory only" || plan.MCPEntries[1].Status != "in memory only" {
		t.Fatalf("mcp status = %#v", plan.MCPEntries)
	}
	if plan.GovernanceNote == "" || plan.GovernanceStorage != "Versioned" {
		t.Fatalf("governance note=%q storage=%q", plan.GovernanceNote, plan.GovernanceStorage)
	}

	foundBackup, foundManifest, foundReplace := false, false, false
	for _, backup := range plan.Backups {
		if backup.Path == ".atlas/backups/<timestamp>/AGENTS.md" {
			foundBackup = true
		}
		if backup.Path == ".atlas/backups/<timestamp>/manifest.json" {
			foundManifest = true
		}
	}
	for _, repl := range plan.Replacements {
		if repl.Path == "AGENTS.md" {
			foundReplace = true
		}
	}
	if !foundBackup || !foundManifest || !foundReplace {
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
