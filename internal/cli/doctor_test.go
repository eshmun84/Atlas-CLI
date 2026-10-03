package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestRunDoctor_ReportSectionsAndWarnings(t *testing.T) {
	root := t.TempDir()

	restore := stubDoctor(root, func(path string) (workspace.DiscoveryResult, error) {
		return workspace.DiscoveryResult{
			RootPath: path,
			Git: workspace.GitInfo{
				IsRepo:        true,
				CurrentBranch: "develop",
				Remotes: []workspace.GitRemote{
					{Name: "origin", URL: "git@example.com:demo.git"},
				},
			},
			Tools: []workspace.ToolInfo{
				{Name: "git", Available: true},
				{Name: "go", Available: true},
				{Name: "gh", Available: false},
			},
		}, nil
	})
	defer restore()

	var stdout, stderr bytes.Buffer
	err := Execute(&stdout, &stderr, []string{"doctor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, "diagnostics are not implemented yet") {
		t.Fatal("doctor still prints placeholder")
	}

	for _, section := range []string{
		"Atlas Doctor",
		"Checks:",
		"Summary:",
		"WARN atlas config: .atlas/config.yaml not found",
		"WARN agents file: AGENTS.md not found",
		"Result: ready with warnings",
	} {
		if !strings.Contains(out, section) {
			t.Fatalf("missing %q in output:\n%s", section, out)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRunDoctor_DoesNotCreateFiles(t *testing.T) {
	root := t.TempDir()
	restore := stubDoctor(root, workspace.Discover)
	defer restore()

	var stdout bytes.Buffer
	if err := Execute(&stdout, nil, []string{"doctor"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not be created, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not be created, stat err = %v", err)
	}
}

func TestRunDoctor_DiscoveryFailureExitCode(t *testing.T) {
	restore := stubDoctor("/tmp", func(string) (workspace.DiscoveryResult, error) {
		return workspace.DiscoveryResult{}, errors.New("boom")
	})
	defer restore()

	var stdout bytes.Buffer
	err := Execute(&stdout, nil, []string{"doctor"})
	if !errors.Is(err, doctor.ErrUnhealthy) {
		t.Fatalf("expected ErrUnhealthy, got %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "FAIL workspace: discovery failed") {
		t.Fatalf("expected FAIL check in output:\n%s", out)
	}
	if !strings.Contains(out, "Result: not ready") {
		t.Fatalf("expected not ready summary:\n%s", out)
	}
}

func TestRunDoctor_WorkingDirectoryFailure(t *testing.T) {
	prevGetwd := doctorGetwd
	prevDiscover := doctorDiscover
	doctorGetwd = func() (string, error) {
		return "", errors.New("no cwd")
	}
	doctorDiscover = workspace.Discover
	defer func() {
		doctorGetwd = prevGetwd
		doctorDiscover = prevDiscover
	}()

	var stdout bytes.Buffer
	err := Execute(&stdout, nil, []string{"doctor"})
	if !errors.Is(err, doctor.ErrUnhealthy) {
		t.Fatalf("expected ErrUnhealthy, got %v", err)
	}
	if !strings.Contains(stdout.String(), "current directory cannot be resolved") {
		t.Fatalf("expected cwd failure in output:\n%s", stdout.String())
	}
}

func stubDoctor(root string, discover func(string) (workspace.DiscoveryResult, error)) func() {
	prevGetwd := doctorGetwd
	prevDiscover := doctorDiscover

	doctorGetwd = func() (string, error) {
		return root, nil
	}
	doctorDiscover = discover

	return func() {
		doctorGetwd = prevGetwd
		doctorDiscover = prevDiscover
	}
}
