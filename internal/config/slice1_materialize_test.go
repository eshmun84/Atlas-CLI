package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func TestApplyConfig_Slice1_GreenfieldMaterialization(t *testing.T) {
	withTempAtlasHome(t)
	config.SetLookPathGitForTest(exec.LookPath)
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice1-Demo", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("source_control.mode", "git_local") {
		t.Fatal("git_local")
	}

	result, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.HomeCreated {
		t.Fatal("Init must not create Atlas Home")
	}

	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(readme) != "# Slice1-Demo\n" {
		t.Fatalf("README = %q err=%v", readme, err)
	}
	for _, banned := range []string{"Atlas", "Cursor", "OpenSpec", "workflow", "Atlas Home", "Memory"} {
		if strings.Contains(string(readme), banned) {
			t.Fatalf("README leaked %q", banned)
		}
	}

	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	wantBlock := config.AtlasLocalGovernanceGitignoreBlock()
	if string(ignore) != wantBlock {
		t.Fatalf("gitignore =\n%s\nwant\n%s", ignore, wantBlock)
	}

	for _, rel := range []string{"openspec", "openspec/changes"} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s missing: %v", rel, err)
		}
	}

	gitInfo, err := project.DiscoverGit(root)
	if err != nil || !gitInfo.IsRepo {
		t.Fatalf("git repo: %#v %v", gitInfo, err)
	}
	head, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil || !strings.Contains(string(head), "refs/heads/main") {
		t.Fatalf("HEAD = %q err=%v", head, err)
	}
	out, err := exec.Command("git", "-C", root, "rev-list", "--all", "--count").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "0" {
		t.Fatalf("expected zero commits, got %s", out)
	}
	remotes, err := exec.Command("git", "-C", root, "remote").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(remotes)) != "" {
		t.Fatalf("unexpected remotes: %s", remotes)
	}

	// Idempotent re-apply
	beforeIgnore := string(ignore)
	beforeReadme := string(readme)
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: true,
		Now: func() time.Time { return time.Date(2026, 10, 14, 15, 5, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	ignore2, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	readme2, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(ignore2) != beforeIgnore {
		t.Fatalf("gitignore drifted on re-apply:\n%s", ignore2)
	}
	if string(readme2) != beforeReadme {
		t.Fatalf("README drifted on re-apply:\n%s", readme2)
	}
	if strings.Count(string(ignore2), "# >>> Atlas local governance >>>") != 1 {
		t.Fatal("gitignore block duplicated")
	}
}

func TestApplyConfig_Slice1_BrownfieldPreserves(t *testing.T) {
	withTempAtlasHome(t)
	config.SetLookPathGitForTest(exec.LookPath)
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })
	root := t.TempDir()
	userReadme := "# Owned README\nkeep secrets out\n"
	userIgnore := "bin/\n.tmp/\n"
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(userReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(userIgnore), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := []byte("proposal keep\n")
	if err := os.WriteFile(filepath.Join(root, "openspec", "changes", "existing", "proposal.md"), marker, 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "-b", "develop")
	runGit(t, root, "remote", "add", "origin", "https://example.invalid/repo.git")

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Brown", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("source_control.mode", "git_github") {
		t.Fatal("git_github")
	}

	result, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 16, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	readme, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(readme) != userReadme {
		t.Fatalf("README mutated: %q", readme)
	}
	gotMarker, _ := os.ReadFile(filepath.Join(root, "openspec", "changes", "existing", "proposal.md"))
	if string(gotMarker) != string(marker) {
		t.Fatal("openspec mutated")
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ignore), userIgnore) {
		t.Fatalf("project rules lost: %s", ignore)
	}
	if !strings.Contains(string(ignore), config.AtlasLocalGovernanceGitignoreBlock()) {
		t.Fatalf("missing atlas block: %s", ignore)
	}
	if result.BackupDir == "" {
		t.Fatal("expected gitignore backup")
	}
	if _, err := os.Stat(filepath.Join(result.BackupDir, ".gitignore")); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	branch, _ := exec.Command("git", "-C", root, "branch", "--show-current").CombinedOutput()
	if strings.TrimSpace(string(branch)) != "develop" {
		t.Fatalf("branch changed: %s", branch)
	}
	remote, _ := exec.Command("git", "-C", root, "remote", "get-url", "origin").CombinedOutput()
	if !strings.Contains(string(remote), "example.invalid") {
		t.Fatalf("remote changed: %s", remote)
	}

	// Second apply: no gitignore byte change, no duplicate block.
	before := string(ignore)
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: true,
		Now: func() time.Time { return time.Date(2026, 10, 14, 16, 5, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if string(after) != before {
		t.Fatalf("gitignore bytes changed on idempotent re-apply")
	}
}

func TestApplyConfig_OpenSpecPartial_FailClosed(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := []byte("do-not-touch\n")
	if err := os.WriteFile(filepath.Join(root, "openspec", "x.txt"), marker, 0o644); err != nil {
		t.Fatal(err)
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "PartialOS", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 17, 0, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "openspec") ||
		!strings.Contains(strings.ToLower(err.Error()), "malformed") {
		t.Fatalf("expected openspec malformed fail-closed, got %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "openspec", "x.txt"))
	if string(data) != string(marker) {
		t.Fatal("partial openspec mutated")
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatal("must not materialize project after openspec failure")
	}
}
