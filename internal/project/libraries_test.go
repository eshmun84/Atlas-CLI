package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func TestDiscoverLibraries_DetectsCharmbraceletStack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), `module demo

go 1.22

require (
	github.com/charmbracelet/bubbletea v1.0.0
	github.com/charmbracelet/lipgloss v1.0.0
	github.com/charmbracelet/bubbles v1.0.0
)
`)

	libs := project.DiscoverLibraries(root, project.FileInfo{HasGoMod: true})
	got := map[string]bool{}
	for _, lib := range libs {
		got[lib.Name] = true
	}
	for _, want := range []string{"Bubble Tea", "Lip Gloss", "Bubbles"} {
		if !got[want] {
			t.Fatalf("missing library %q in %#v", want, libs)
		}
	}
}

func TestDiscoverRuntimeArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# agents")
	writeFile(t, filepath.Join(root, "CLAUDE.md"), "# claude")
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir .agents: %v", err)
	}

	found := project.DiscoverRuntimeArtifacts(root)
	want := map[string]bool{"AGENTS.md": true, "CLAUDE.md": true, ".agents": true}
	if len(found) != len(want) {
		t.Fatalf("got %#v, want %d entries", found, len(want))
	}
	for _, path := range found {
		if !want[path] {
			t.Fatalf("unexpected artifact %q", path)
		}
	}
}
