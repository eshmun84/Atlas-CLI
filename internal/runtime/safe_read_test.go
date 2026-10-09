package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestEvaluateHealth_AtlasDirSymlinkNotParsed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	evil := []byte("schema_version: 1\nproject:\n  name: EXTERNAL\n  mode: existing\nadapters:\n  selected: [cursor]\n")
	if err := os.WriteFile(filepath.Join(outside, ".atlas", "config.yaml"), evil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, ".atlas"), filepath.Join(root, ".atlas")); err != nil {
		t.Fatal(err)
	}
	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	h := runtime.EvaluateHealth(root, atlas, files)
	if h.ConfigLoads {
		t.Fatal("must not load config through .atlas symlink")
	}
	if h.ConfigError == "" || !strings.Contains(strings.ToLower(h.ConfigError), "symlink") {
		t.Fatalf("ConfigError=%q", h.ConfigError)
	}
	if h.Document.Project.Name == "EXTERNAL" {
		t.Fatal("external config must not be parsed")
	}
}

func TestEvaluateHealth_AgentsSymlinkNotParsed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ext := filepath.Join(outside, "AGENTS.md")
	if err := os.WriteFile(ext, []byte("# EXTERNAL AGENTS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(root, config.FileAgentsMD)); err != nil {
		t.Fatal(err)
	}
	files, _ := project.DiscoverFiles(root)
	h := runtime.EvaluateHealth(root, project.AtlasStatus{}, files)
	if h.AgentsError == "" {
		t.Fatal("expected AgentsError for symlink leaf")
	}
	if h.AgentsMarkers.BaseBegin {
		t.Fatal("must not parse external AGENTS.md")
	}
}

func TestEvaluateHealth_DirectoryAtRuntimeManifest_NotOrdinaryPresent(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	rel := config.FileRuntimeManifest
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	files := mustFiles(t, root)
	atlas := project.EvaluateAtlasStatus(root, files)
	h := runtime.EvaluateHealth(root, atlas, files)
	if h.RuntimeManifestPresent {
		t.Fatal("directory-at-runtime-manifest must not count as ordinary present file")
	}
	if h.RuntimeManifestMatches {
		t.Fatal("directory must not match as runtime manifest")
	}
}

func TestEvaluateHealth_AdapterRuntimeSymlinkNotPresent(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	outside := t.TempDir()
	ext := filepath.Join(outside, "atlas.mdc")
	if err := os.WriteFile(ext, []byte("EXTERNAL RULE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(config.FileCursorAtlasMDC))
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, target); err != nil {
		t.Fatal(err)
	}
	files := mustFiles(t, root)
	atlas := project.EvaluateAtlasStatus(root, files)
	h := runtime.EvaluateHealth(root, atlas, files)
	for _, p := range h.ExpectedProjections {
		if p.Path == config.FileCursorAtlasMDC && p.Present {
			t.Fatalf("symlink adapter runtime must not count as present: %#v", p)
		}
	}
}
