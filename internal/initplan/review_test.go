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
	if plan.ContextGraph != "Enabled" {
		t.Fatalf("context graph = %q", plan.ContextGraph)
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
	if !stringsContainsAll(plan, []string{
		"Existing project source files are preserved.",
		"README.md is preserved unless future explicit README integration is enabled.",
		"Git history is not modified.",
		"No commits are created.",
		"No branches are created.",
		"No remote operations are performed.",
		"Secrets and credentials are not stored.",
		"Developer-owned non-Atlas agents under .cursor/agents/ and .opencode/agents/ are left untouched.",
		"Skills are registry-first and are not copied into .cursor/skills or .opencode/skills.",
		"Apply writes Atlas configuration under .atlas/ and materializes compact runtime gateway files.",
		"Apply creates/updates Atlas Home (ATLAS_HOME or ~/.atlas) and mirrors bundled Atlas-owned assets.",
		"AGENTS.md is the project authority; Atlas agents are cataloged in .atlas/agent-registry.md with Home source paths.",
		"Skills remain registry-first; this slice does not vendor skills into adapter skill folders.",
		"Context Graph is a preference/context aid only; no graph engine, database, embeddings, index, capsules, or packs.",
		"Cursor/OpenCode entrypoints point at AGENTS.md and atlas-orchestrator; they must not bypass AGENTS.md.",
		"Existing Atlas-managed runtime targets are backed up under .atlas/backups/<timestamp>/ before replacement.",
		"CLAUDE.md, GEMINI.md, .agents/, .claude/, README.md, and .gitignore are not materialized.",
		"No Git operations are performed.",
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
		".atlas/agent-registry.md",
		".atlas/runtime-manifest.yaml",
		".atlas/contracts/sdd-openspec.md",
		".atlas/backups/",
		"AGENTS.md",
		".cursor/rules/atlas.mdc",
		".opencode/atlas.md",
		".cursor/agents/atlas-orchestrator.md",
		".opencode/agents/atlas-orchestrator.md",
	}
	assertCreatePaths(t, plan, wantCreates)
	for _, file := range plan.Creates {
		if file.Path == ".atlas/backups/" {
			if file.Status != "create if needed" {
				t.Fatalf("backups status = %q", file.Status)
			}
			continue
		}
		if file.Status != "create/update this slice" {
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
	if plan.MCPEntries[0].Status != "will persist in .atlas/config.yaml" || plan.MCPEntries[1].Status != "will persist in .atlas/config.yaml" {
		t.Fatalf("mcp status = %#v", plan.MCPEntries)
	}
	if plan.GovernanceNote == "" || plan.GovernanceStorage != "Versioned" {
		t.Fatalf("governance note=%q storage=%q", plan.GovernanceNote, plan.GovernanceStorage)
	}

	foundAgents, foundCursor, foundManifest, foundReplaceAgents, foundReplaceCursor := false, false, false, false, false
	for _, backup := range plan.Backups {
		if backup.Path == ".atlas/backups/<timestamp>/AGENTS.md" {
			foundAgents = true
		}
		if backup.Path == ".atlas/backups/<timestamp>/.cursor/rules/atlas.mdc" {
			foundCursor = true
		}
		if backup.Path == ".atlas/backups/<timestamp>/manifest.json" {
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
