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

func TestRunInit_PrintsDryRunPlan(t *testing.T) {
	root := t.TempDir()
	named := filepath.Join(root, "Atlas-CLI")
	if err := os.Mkdir(named, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(named, "README.md"), []byte("# demo"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	restore := stubInit(named, func(path string) (workspace.DiscoveryResult, error) {
		return workspace.DiscoveryResult{
			RootPath: path,
			Files:    workspace.FileInfo{HasReadme: true},
		}, nil
	})
	defer restore()

	for _, args := range [][]string{{"init"}, {"init", "--dry-run"}} {
		var stdout, stderr bytes.Buffer
		err := Execute(&stdout, &stderr, args)
		if err != nil {
			t.Fatalf("%v: unexpected error: %v", args, err)
		}

		out := stdout.String()
		if strings.Contains(out, "project initialization is not implemented yet") {
			t.Fatalf("%v still prints placeholder", args)
		}
		for _, want := range []string{
			"Atlas Init Plan",
			"Project:",
			"Name: Atlas-CLI",
			"Mode: existing",
			"Planned Files:",
			"create AGENTS.md",
			"create .atlas/config.yaml",
			"No files were created.",
			"Result:",
			"ready to initialize",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("%v missing %q in output:\n%s", args, want, out)
			}
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v expected empty stderr, got %q", args, stderr.String())
		}
	}
}

func TestRunInit_DoesNotCreateFiles(t *testing.T) {
	root := t.TempDir()
	restore := stubInit(root, workspace.Discover)
	defer restore()

	var stdout bytes.Buffer
	if err := Execute(&stdout, nil, []string{"init"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not be created, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not be created, stat err = %v", err)
	}
}

func TestRunInit_DiscoveryError(t *testing.T) {
	restore := stubInit("/tmp", func(string) (workspace.DiscoveryResult, error) {
		return workspace.DiscoveryResult{}, errors.New("boom")
	})
	defer restore()

	var stdout bytes.Buffer
	err := Execute(&stdout, nil, []string{"init"})
	if err == nil {
		t.Fatal("expected discovery error")
	}
	if !strings.Contains(err.Error(), "init discovery failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInit_WorkingDirectoryError(t *testing.T) {
	prevGetwd := initGetwd
	initGetwd = func() (string, error) {
		return "", errors.New("no cwd")
	}
	defer func() { initGetwd = prevGetwd }()

	err := Execute(nil, nil, []string{"init"})
	if err == nil {
		t.Fatal("expected cwd error")
	}
	if !strings.Contains(err.Error(), "resolve working directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stubInit(root string, discover func(string) (workspace.DiscoveryResult, error)) func() {
	prevGetwd := initGetwd
	prevDiscover := initDiscover

	initGetwd = func() (string, error) {
		return root, nil
	}
	initDiscover = discover

	return func() {
		initGetwd = prevGetwd
		initDiscover = prevDiscover
	}
}
