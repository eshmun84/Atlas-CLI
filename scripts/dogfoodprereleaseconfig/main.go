// Dogfood harness: pre-release configuration reconciliation physical scenarios.
//
//	go run ./scripts/dogfoodprereleaseconfig
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dogfoodprereleaseconfig FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("dogfoodprereleaseconfig PASS")
}

func run() error {
	fmt.Printf("version=%s\n", version.Version)
	if version.Version != "0.1.0" {
		return fmt.Errorf("version must remain 0.1.0, got %s", version.Version)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git required for physical visibility checks: %w", err)
	}

	homeDir, err := os.MkdirTemp("", "atlas-dogfood-prerelease-home-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(homeDir)
	os.Setenv("ATLAS_HOME", homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		return fmt.Errorf("home: %w", err)
	}

	steps := []struct {
		name string
		fn   func(string) error
	}{
		{"1 zero adapter blocked", scenarioZeroAdapterBlocked},
		{"2 cursor local_only + MCP", scenarioCursorLocalOnlyMCP},
		{"3 opencode local_only + MCP", scenarioOpenCodeLocalOnlyMCP},
		{"4 cursor versioned", scenarioCursorVersioned},
		{"5 opencode versioned", scenarioOpenCodeVersioned},
		{"6 cursor -> opencode full reprojection", scenarioCursorToOpenCode},
		{"7 opencode -> cursor full reprojection", scenarioOpenCodeToCursor},
		{"8 cursor -> cursor+opencode", scenarioAddOpenCode},
		{"9 cursor+opencode -> opencode", scenarioRemoveCursor},
		{"10 unknown/user-owned preserved", scenarioUserOwnedPreserved},
		{"11 legacy custom not rematerialized", scenarioLegacyTolerant},
	}
	for _, s := range steps {
		if err := s.fn(homeDir); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
		fmt.Printf("  ok: %s\n", s.name)
	}
	return nil
}

func fullMCP() config.MCPDraft {
	mcp := config.EmptyMCPDraft()
	_ = mcp.EnableBuiltin(config.MCPBuiltinContext7)
	_ = mcp.EnableBuiltin(config.MCPBuiltinChromeDevTools)
	return mcp
}

func scenarioZeroAdapterBlocked(homeDir string) error {
	root, err := os.MkdirTemp("", "atlas-prerelease-zero-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "zero", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_, err = config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
	if err == nil || !strings.Contains(err.Error(), config.ErrAdaptersRequired) {
		return fmt.Errorf("expected adapters required, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas/config.yaml")); !os.IsNotExist(err) {
		return fmt.Errorf("zero-adapter must not write config")
	}
	_ = homeDir
	return nil
}

func scenarioCursorLocalOnlyMCP(homeDir string) error {
	root, err := gitTempRoot("cursor-lo")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "cursor-lo", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SelectOption("source_control.mode", "git_local")
	_ = draft.SelectOption("source_control.governance_storage", "local_only")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCP()}); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(root, ".cursor/mcp.json"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(raw), "context7") || !strings.Contains(string(raw), "chrome_devtools") {
		return fmt.Errorf("managed MCP missing: %s", raw)
	}
	status := gitStatus(root)
	for _, rel := range []string{".cursor/mcp.json", ".atlas/config.yaml", "AGENTS.md"} {
		if pathInStatus(status, rel) {
			return fmt.Errorf("local_only: %s versionable:\n%s", rel, status)
		}
		if strings.TrimSpace(gitCheckIgnore(root, rel)) == "" {
			return fmt.Errorf("local_only: check-ignore empty for %s", rel)
		}
	}
	_ = homeDir
	return nil
}

func scenarioOpenCodeLocalOnlyMCP(homeDir string) error {
	root, err := gitTempRoot("oc-lo")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if err := os.WriteFile(filepath.Join(root, "opencode.json"), []byte(`{"model":"user/keep"}`+"\n"), 0o644); err != nil {
		return err
	}
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "oc-lo", ProjectMode: "existing", ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_ = draft.SelectOption("source_control.mode", "git_local")
	_ = draft.SelectOption("source_control.governance_storage", "local_only")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCP()}); err != nil {
		return err
	}
	atlasMCP, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(opencode.ConfigRelPath)))
	if err != nil {
		return err
	}
	if !strings.Contains(string(atlasMCP), "context7") || !strings.Contains(string(atlasMCP), "chrome_devtools") {
		return fmt.Errorf("atlas MCP layer missing: %s", atlasMCP)
	}
	rootOC, _ := os.ReadFile(filepath.Join(root, "opencode.json"))
	if strings.Contains(string(rootOC), "context7") {
		return fmt.Errorf("Atlas MCP must not live in root opencode.json: %s", rootOC)
	}
	status := gitStatus(root)
	if pathInStatus(status, opencode.ConfigRelPath) {
		return fmt.Errorf("Atlas MCP layer versionable:\n%s", status)
	}
	if !pathInStatus(status, "opencode.json") {
		return fmt.Errorf("user root opencode.json must stay versionable:\n%s", status)
	}
	gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if strings.Contains(string(gi), "\n.opencode/\n") {
		return fmt.Errorf("must not blanket-ignore .opencode/")
	}
	for _, line := range strings.Split(string(gi), "\n") {
		if strings.TrimSpace(line) == "opencode.json" {
			return fmt.Errorf("must not ignore root opencode.json")
		}
	}
	_ = homeDir
	return nil
}

