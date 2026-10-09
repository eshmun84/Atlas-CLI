package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestAtlasOwnedFileExists_Semantics(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}

	present, err := config.AtlasOwnedFileExists(root, config.FileState)
	if err != nil || present {
		t.Fatalf("missing: present=%v err=%v", present, err)
	}

	if err := os.WriteFile(filepath.Join(root, config.FileState), []byte("initialized: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	present, err = config.AtlasOwnedFileExists(root, config.FileState)
	if err != nil || !present {
		t.Fatalf("regular: present=%v err=%v", present, err)
	}

	if err := os.Remove(filepath.Join(root, config.FileState)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, config.FileState), 0o755); err != nil {
		t.Fatal(err)
	}
	present, err = config.AtlasOwnedFileExists(root, config.FileState)
	if err == nil || present {
		t.Fatalf("directory must error: present=%v err=%v", present, err)
	}
	if !strings.Contains(err.Error(), "regular") {
		t.Fatalf("err=%v", err)
	}

	outside := t.TempDir()
	ext := filepath.Join(outside, "state.yaml")
	if err := os.WriteFile(ext, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, config.FileState)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(root, config.FileState)); err != nil {
		t.Fatal(err)
	}
	present, err = config.AtlasOwnedFileExists(root, config.FileState)
	if err == nil || present {
		t.Fatalf("symlink must error: present=%v err=%v", present, err)
	}
}

func TestEvaluateHealth_DirectoryAtState_Unreadable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.FileConfig), []byte(`schema_version: 1
project:
  name: dir-state
  mode: existing
adapters:
  selected: [cursor]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, config.FileState), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	h := runtime.EvaluateHealth(root, atlas, files)
	if h.StateLoads {
		t.Fatal("directory-at-state must not load as ordinary state")
	}
	if h.StateError == "" || !strings.Contains(h.StateError, "regular") {
		t.Fatalf("StateError=%q", h.StateError)
	}
}

func TestEvaluateHealth_DirectoryAtConfig_Unreadable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, config.FileConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	h := runtime.EvaluateHealth(root, atlas, files)
	if h.ConfigLoads {
		t.Fatal("directory-at-config must not load")
	}
	if h.ConfigExists && h.ConfigError == "" {
		t.Fatal("directory-at-config must report ConfigError when treated as present")
	}
}
