package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestRunStatus_ReportSections(t *testing.T) {
	root := t.TempDir()

	restore := stubStatus(root, func(path string) (workspace.DiscoveryResult, error) {
		return workspace.DiscoveryResult{
			RootPath: path,
			Git: workspace.GitInfo{
				IsRepo:        true,
				CurrentBranch: "develop",
				Remotes: []workspace.GitRemote{
					{Name: "origin", URL: "git@github.com:eshmun84/Atlas-CLI.git"},
				},
			},
			Files: workspace.FileInfo{
				HasReadme:    true,
				HasGitignore: true,
				HasMakefile:  true,
				HasGoMod:     true,
			},
			Technologies: []workspace.Technology{
				{Name: "Go", Source: "go.mod", Confidence: workspace.ConfidenceHigh},
			},
			Tools: []workspace.ToolInfo{
				{Name: "git", Available: true, Path: "/usr/bin/git"},
				{Name: "go", Available: true, Path: "/usr/local/bin/go"},
				{Name: "gh", Available: true, Path: "/usr/local/bin/gh"},
				{Name: "openspec", Available: false},
				{Name: "cursor", Available: false},
				{Name: "opencode", Available: false},
			},
		}, nil
	})
	defer restore()

	var stdout, stderr bytes.Buffer
	err := Execute(&stdout, &stderr, []string{"status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, "status inspection is not implemented yet") {
		t.Fatal("status still prints placeholder")
	}

	for _, section := range []string{
		"Atlas Status",
		"Workspace:",
		"Git:",
		"Files:",
		"Technologies:",
		"Tools:",
	} {
		if !strings.Contains(out, section) {
			t.Fatalf("missing section %q in output:\n%s", section, out)
		}
	}

	checks := []string{
		"Root: " + root,
		"Repository: yes",
		"Branch: develop",
		"- origin git@github.com:eshmun84/Atlas-CLI.git",
		"README.md: yes",
		".gitignore: yes",
		"Makefile: yes",
		"go.mod: yes",
		"AGENTS.md: no",
		".atlas/: no",
		".atlas/config.yaml: no",
		"- Go (go.mod, high)",
		"git: available",
		"openspec: unavailable",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Warnings:") {
		t.Fatalf("did not expect warnings section:\n%s", out)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRunStatus_DoesNotCreateFiles(t *testing.T) {
	root := t.TempDir()
	restore := stubStatus(root, workspace.Discover)
	defer restore()

	var stdout bytes.Buffer
	if err := Execute(&stdout, nil, []string{"status"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not be created, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not be created, stat err = %v", err)
	}
	if strings.Contains(stdout.String(), "status inspection is not implemented yet") {
		t.Fatal("status still prints placeholder")
	}
}

func TestRunStatus_DiscoveryError(t *testing.T) {
	restore := stubStatus("/tmp", func(string) (workspace.DiscoveryResult, error) {
		return workspace.DiscoveryResult{}, errors.New("boom")
	})
	defer restore()

	var stdout bytes.Buffer
	err := Execute(&stdout, nil, []string{"status"})
	if err == nil {
		t.Fatal("expected discovery error")
	}
	if !strings.Contains(err.Error(), "status discovery failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteStatusReport_EmptyTechRemotesAndWarnings(t *testing.T) {
	t.Parallel()

	result := workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo: false,
		},
		Tools: []workspace.ToolInfo{
			{Name: "git", Available: true, Path: "/usr/bin/git"},
			{Name: "openspec", Available: false},
		},
		Warnings: []string{"git is not available on PATH"},
	}

	var buf bytes.Buffer
	writeStatusReport(&buf, result)
	out := buf.String()

	if !strings.Contains(out, "Remotes:\n    none detected") {
		t.Fatalf("expected none detected remotes:\n%s", out)
	}
	if !strings.Contains(out, "Technologies:\n  none detected") {
		t.Fatalf("expected none detected technologies:\n%s", out)
	}
	if !strings.Contains(out, "git: available") || !strings.Contains(out, "openspec: unavailable") {
		t.Fatalf("expected tool availability lines:\n%s", out)
	}
	if !strings.Contains(out, "Warnings:\n  - git is not available on PATH") {
		t.Fatalf("expected warnings section:\n%s", out)
	}
}

func TestWriteStatusReport_EmptyBranchPrintsNone(t *testing.T) {
	t.Parallel()

	result := workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "",
		},
	}

	var buf bytes.Buffer
	writeStatusReport(&buf, result)
	out := buf.String()

	if !strings.Contains(out, "Repository: yes") {
		t.Fatalf("expected repository yes:\n%s", out)
	}
	if !strings.Contains(out, "Branch: none") {
		t.Fatalf("expected Branch: none for empty branch:\n%s", out)
	}
	if strings.Contains(out, "Branch: \n") {
		t.Fatalf("branch line must not be blank:\n%s", out)
	}
}

func TestDisplayBranch(t *testing.T) {
	t.Parallel()

	if got := displayBranch("develop"); got != "develop" {
		t.Fatalf("displayBranch(develop) = %q, want develop", got)
	}
	if got := displayBranch(""); got != "none" {
		t.Fatalf("displayBranch(\"\") = %q, want none", got)
	}
	if got := displayBranch("   "); got != "none" {
		t.Fatalf("displayBranch(whitespace) = %q, want none", got)
	}
}

func stubStatus(root string, discover func(string) (workspace.DiscoveryResult, error)) func() {
	prevGetwd := statusGetwd
	prevDiscover := statusDiscover

	statusGetwd = func() (string, error) {
		return root, nil
	}
	statusDiscover = discover

	return func() {
		statusGetwd = prevGetwd
		statusDiscover = prevDiscover
	}
}