func scenarioCursorVersioned(homeDir string) error {
	root, err := gitTempRoot("cursor-ver")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "cursor-ver", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SelectOption("source_control.mode", "git_local")
	_ = draft.SelectOption("source_control.governance_storage", "versioned")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCP()}); err != nil {
		return err
	}
	status := gitStatus(root)
	for _, rel := range []string{".atlas/config.yaml", "AGENTS.md", ".cursor/mcp.json"} {
		if !pathInStatus(status, rel) {
			return fmt.Errorf("versioned: %s should be visible:\n%s", rel, status)
		}
	}
	gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if strings.Contains(string(gi), "# >>> Atlas local governance >>>") {
		return fmt.Errorf("versioned must strip managed ignore block")
	}
	_ = homeDir
	return nil
}

func scenarioOpenCodeVersioned(homeDir string) error {
	root, err := gitTempRoot("oc-ver")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "oc-ver", ProjectMode: "existing", ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_ = draft.SelectOption("source_control.mode", "git_local")
	_ = draft.SelectOption("source_control.governance_storage", "versioned")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCP()}); err != nil {
		return err
	}
	status := gitStatus(root)
	for _, rel := range []string{".opencode/atlas.md", opencode.ConfigRelPath, "AGENTS.md"} {
		if !pathInStatus(status, rel) {
			return fmt.Errorf("versioned: %s should be visible:\n%s", rel, status)
		}
	}
	_ = homeDir
	return nil
}

func scenarioCursorToOpenCode(homeDir string) error {
	root, err := os.MkdirTemp("", "atlas-prerelease-cu2oc-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "cu2oc", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCP()
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	user := filepath.Join(root, ".cursor", "rules", "mine.mdc")
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(user, []byte("user-owned\n"), 0o644); err != nil {
		return err
	}
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor/rules/atlas.mdc")); !os.IsNotExist(err) {
		return fmt.Errorf("atlas cursor rule still present")
	}
	got, err := os.ReadFile(user)
	if err != nil || string(got) != "user-owned\n" {
		return fmt.Errorf("user cursor content lost: %v %q", err, got)
	}
	for _, rel := range []string{
		".opencode/atlas.md",
		".opencode/agents/atlas-orchestrator.md",
		".opencode/plugins/atlas-hooks/index.ts",
		".opencode/skills/testing/SKILL.md",
		opencode.ConfigRelPath,
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("incomplete OpenCode reprojection missing %s: %w", rel, err)
		}
	}
	oc, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(opencode.ConfigRelPath)))
	if err != nil || !strings.Contains(string(oc), "context7") || !strings.Contains(string(oc), "chrome_devtools") {
		return fmt.Errorf("opencode managed MCP missing: %v %s", err, oc)
	}
	_ = homeDir
	return nil
}

func scenarioOpenCodeToCursor(homeDir string) error {
	root, err := os.MkdirTemp("", "atlas-prerelease-oc2cu-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "oc2cu", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	mcp := fullMCP()
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	user := filepath.Join(root, ".opencode", "agents", "my-agent.md")
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(user, []byte("user-agent\n"), 0o644); err != nil {
		return err
	}
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode/atlas.md")); !os.IsNotExist(err) {
		return fmt.Errorf("opencode atlas entry still present")
	}
	got, err := os.ReadFile(user)
	if err != nil || string(got) != "user-agent\n" {
		return fmt.Errorf("user opencode agent lost: %v %q", err, got)
	}
	for _, rel := range []string{
		".cursor/rules/atlas.mdc",
		".cursor/agents/atlas-orchestrator.md",
		".cursor/mcp.json",
		".cursor/skills/testing/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("incomplete Cursor reprojection missing %s: %w", rel, err)
		}
	}
	_ = homeDir
	return nil
}

func scenarioAddOpenCode(homeDir string) error {
	root, err := os.MkdirTemp("", "atlas-prerelease-add-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "add", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCP()
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	before, _ := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	after, err := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	if err != nil || string(before) != string(after) {
		return fmt.Errorf("cursor projection churned on add")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(opencode.ConfigRelPath))); err != nil {
		return fmt.Errorf("opencode MCP missing after add: %w", err)
	}
	_ = homeDir
	return nil
}

