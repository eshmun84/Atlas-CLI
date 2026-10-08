package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"gopkg.in/yaml.v3"
)

func TestEvaluateAtlasStatus_NotInitialized(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	status := project.EvaluateAtlasStatus(root, project.FileInfo{})
	if status.State != project.AtlasStateNotInitialized {
		t.Fatalf("state = %q", status.State)
	}
	if status.Initialized() {
		t.Fatal("expected not initialized")
	}
}

func TestEvaluateAtlasStatus_PartialAndInvalidAndInitialized(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := project.EvaluateAtlasStatus(root, project.FileInfo{HasAtlasDir: true})
	if partial.State != project.AtlasStatePartialSetup {
		t.Fatalf("partial state = %q", partial.State)
	}

	writeFile(t, filepath.Join(root, ".atlas", "config.yaml"), "atlas: {}\n")
	invalid := project.EvaluateAtlasStatus(root, project.FileInfo{HasAtlasDir: true, HasAtlasConfig: true})
	if invalid.State != project.AtlasStateInvalidConfig {
		t.Fatalf("invalid state = %q", invalid.State)
	}

	cfg := config.DefaultConfig("demo")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".atlas", "config.yaml"), string(data))
	ok := project.EvaluateAtlasStatus(root, project.FileInfo{HasAtlasDir: true, HasAtlasConfig: true})
	if ok.State != project.AtlasStateInitialized {
		t.Fatalf("initialized state = %q", ok.State)
	}
	if !ok.Initialized() {
		t.Fatal("expected initialized")
	}
	if ok.Config.Project.Name != "demo" {
		t.Fatalf("config name = %q", ok.Config.Project.Name)
	}
}
