package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"gopkg.in/yaml.v3"
)

func withTempAtlasHome(t *testing.T) {
	t.Helper()
	t.Setenv("ATLAS_HOME", t.TempDir())
}

func TestApplyConfig_WritesAtlasAndRuntime(t *testing.T) {
	withTempAtlasHome(t)

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
	mcp := config.EmptyMCPDraft()
	if !mcp.EnableBuiltin(config.MCPBuiltinFilesystem) {
		t.Fatal("filesystem")
	}
	if _, err := mcp.AddCustom("Internal Docs", config.MCPTransportStreamableHTTP, "https://example.local/mcp", "", ""); err != nil {
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
	if len(result.Files) != 8 {
		t.Fatalf("files = %#v", result.Files)
	}
	wantRuntime := 1 + 2 + len(config.AtlasAgentRuntimePaths([]string{"cursor", "opencode"}))
	if len(result.RuntimeFiles) != wantRuntime {
		t.Fatalf("runtime len=%d want=%d files=%#v", len(result.RuntimeFiles), wantRuntime, result.RuntimeFiles)
	}
	if result.HomePath == "" {
		t.Fatal("expected Atlas Home path on apply")
	}
	if _, err := os.Stat(filepath.Join(result.HomePath, "state", "home.yaml")); err != nil {
		t.Fatalf("atlas home state missing: %v", err)
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
		"<!-- ATLAS:BASE:BEGIN -->",
		"<!-- ATLAS:BASE:END -->",
		"<!-- ATLAS:USER:BEGIN -->",
		"<!-- ATLAS:USER:END -->",
		"Atlas-CLI",
		"## 1. Purpose and Authority",
		"## 10. Context Economy",
		"context.graph.enabled",
		"Skills are canonical under Atlas Home",
		".atlas/agent-registry.md",
		".atlas/skill-registry.md",
		".atlas/contracts/sdd-openspec.md",
		"atlas-orchestrator",
	} {
		if !strings.Contains(agentsText, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, agentsText)
		}
	}
	for _, rel := range []string{
		config.FileAgentRegistry,
		config.FileRuntimeManifest,
		config.FileSDDOpenSpecContract,
		".cursor/agents/atlas-orchestrator.md",
		".opencode/agents/atlas-orchestrator.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	contractText, err := os.ReadFile(filepath.Join(root, config.FileSDDOpenSpecContract))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contractText), "Operational Contract") ||
		!strings.Contains(string(contractText), "## 4. Phase contracts") {
		t.Fatalf("contract content unexpected:\n%s", contractText)
	}
	orch, err := os.ReadFile(filepath.Join(root, ".cursor", "agents", "atlas-orchestrator.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(orch), config.FileSDDOpenSpecContract) {
		t.Fatalf("orchestrator missing contract reference:\n%s", orch)
	}
	registryText, err := os.ReadFile(filepath.Join(root, config.FileAgentRegistry))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(registryText), config.FileSDDOpenSpecContract) {
		t.Fatalf("registry missing contract reference:\n%s", registryText)
	}
	manifestText, err := os.ReadFile(filepath.Join(root, config.FileRuntimeManifest))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestText), "sdd_contract:") ||
		!strings.Contains(string(manifestText), config.FileSDDOpenSpecContract) {
		t.Fatalf("manifest missing sdd_contract:\n%s", manifestText)
	}
	lockText, err := os.ReadFile(filepath.Join(root, config.FileAssetsLock))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lockText), "contracts/sdd-openspec.md") ||
		!strings.Contains(string(lockText), "checksum:") {
		t.Fatalf("lock missing contract entry:\n%s", lockText)
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
	if !strings.Contains(string(cursor), "alwaysApply: true") ||
		!strings.Contains(string(cursor), "Atlas Cursor Entrypoint") ||
		!strings.Contains(string(cursor), "do not bypass") ||
		!strings.Contains(string(cursor), config.FileSDDOpenSpecContract) {
		t.Fatalf("cursor projection = %s", cursor)
	}

	opencode, err := os.ReadFile(filepath.Join(root, ".opencode", "atlas.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(opencode), "Atlas OpenCode Entrypoint") ||
		!strings.Contains(string(opencode), ".opencode/atlas.md") ||
		!strings.Contains(string(opencode), "do not bypass") ||
		!strings.Contains(string(opencode), config.FileSDDOpenSpecContract) {
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
	withTempAtlasHome(t)

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
	withTempAtlasHome(t)

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
	id, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := filepath.ToSlash(filepath.Join("projects", id, "backups", "20261004T200000"))
	if !strings.HasPrefix(result.BackupDir, wantPrefix) || !strings.Contains(result.BackupDir, "-") {
		t.Fatalf("backup dir = %q want unique stamp under %q", result.BackupDir, wantPrefix)
	}
	backupAbs := filepath.Join(result.HomePath, filepath.FromSlash(result.BackupDir))
	if _, err := os.Stat(filepath.Join(root, ".atlas", "backups")); !os.IsNotExist(err) {
		t.Fatal("new backups must not be written under product repo .atlas/backups")
	}

	backupAgents, err := os.ReadFile(filepath.Join(backupAbs, "AGENTS.md"))
	if err != nil || string(backupAgents) != "# old agents\n" {
		t.Fatalf("backup agents = %q err=%v", backupAgents, err)
	}
	backupCursor, err := os.ReadFile(filepath.Join(backupAbs, ".cursor", "rules", "atlas.mdc"))
	if err != nil || string(backupCursor) != "old cursor\n" {
		t.Fatalf("backup cursor = %q err=%v", backupCursor, err)
	}

	manifestRaw, err := os.ReadFile(filepath.Join(backupAbs, "manifest.json"))
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
	if !strings.Contains(string(agents), "<!-- ATLAS:BASE:BEGIN -->") {
		t.Fatalf("AGENTS.md not replaced:\n%s", agents)
	}
	other, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "other.mdc"))
	if err != nil || string(other) != "keep\n" {
		t.Fatalf("unrelated cursor file mutated: %q err=%v", other, err)
	}
}

func TestApplyConfig_PreservesUserSection(t *testing.T) {
	withTempAtlasHome(t)

	root := t.TempDir()
	existing := "<!-- ATLAS:BASE:BEGIN -->\nold\n<!-- ATLAS:BASE:END -->\n\n" +
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
	withTempAtlasHome(t)

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
	if _, err := config.ApplyConfig(config.ApplyInput{
		Draft: config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{ProjectName: "x", ProjectMode: "new"}),
		MCP:   config.EmptyMCPDraft(),
	}); err == nil {
		t.Fatal("expected root error")
	}
}

