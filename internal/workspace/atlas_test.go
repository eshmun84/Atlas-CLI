package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestEvaluateAtlasStatus_NotInitialized(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	status := workspace.EvaluateAtlasStatus(root, workspace.FileInfo{})
	if status.State != workspace.AtlasStateNotInitialized {
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
	partial := workspace.EvaluateAtlasStatus(root, workspace.FileInfo{HasAtlasDir: true})
	if partial.State != workspace.AtlasStatePartialSetup {
		t.Fatalf("partial state = %q", partial.State)
	}

	writeFile(t, filepath.Join(root, ".atlas", "config.yaml"), "atlas: {}\n")
	invalid := workspace.EvaluateAtlasStatus(root, workspace.FileInfo{HasAtlasDir: true, HasAtlasConfig: true})
	if invalid.State != workspace.AtlasStateInvalidConfig {
		t.Fatalf("invalid state = %q", invalid.State)
	}

	cfg := config.DefaultConfig("demo")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".atlas", "config.yaml"), string(data))
	ok := workspace.EvaluateAtlasStatus(root, workspace.FileInfo{HasAtlasDir: true, HasAtlasConfig: true})
	if ok.State != workspace.AtlasStateInitialized {
		t.Fatalf("initialized state = %q", ok.State)
	}
	if !ok.Initialized() {
		t.Fatal("expected initialized")
	}
	if ok.Config.Project.Name != "demo" {
		t.Fatalf("config name = %q", ok.Config.Project.Name)
	}
}
