package config_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func fullMCPDraft(t *testing.T) config.MCPDraft {
	t.Helper()
	mcp := config.EmptyMCPDraft()
	_ = mcp.EnableBuiltin(config.MCPBuiltinContext7)
	_ = mcp.EnableBuiltin(config.MCPBuiltinChromeDevTools)
	return mcp
}

func configureDraftBoth(t *testing.T, name string) config.ConfigDraft {
	t.Helper()
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: name, ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	return draft
}

func assertCursorAtlasSurfaces(t *testing.T, root string, want bool) {
	t.Helper()
	paths := []string{
		".cursor/rules/atlas.mdc",
		".cursor/agents/atlas-orchestrator.md",
		".cursor/hooks/atlas/dispatch",
		".cursor/skills/testing/SKILL.md",
		".cursor/skills/memory/SKILL.md",
	}
	for _, rel := range paths {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if want && err != nil {
			t.Fatalf("missing Cursor Atlas surface %s: %v", rel, err)
		}
		if !want && !os.IsNotExist(err) {
			t.Fatalf("Cursor Atlas surface %s should be gone", rel)
		}
	}
	if want {
		raw, err := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
		if err != nil || !strings.Contains(string(raw), "context7") || !strings.Contains(string(raw), "chrome_devtools") {
			t.Fatalf("Cursor MCP incomplete: %v %s", err, raw)
		}
	}
}

func assertOpenCodeAtlasSurfaces(t *testing.T, root string, want bool) {
	t.Helper()
	paths := []string{
		".opencode/atlas.md",
		".opencode/agents/atlas-orchestrator.md",
		".opencode/plugins/atlas-hooks/index.ts",
		".opencode/skills/testing/SKILL.md",
		".opencode/skills/memory/SKILL.md",
		opencode.ConfigRelPath,
	}
	for _, rel := range paths {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if want && err != nil {
			t.Fatalf("missing OpenCode Atlas surface %s: %v", rel, err)
		}
		if !want && rel == opencode.ConfigRelPath {
			// Layer may be absent when MCP removed; other surfaces must be gone.
			continue
		}
		if !want && !os.IsNotExist(err) && !strings.HasSuffix(rel, "opencode.json") {
			t.Fatalf("OpenCode Atlas surface %s should be gone", rel)
		}
	}
	if want {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(opencode.ConfigRelPath)))
		if err != nil || !strings.Contains(string(raw), "context7") || !strings.Contains(string(raw), "chrome_devtools") {
			t.Fatalf("OpenCode MCP incomplete: %v %s", err, raw)
		}
	} else {
		if raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(opencode.ConfigRelPath))); err == nil {
			if strings.Contains(string(raw), "context7") || strings.Contains(string(raw), "chrome_devtools") {
				t.Fatalf("OpenCode Atlas MCP keys should be removed: %s", raw)
			}
		}
		for _, rel := range []string{
			".opencode/atlas.md",
			".opencode/agents/atlas-orchestrator.md",
			".opencode/plugins/atlas-hooks/index.ts",
		} {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
				t.Fatalf("OpenCode Atlas surface %s should be gone", rel)
			}
		}
	}
}