func TestApplyConfig_RequiresHomeResetWhenProjectDataExists(t *testing.T) {
	withTempAtlasHome(t)

	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "new",
	})
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
	})
	if err == nil || !strings.Contains(err.Error(), "explicit reset") {
		t.Fatalf("expected reset gate, got %v", err)
	}
	result, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.HomeReset {
		t.Fatal("expected HomeReset")
	}

	otherRoot := t.TempDir()
	otherID, err := home.ProjectID(otherRoot, "demo")
	if err != nil {
		t.Fatal(err)
	}
	homePath := result.HomePath
	if err := home.EnsureProjectLayout(homePath, otherID); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home.ProjectContextDir(homePath, otherID), "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("same-name different-root Home data must survive reset")
	}
}

func TestPersistConfigure_WritesConfigOnly(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinJira)
	if _, err := mcp.AddCustom("Docs", config.MCPTransportStreamableHTTP, "https://docs.local/mcp", "", "HOME"); err != nil {
		t.Fatal(err)
	}
	mcp.ToggleCustom(0)

	result, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp})
	if err != nil {
		t.Fatalf("persist: %v", err)
	}
	if !strings.Contains(result.Notice, "Configuration changes are saved to .atlas/config.yaml.") {
		t.Fatalf("notice = %q", result.Notice)
	}
	if !strings.Contains(result.Notice, "Runtime files are not repaired automatically.") {
		t.Fatalf("missing runtime repair honesty: %q", result.Notice)
	}
	if !strings.Contains(result.Notice, "MCP selections were reconciled") {
		t.Fatalf("missing MCP projection notice: %q", result.Notice)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.MCP.Builtins.Jira.Enabled || len(doc.MCP.Custom) != 1 || doc.MCP.Custom[0].Transport != "streamable_http" {
		t.Fatalf("doc mcp = %#v", doc.MCP)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "local.yaml")); !os.IsNotExist(err) {
		t.Fatal("persist configure must not write local.yaml")
	}
	assertMissing(t, root, "AGENTS.md")
	// No adapters selected => no native MCP projection files.
	assertMissing(t, root, ".cursor")
	assertMissing(t, root, "opencode.json")
	assertMissing(t, root, ".atlas/state.yaml")
	assertMissing(t, root, "skills")
	assertMissing(t, root, ".agents")
	assertMissing(t, root, config.FileProjectDocsREADME)
}

