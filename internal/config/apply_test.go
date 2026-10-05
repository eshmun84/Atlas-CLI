package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"gopkg.in/yaml.v3"
)

func TestApplyConfig_WritesAtlasAndRuntime(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	head := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), head, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "Atlas-CLI",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SelectOption("source_control.mode", "git_github") {
		t.Fatal("source control")
	}
	if !draft.SelectOption("source_control.branch_strategy", "main_develop") {
		t.Fatal("branch")
	}
	mcp := config.EmptyMCPDraft()
	if !mcp.ToggleBuiltin(0) {
		t.Fatal("jira")
	}
	if _, err := mcp.AddCustom("Internal Docs", config.MCPTransportHTTP, "https://example.local/mcp", "", ""); err != nil {
		t.Fatalf("custom: %v", err)
	}
	mcp.ToggleCustom(0)

	fixed := time.Date(2026, 10, 4, 19, 35, 0, 0, time.UTC)
	result, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   mcp,
		Now:   func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Files) != 4 {
		t.Fatalf("files = %#v", result.Files)
	}
	if len(result.RuntimeFiles) != 3 {
		t.Fatalf("runtime = %#v", result.RuntimeFiles)
	}

	cfgPath := filepath.Join(root, ".atlas", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"name: Atlas-CLI",
		"mode: new",
		"workflow: sdd",
		"spec_engine: openspec",
		"jira:",
		"enabled: true",
		"Internal Docs",
		"governance_files: local_only",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.yaml missing %q:\n%s", want, text)
		}
	}
	for _, banned := range []string{"password", "token", "secret", "api_key"} {
		if strings.Contains(strings.ToLower(text), banned) {
			t.Fatalf("config.yaml must not contain %q:\n%s", banned, text)
		}
	}

	stateRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var state config.StateDocument
	if err := yaml.Unmarshal(stateRaw, &state); err != nil {
		t.Fatal(err)
	}
	if !state.Initialized || !state.RuntimeMaterialized || state.AppliedAt != "2026-10-04T19:35:00Z" || state.RuntimeMaterializedAt != "2026-10-04T19:35:00Z" {
		t.Fatalf("state = %#v", state)
	}

	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	agentsText := string(agents)
	for _, want := range []string{
		"<!-- ATLAS:MANAGED:BEGIN -->",
		"<!-- ATLAS:MANAGED:END -->",
		"<!-- ATLAS:USER:BEGIN -->",
		"<!-- ATLAS:USER:END -->",
		"Atlas-CLI",
		"## 1. Rules",
		"## 10. Agent and Subagent Orchestration",
		"context.graph.enabled",
		"Do not expect a full skills or agents catalog",
	} {
		if !strings.Contains(agentsText, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, agentsText)
		}
	}
	if !strings.Contains(text, "context:") || !strings.Contains(text, "graph:") || !strings.Contains(text, "enabled: true") {
		t.Fatalf("config.yaml missing context.graph.enabled:\n%s", text)
	}
	assertMissing(t, root, "skills")
	assertMissing(t, root, ".agents")
	assertMissing(t, root, "agents")

	cursor, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cursor), "alwaysApply: true") || !strings.Contains(string(cursor), "AGENTS.md") {
		t.Fatalf("cursor rule = %s", cursor)
	}

	opencode, err := os.ReadFile(filepath.Join(root, ".opencode", "atlas.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(opencode), "OpenCode") || !strings.Contains(string(opencode), ".opencode/atlas.md") {
		t.Fatalf("opencode = %s", opencode)
	}

	assertMissing(t, root, ".agents")
	assertMissing(t, root, ".claude")
	assertMissing(t, root, "CLAUDE.md")
	assertMissing(t, root, "GEMINI.md")
	assertMissing(t, root, filepath.Join(".atlas", "memory", "atlas.sqlite"))
	assertMissing(t, root, filepath.Join(".atlas", "context", "memory-capsule.md"))

	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(readme) != "keep me" {
		t.Fatalf("README mutated: %s err=%v", readme, err)
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil || string(ignore) != "bin/\n" {
		t.Fatalf("gitignore mutated: %s err=%v", ignore, err)
	}
	gotHead, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil || string(gotHead) != string(head) {
		t.Fatalf("git HEAD mutated: %s err=%v", gotHead, err)
	}
}

func TestApplyConfig_AgentsOnlyWithoutAdapters(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "solo",
		ProjectMode: "new",
	})
	result, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RuntimeFiles) != 1 || result.RuntimeFiles[0] != "AGENTS.md" {
		t.Fatalf("runtime = %#v", result.RuntimeFiles)
	}
	assertMissing(t, root, ".cursor")
	assertMissing(t, root, ".opencode")
}