func TestFullReproject_OpenCodeToCursor(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "oc2cu")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_ = draft.SelectOption("source_control.mode", "git_local")
	_ = draft.SelectOption("source_control.governance_storage", "local_only")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	userOC := filepath.Join(root, ".opencode", "skills", "mine", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userOC), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userOC, []byte("user skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Seed legacy root Atlas keys — must migrate out on swap/reconcile.
	if err := os.WriteFile(filepath.Join(root, "opencode.json"), []byte(`{
  "$schema": "https://opencode.ai/config.json",
  "model": "user/model",
  "mcp": {
    "context7": {"type": "remote", "url": "https://old.example", "enabled": true},
    "user-keep": {"type": "local", "command": ["echo"], "enabled": true}
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeCfg, _ := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))

	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertCursorAtlasSurfaces(t, root, true)
	assertOpenCodeAtlasSurfaces(t, root, false)
	got, err := os.ReadFile(userOC)
	if err != nil || string(got) != "user skill\n" {
		t.Fatalf("unknown OpenCode content lost: %v %q", err, got)
	}
	rootOC, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rootOC), "context7") {
		t.Fatalf("legacy Atlas key must leave root opencode.json: %s", rootOC)
	}
	if !strings.Contains(string(rootOC), "user-keep") || !strings.Contains(string(rootOC), "user/model") {
		t.Fatalf("user root OpenCode config lost: %s", rootOC)
	}
	afterCfg, _ := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if !strings.Contains(string(afterCfg), "context7") || !strings.Contains(string(afterCfg), "chrome_devtools") {
		t.Fatal("canonical MCP desired state lost")
	}
	_ = beforeCfg
}

func TestFullReproject_CursorToOpenCode(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "cu2oc")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SelectOption("source_control.governance_storage", "local_only")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	userRule := filepath.Join(root, ".cursor", "rules", "mine.mdc")
	if err := os.MkdirAll(filepath.Dir(userRule), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userRule, []byte("user owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertOpenCodeAtlasSurfaces(t, root, true)
	assertCursorAtlasSurfaces(t, root, false)
	got, err := os.ReadFile(userRule)
	if err != nil || string(got) != "user owned\n" {
		t.Fatalf("unknown Cursor content lost: %v %q", err, got)
	}
	agents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if strings.Contains(string(agents), "ATLAS:ADAPTER:CURSOR") || !strings.Contains(string(agents), "ATLAS:ADAPTER:OPENCODE") {
		t.Fatalf("AGENTS adapter blocks wrong: %s", agents)
	}
}

func TestFullReproject_AddAdapter_CursorToBoth(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "add")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	cursorBefore, _ := os.ReadFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertCursorAtlasSurfaces(t, root, true)
	assertOpenCodeAtlasSurfaces(t, root, true)
	cursorAfter, _ := os.ReadFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	if string(cursorBefore) != string(cursorAfter) {
		t.Fatal("healthy Cursor Atlas projection rewritten without need")
	}
}

func TestFullReproject_RemoveAdapter_BothToOpenCode(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "rm")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	userCursor := filepath.Join(root, ".cursor", "rules", "keep.mdc")
	if err := os.MkdirAll(filepath.Dir(userCursor), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCursor, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertOpenCodeAtlasSurfaces(t, root, true)
	assertCursorAtlasSurfaces(t, root, false)
	got, err := os.ReadFile(userCursor)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("user Cursor content lost: %v %q", err, got)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.MCP.Builtins.Context7.Enabled || !doc.MCP.Builtins.ChromeDevTools.Enabled {
		t.Fatalf("MCP desired state changed: %#v", doc.MCP.Builtins)
	}
}

func TestGitVisibility_LocalOnlyAndVersioned_Physical(t *testing.T) {
	withTempAtlasHome(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}

	for _, tc := range []struct {
		name     string
		adapters []string
		storage  string
		local    []string // must be ignored / not versionable
		visible  []string // may appear as untracked (non-secret Atlas or user)
	}{
		{
			name:     "cursor_local_only",
			adapters: []string{"cursor"},
			storage:  config.GovernanceFilesLocalOnly,
			local:    []string{".atlas/config.yaml", "AGENTS.md", ".cursor/rules/atlas.mdc", ".cursor/mcp.json"},
		},
		{
			name:     "opencode_local_only",
			adapters: []string{"opencode"},
			storage:  config.GovernanceFilesLocalOnly,
			local:    []string{".atlas/config.yaml", "AGENTS.md", ".opencode/atlas.md", ".opencode/opencode.json", ".opencode/plugins/atlas-hooks/index.ts"},
			visible:  []string{}, // root opencode.json may be absent
		},
		{
			name:     "both_local_only",
			adapters: []string{"cursor", "opencode"},
			storage:  config.GovernanceFilesLocalOnly,
			local:    []string{".cursor/mcp.json", ".opencode/opencode.json", "AGENTS.md"},
		},
		{
			name:     "cursor_versioned",
			adapters: []string{"cursor"},
			storage:  config.GovernanceFilesVersioned,
			visible:  []string{".atlas/config.yaml", "AGENTS.md", ".cursor/rules/atlas.mdc", ".cursor/mcp.json"},
		},
		{
			name:     "opencode_versioned",
			adapters: []string{"opencode"},
			storage:  config.GovernanceFilesVersioned,
			visible:  []string{".atlas/config.yaml", "AGENTS.md", ".opencode/atlas.md", ".opencode/opencode.json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runGitQuiet(t, root, "init")
			// User-owned OpenCode root config should stay versionable under local_only.
			userRoot := filepath.Join(root, "opencode.json")
			if err := os.WriteFile(userRoot, []byte(`{"$schema":"https://opencode.ai/config.json","model":"user/keep"}`+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			userSkill := filepath.Join(root, ".opencode", "skills", "custom-user", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(userSkill), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(userSkill, []byte("custom\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			draft := configureDraftBoth(t, tc.name)
			for _, a := range tc.adapters {
				_ = draft.ToggleMulti("adapters.selected", a)
			}
			_ = draft.SelectOption("source_control.mode", "git_local")
			_ = draft.SelectOption("source_control.governance_storage", tc.storage)
			if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCPDraft(t)}); err != nil {
				t.Fatal(err)
			}

			status := gitStatusAll(t, root)
			if tc.storage == config.GovernanceFilesLocalOnly {
				for _, rel := range tc.local {
					if gitPathVersionable(status, rel) {
						t.Fatalf("local_only: %s must not be versionable; status=\n%s", rel, status)
					}
					out := runGitOutput(t, root, "check-ignore", "-v", rel)
					if strings.TrimSpace(out) == "" {
						t.Fatalf("local_only: check-ignore empty for %s", rel)
					}
				}
				// Root user OpenCode config must remain versionable.
				if !gitPathVersionable(status, "opencode.json") {
					t.Fatalf("user root opencode.json must stay versionable; status=\n%s", status)
				}
				if gitPathVersionable(status, ".opencode/opencode.json") {
					t.Fatalf("Atlas MCP layer must not be versionable; status=\n%s", status)
				}
			} else {
				for _, rel := range tc.visible {
					if !gitPathVersionable(status, rel) {
						t.Fatalf("versioned: %s should be versionable; status=\n%s\nignore=%s",
							rel, status, runGitOutput(t, root, "check-ignore", "-v", rel))
					}
				}
				gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
				if strings.Contains(string(gi), "# >>> Atlas local governance >>>") {
					t.Fatal("versioned must strip Atlas ignore block")
				}
			}
		})
	}
}

func TestMCPReproject_DesiredStatePreservedAcrossAdapters(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "mcp-state")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.MCP.Builtins.Context7.Enabled || !doc.MCP.Builtins.ChromeDevTools.Enabled {
		t.Fatalf("desired MCP lost: %#v", doc.MCP.Builtins)
	}
	assertOpenCodeAtlasSurfaces(t, root, true)
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertCursorAtlasSurfaces(t, root, true)
	assertOpenCodeAtlasSurfaces(t, root, true)
}

func TestOwnershipIgnore_AdapterCombinations(t *testing.T) {
	t.Parallel()
	cursorOnly := config.AtlasLocalGovernanceGitignoreLinesFor(config.ProjectDocument{
		Adapters: config.AdaptersPersist{Selected: []string{"cursor"}},
	})
	join := strings.Join(cursorOnly, "\n")
	if !strings.Contains(join, ".cursor/") || strings.Contains(join, ".opencode/") {
		t.Fatalf("cursor-only lines: %v", cursorOnly)
	}
	ocOnly := config.AtlasLocalGovernanceGitignoreLinesFor(config.ProjectDocument{
		Adapters: config.AdaptersPersist{Selected: []string{"opencode"}},
	})
	join = strings.Join(ocOnly, "\n")
	if strings.Contains(join, ".cursor/") || !strings.Contains(join, ".opencode/opencode.json") {
		t.Fatalf("opencode-only lines: %v", ocOnly)
	}
	for _, line := range ocOnly {
		if strings.TrimSpace(line) == "opencode.json" {
			t.Fatal("must not ignore root opencode.json")
		}
	}
	both := config.AtlasLocalGovernanceGitignoreLinesFor(config.ProjectDocument{
		Adapters: config.AdaptersPersist{Selected: []string{"cursor", "opencode"}},
	})
	join = strings.Join(both, "\n")
	if !strings.Contains(join, ".cursor/") || !strings.Contains(join, ".opencode/opencode.json") {
		t.Fatalf("both lines: %v", both)
	}
}

func TestContext7_UsesEnvRefNotRawSecret(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "sec")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCPDraft(t)}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".cursor/mcp.json", opencode.ConfigRelPath, ".atlas/config.yaml"} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(raw))
		for _, banned := range []string{"sk-", "api_key\": \"", "bearer ey"} {
			if strings.Contains(lower, banned) {
				t.Fatalf("%s leaked secret-like payload: %s", rel, raw)
			}
		}
		if rel != ".atlas/config.yaml" && strings.Contains(string(raw), "context7") {
			var doc map[string]any
			_ = json.Unmarshal(raw, &doc)
			_ = doc
		}
	}
}

func gitStatusAll(t *testing.T, root string) string {
	t.Helper()
	return runGitOutput(t, root, "status", "--short", "--untracked-files=all")
}

func gitPathVersionable(status, rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// status --short: XY path
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		path := filepath.ToSlash(fields[len(fields)-1])
		if path == rel || strings.HasPrefix(path, rel+"/") {
			return true
		}
	}
	return false
}

func runGitQuiet(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func runGitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// check-ignore returns exit 1 when not ignored — still useful.
		if len(args) > 0 && args[0] == "check-ignore" {
			return string(out)
		}
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