func scenarioRemoveCursor(homeDir string) error {
	root, err := os.MkdirTemp("", "atlas-prerelease-rm-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "rm", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	mcp := fullMCP()
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	user := filepath.Join(root, ".cursor", "rules", "keep.mdc")
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(user, []byte("keep\n"), 0o644); err != nil {
		return err
	}
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor/rules/atlas.mdc")); !os.IsNotExist(err) {
		return fmt.Errorf("atlas cursor rule not removed")
	}
	got, err := os.ReadFile(user)
	if err != nil || string(got) != "keep\n" {
		return fmt.Errorf("user cursor lost: %v %q", err, got)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(opencode.ConfigRelPath))); err != nil {
		return fmt.Errorf("opencode should remain fully projected: %w", err)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		return err
	}
	if !doc.MCP.Builtins.Context7.Enabled || !doc.MCP.Builtins.ChromeDevTools.Enabled {
		return fmt.Errorf("MCP desired state changed: %#v", doc.MCP.Builtins)
	}
	_ = homeDir
	return nil
}

func scenarioUserOwnedPreserved(homeDir string) error {
	root, err := os.MkdirTemp("", "atlas-prerelease-user-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "user", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCP()}); err != nil {
		return err
	}
	fixtures := map[string]string{
		".cursor/rules/custom.mdc":         "cursor-custom\n",
		".opencode/agents/custom-agent.md": "oc-custom\n",
		"opencode.json":                    "{\n  \"model\": \"user/model\"\n}\n",
		".opencode/skills/custom/SKILL.md": "skill\n",
	}
	for rel, body := range fixtures {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: fullMCP()}); err != nil {
		return err
	}
	for rel, body := range fixtures {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || string(got) != body {
			return fmt.Errorf("fixture %s lost: %v %q", rel, err, got)
		}
	}
	_ = homeDir
	return nil
}

func scenarioLegacyTolerant(homeDir string) error {
	root, err := os.MkdirTemp("", "atlas-prerelease-legacy-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		return err
	}
	yaml := `
schema_version: 1
project:
  name: legacy
  mode: existing
governance:
  workflow: sdd
  spec_engine: openspec
  testing_required: true
  review_required: true
  evidence_required: false
adapters:
  selected: []
source_control:
  mode: git_local
  default_remote: origin
  branch_strategy: manual
  governance_files: local_only
memory:
  strategy: sqlite_plus_context_capsule
mcp:
  builtins:
    filesystem:
      enabled: true
    github:
      enabled: false
    jira:
      enabled: true
    context7:
      enabled: false
    chrome_devtools:
      enabled: false
  custom:
    - name: Old Custom
      transport: streamable_http
      command_or_url: https://example.local/mcp
      enabled: true
`
	if err := os.WriteFile(filepath.Join(root, ".atlas/config.yaml"), []byte(yaml), 0o644); err != nil {
		return err
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		return fmt.Errorf("legacy load: %w", err)
	}
	if !doc.MCP.Builtins.Filesystem.Enabled || !doc.MCP.Builtins.Jira.Enabled || len(doc.MCP.Custom) != 1 {
		return fmt.Errorf("legacy mcp not preserved on load: %#v", doc.MCP)
	}
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "legacy", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	config.ApplyProjectDocument(&draft, doc)
	mcp := doc.ToMCPDraft()
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err == nil {
		return fmt.Errorf("legacy zero adapters must not re-apply")
	}
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		return fmt.Errorf("legacy configure after selecting adapter: %w", err)
	}
	out, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		return err
	}
	if !out.Governance.EvidenceRequired {
		return fmt.Errorf("evidence must normalize on")
	}
	if !out.MCP.Builtins.Filesystem.Enabled || !out.MCP.Builtins.Jira.Enabled {
		return fmt.Errorf("retired builtin flags must be preserved: %#v", out.MCP.Builtins)
	}
	if raw, err := os.ReadFile(filepath.Join(root, ".cursor/mcp.json")); err == nil {
		if strings.Contains(string(raw), "example.local") || strings.Contains(string(raw), "Old Custom") {
			return fmt.Errorf("legacy custom rematerialized: %s", raw)
		}
	}
	_ = homeDir
	return nil
}

func gitTempRoot(suffix string) (string, error) {
	root, err := os.MkdirTemp("", "atlas-prerelease-"+suffix+"-*")
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "-C", root, "init")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(root)
		return "", fmt.Errorf("git init: %v\n%s", err, out)
	}
	return root, nil
}

func gitStatus(root string) string {
	cmd := exec.Command("git", "-C", root, "status", "--short", "--untracked-files=all")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func gitCheckIgnore(root, rel string) string {
	cmd := exec.Command("git", "-C", root, "check-ignore", "-v", rel)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func pathInStatus(status, rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
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
