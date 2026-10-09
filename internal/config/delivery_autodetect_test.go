package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func TestDeliveryAutodetect_GitRepoFromDiscovery(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())

	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "branch", "-M", "develop")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Atlas Git Autodetect Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "chore: seed project")

	before := gitStatusShort(t, root)

	result, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Git.IsRepo {
		t.Fatal("discovery must report IsRepo=true")
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      filepath.Base(root),
		ProjectMode:      "existing",
		GitRepoDetected:  result.Git.IsRepo,
		DefaultRemote:    result.Git.DefaultRemote,
		ToolGitAvailable: true,
	})
	assertField(t, draft, "source_control.mode", config.SourceControlGitLocal, config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "source_control.delivery_assist", "false", config.FieldEditable, config.FieldEditable)

	afterDiscover := gitStatusShort(t, root)
	if afterDiscover != before {
		t.Fatalf("discovery/default selection mutated git status:\nbefore=%q\nafter=%q", before, afterDiscover)
	}

	_, err = config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	refreshed, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed.Runtime.ConfigLoads {
		t.Fatal("expected config to load after apply")
	}
	if refreshed.Runtime.Document.SourceControl.Mode != config.SourceControlGitLocal {
		t.Fatalf("runtime mode = %q, want git_local", refreshed.Runtime.Document.SourceControl.Mode)
	}
	if refreshed.Runtime.Document.SourceControl.DeliveryAssist {
		t.Fatal("assisted operations must stay false")
	}
	if refreshed.Git.CurrentBranch != "develop" {
		t.Fatalf("branch mutated: %q", refreshed.Git.CurrentBranch)
	}

	report := doctor.Evaluate(refreshed)
	status := screens.StatusWithReport(refreshed, report)
	docView := screens.Doctor(report, refreshed)
	if !strings.Contains(status, "Atlas Status") {
		t.Fatalf("status render failed:\n%s", status)
	}
	if !strings.Contains(docView, "Atlas Doctor") {
		t.Fatalf("doctor render failed:\n%s", docView)
	}
	if strings.Contains(status, "Code Intelligence") || strings.Contains(docView, "Code Intelligence") {
		// optional: presence is fine either way depending on PATH
	}
}

func TestDeliveryAutodetect_NoGitStaysNone(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# no git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTreeFiles(t, root)

	result, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Git.IsRepo {
		t.Fatal("expected IsRepo=false")
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:     "fresh",
		ProjectMode:     "new",
		GitRepoDetected: result.Git.IsRepo,
	})
	assertField(t, draft, "source_control.mode", config.SourceControlNone, config.FieldEditable, config.FieldEditable)

	assertTreeFilesUnchanged(t, root, before)
}

func TestDeliveryAutodetect_NewProjectWithoutGitInitStillWorks(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:     "brand-new",
		ProjectMode:     "new",
		GitRepoDetected: false,
	})
	assertField(t, draft, "source_control.mode", config.SourceControlNone, config.FieldEditable, config.FieldEditable)
	doc := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	if doc.SourceControl.Mode != config.SourceControlNone {
		t.Fatalf("mode = %q", doc.SourceControl.Mode)
	}
	if err := config.ValidateProjectDocument(doc); err != nil {
		t.Fatalf("fresh init document invalid: %v", err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitStatusShort(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	return string(out)
}

func snapshotTreeFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertTreeFilesUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotTreeFiles(t, root)
	if len(before) != len(after) {
		t.Fatalf("tree size changed: before=%d after=%d", len(before), len(after))
	}
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("path mutated: %s", path)
		}
	}
}