func TestPersistConfigure_AdapterChangeRecommendsRepair(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	base := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo", ProjectMode: "existing",
	})
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: base, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatal(err)
	}
	changed := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	if !changed.ToggleMulti("adapters.selected", "cursor") {
		t.Fatal("toggle cursor")
	}
	result, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: changed, MCP: config.EmptyMCPDraft()})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Impact.RuntimeRepairNeeded || !strings.Contains(result.Notice, "Runtime Repair") {
		t.Fatalf("expected repair recommendation: %#v notice=%q", result.Impact, result.Notice)
	}
	assertMissing(t, root, "AGENTS.md")
	assertMissing(t, root, ".cursor/rules/atlas.mdc")
}

func TestFormatConfigureFooterNote_DefaultConfigOnly(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	note := config.FormatConfigureFooterNote(draft)
	for _, want := range []string{
		"Apply saves .atlas/config.yaml.",
		"Runtime files (AGENTS.md, rules, agents) are not repaired or rematerialized automatically.",
		"Context Economy payloads are not updated automatically.",
		"MCP projections for selected adapters are reconciled on Apply",
		"Runtime Repair and Update Context are separate flows.",
	} {
		if !strings.Contains(note, want) {
			t.Fatalf("default footer missing %q:\n%s", want, note)
		}
	}
	for _, banned := range []string{
		"docs/atlas/README.md",
		"only possible write",
		"config.yaml only",
		"writes .atlas/config.yaml only",
	} {
		if strings.Contains(note, banned) {
			t.Fatalf("default footer unexpected %q:\n%s", banned, note)
		}
	}
	// Default meaning: config.yaml is the Apply write; no scaffold create claim.
	if strings.Contains(note, "may also create") {
		t.Fatalf("default footer must not claim docs create:\n%s", note)
	}
}

func TestFormatConfigureFooterNote_DocsScaffoldSelected(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	if !draft.SetValue("project.docs_scaffold", "true") {
		t.Fatal("enable docs scaffold")
	}
	note := config.FormatConfigureFooterNote(draft)
	for _, want := range []string{
		"Apply saves .atlas/config.yaml.",
		"docs/atlas/README.md once",
		"Runtime files (AGENTS.md, rules, agents) are not repaired or rematerialized automatically.",
		"Context Economy payloads are not updated automatically.",
		"MCP projections for selected adapters are reconciled on Apply",
		"Runtime Repair and Update Context are separate flows.",
	} {
		if !strings.Contains(note, want) {
			t.Fatalf("scaffold footer missing %q:\n%s", want, note)
		}
	}
	for _, banned := range []string{
		"config.yaml only",
		"writes .atlas/config.yaml only",
		"only — not runtime",
	} {
		if strings.Contains(note, banned) {
			t.Fatalf("scaffold footer must not claim config.yaml is the only write (%q):\n%s", banned, note)
		}
	}
}

func TestEnsureProjectDocsScaffold_NoOverwrite(t *testing.T) {
	root := t.TempDir()
	created, skipped, err := config.EnsureProjectDocsScaffold(root, true)
	if err != nil || len(created) != 1 || len(skipped) != 0 {
		t.Fatalf("first create = %v %v err=%v", created, skipped, err)
	}
	path := filepath.Join(root, config.FileProjectDocsREADME)
	if err := os.WriteFile(path, []byte("keep mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, skipped, err = config.EnsureProjectDocsScaffold(root, true)
	if err != nil || len(created) != 0 || len(skipped) != 1 {
		t.Fatalf("second = %v %v err=%v", created, skipped, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "keep mine\n" {
		t.Fatalf("overwrote docs: %q err=%v", got, err)
	}
	created, skipped, err = config.EnsureProjectDocsScaffold(root, false)
	if err != nil || created != nil || skipped != nil {
		t.Fatalf("off by default path should no-op: %v %v %v", created, skipped, err)
	}
}

func assertMissing(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist, err=%v", rel, err)
	}
}
