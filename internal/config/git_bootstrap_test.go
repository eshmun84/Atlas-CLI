package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestRequireGitForBootstrap_Available(t *testing.T) {
	root := t.TempDir()
	config.SetLookPathGitForTest(func(file string) (string, error) {
		if file != "git" {
			t.Fatalf("LookPath(%q)", file)
		}
		return "/usr/bin/git", nil
	})
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })

	need, err := config.NeedsGitBootstrap(root, config.SourceControlGitLocal)
	if err != nil || !need {
		t.Fatalf("need=%v err=%v", need, err)
	}
	if err := config.RequireGitForBootstrap(root, config.SourceControlGitLocal); err != nil {
		t.Fatalf("preflight: %v", err)
	}
}

func TestRequireGitForBootstrap_MissingBlocks(t *testing.T) {
	root := t.TempDir()
	config.SetLookPathGitForTest(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })

	need, err := config.NeedsGitBootstrap(root, config.SourceControlGitGitHub)
	if err != nil || !need {
		t.Fatalf("need=%v err=%v", need, err)
	}
	err = config.RequireGitForBootstrap(root, config.SourceControlGitGitHub)
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("expected PATH blocker, got %v", err)
	}
}

func TestRequireGitForBootstrap_ExistingRepoNoPreflight(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	config.SetLookPathGitForTest(func(string) (string, error) {
		return "", errors.New("git absent")
	})
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })

	need, err := config.NeedsGitBootstrap(root, config.SourceControlGitLocal)
	if err != nil || need {
		t.Fatalf("existing .git must not need bootstrap: need=%v err=%v", need, err)
	}
	if err := config.RequireGitForBootstrap(root, config.SourceControlGitLocal); err != nil {
		t.Fatalf("preflight must not block existing repo: %v", err)
	}
}

func TestApplyConfig_GitMissingDoesNotMutate(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	config.SetLookPathGitForTest(func(string) (string, error) {
		return "", errors.New("not found")
	})
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "NoGit", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("source_control.mode", "git_local") {
		t.Fatal("git_local")
	}

	before, err := snapshotRelPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 16, 0, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("expected git PATH error, got %v", err)
	}
	after, err := snapshotRelPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("workspace mutated on git-missing preflight\nbefore=%s\nafter=%s", before, after)
	}
	if _, err := os.Lstat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git must not exist: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas must not exist: %v", err)
	}
}

func snapshotRelPaths(root string) (string, error) {
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		b.WriteString(filepath.ToSlash(rel))
		if info.IsDir() {
			b.WriteByte('/')
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String(), err
}