func TestApplyConfig_BacksUpExistingTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# old agents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"), []byte("old cursor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "rules", "other.mdc"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:    "demo",
		ProjectMode:    "existing",
		CursorDetected: true,
	})
	fixed := time.Date(2026, 10, 4, 20, 0, 0, 0, time.UTC)
	result, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupDir != ".atlas/backups/20261004T200000Z" {
		t.Fatalf("backup dir = %q", result.BackupDir)
	}

	backupAgents, err := os.ReadFile(filepath.Join(root, ".atlas", "backups", "20261004T200000Z", "AGENTS.md"))
	if err != nil || string(backupAgents) != "# old agents\n" {
		t.Fatalf("backup agents = %q err=%v", backupAgents, err)
	}
	backupCursor, err := os.ReadFile(filepath.Join(root, ".atlas", "backups", "20261004T200000Z", ".cursor", "rules", "atlas.mdc"))
	if err != nil || string(backupCursor) != "old cursor\n" {
		t.Fatalf("backup cursor = %q err=%v", backupCursor, err)
	}

	manifestRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "backups", "20261004T200000Z", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest config.BackupManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.Reason == "" || len(manifest.Entries) != 2 {
		t.Fatalf("manifest = %#v", manifest)
	}
	for _, entry := range manifest.Entries {
		if entry.Kind != "file" || entry.Action != "replaced" || entry.SHA256 == "" {
			t.Fatalf("entry = %#v", entry)
		}
	}

	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "<!-- ATLAS:MANAGED:BEGIN -->") {
		t.Fatalf("AGENTS.md not replaced:\n%s", agents)
	}
	other, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "other.mdc"))
	if err != nil || string(other) != "keep\n" {
		t.Fatalf("unrelated cursor file mutated: %q err=%v", other, err)
	}
}

func TestApplyConfig_PreservesUserSection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	existing := "<!-- ATLAS:MANAGED:BEGIN -->\nold\n<!-- ATLAS:MANAGED:END -->\n\n" +
		"<!-- ATLAS:USER:BEGIN -->\nKeep my notes\n<!-- ATLAS:USER:END -->\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	if _, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Keep my notes") {
		t.Fatalf("user section lost:\n%s", got)
	}
}

func TestApplyConfig_BlocksDirectoryTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "AGENTS.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "new",
	})
	_, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("err = %v", err)
	}
	assertMissing(t, root, ".atlas/config.yaml")
}

func TestApplyConfig_RequiresRoot(t *testing.T) {
	t.Parallel()
	if _, err := config.ApplyConfig(config.ApplyInput{
		Draft: config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{ProjectName: "x", ProjectMode: "new"}),
		MCP:   config.EmptyMCPDraft(),
	}); err == nil {
		t.Fatal("expected root error")
	}
}

func TestPersistConfigure_WritesConfigOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	mcp := config.EmptyMCPDraft()
	mcp.ToggleBuiltin(0)
	if _, err := mcp.AddCustom("Docs", config.MCPTransportSSE, "https://docs.local", "a", "HOME"); err != nil {
		t.Fatal(err)
	}
	mcp.ToggleCustom(0)

	if err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	doc, err := config.LoadProjectDocument(filepath.Join(root, config.FileConfig))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.MCP.Builtins.Jira.Enabled || len(doc.MCP.Custom) != 1 || doc.MCP.Custom[0].Transport != "sse" {
		t.Fatalf("doc mcp = %#v", doc.MCP)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "local.yaml")); !os.IsNotExist(err) {
		t.Fatal("persist configure must not write local.yaml")
	}
	assertMissing(t, root, "AGENTS.md")
	assertMissing(t, root, ".cursor")
	assertMissing(t, root, ".opencode")
}

func assertMissing(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist, err=%v", rel, err)
	}
}
