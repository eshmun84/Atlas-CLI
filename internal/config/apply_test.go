package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"gopkg.in/yaml.v3"
)

func TestApplyConfig_WritesAtlasFilesOnly(t *testing.T) {
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
		"context7:",
		"chrome_devtools:",
		"Internal Docs",
		"transport: http",
		"command_or_url: https://example.local/mcp",
		"governance_files: local_only",
		"strategy: sqlite_plus_context_capsule",
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

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load persisted config: %v", err)
	}
	if loaded.Project.Name != "Atlas-CLI" || loaded.Project.Mode != config.ModeGreenfield {
		t.Fatalf("loaded = %#v", loaded.Project)
	}

	localRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(localRaw), "credentials_stored: false") {
		t.Fatalf("local.yaml = %s", localRaw)
	}

	stateRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var state config.StateDocument
	if err := yaml.Unmarshal(stateRaw, &state); err != nil {
		t.Fatal(err)
	}
	if !state.Initialized || state.RuntimeMaterialized || state.AppliedAt != "2026-10-04T19:35:00Z" {
		t.Fatalf("state = %#v", state)
	}

	lockRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "assets.lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lockRaw), "assets:") {
		t.Fatalf("lock = %s", lockRaw)
	}

	info, err := os.Stat(filepath.Join(root, ".atlas", "backups"))
	if err != nil || !info.IsDir() {
		t.Fatalf("backups dir: %v %#v", err, info)
	}

	assertMissing(t, root, "AGENTS.md")
	assertMissing(t, root, ".cursor")
	assertMissing(t, root, ".opencode")
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
}

func assertMissing(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist, err=%v", rel, err)
	}
}
