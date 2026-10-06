package config_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

func TestDefaultConfigIsValid(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}

	if cfg.Project.Name != "demo" {
		t.Fatalf("project name = %q, want demo", cfg.Project.Name)
	}
	if cfg.Atlas.Version != version.Version {
		t.Fatalf("atlas.version = %q, want %q", cfg.Atlas.Version, version.Version)
	}
	if cfg.Governance.StorageMode != config.StorageModeLocal {
		t.Fatalf("governance.storage_mode = %q, want %q", cfg.Governance.StorageMode, config.StorageModeLocal)
	}
	if cfg.Orchestration.DefaultWorkflow != config.WorkflowSDD {
		t.Fatalf("default_workflow = %q, want %q", cfg.Orchestration.DefaultWorkflow, config.WorkflowSDD)
	}
	if cfg.Memory.Provider != config.MemoryProviderSQLite {
		t.Fatalf("memory.provider = %q, want %q", cfg.Memory.Provider, config.MemoryProviderSQLite)
	}
	if cfg.Assets.Source != config.AssetsSourceEmbedded {
		t.Fatalf("assets.source = %q, want %q", cfg.Assets.Source, config.AssetsSourceEmbedded)
	}
	if !cfg.Materialization.DryRunRequired {
		t.Fatal("materialization.dry_run_required should be true")
	}
}

func TestPathConstants(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"DirAtlas":                     config.DirAtlas,
		"FileConfig":                   config.FileConfig,
		"FileLocal":                    config.FileLocal,
		"FileState":                    config.FileState,
		"FileMemoryDB":                 config.FileMemoryDB,
		"FileCapsule":                  config.FileCapsule,
		"FileAssetsLock":               config.FileAssetsLock,
		"FileAgentRegistry":            config.FileAgentRegistry,
		"FileRuntimeManifest":          config.FileRuntimeManifest,
		"FileSDDOpenSpecContract":      config.FileSDDOpenSpecContract,
		"EmbedPathSDDOpenSpecContract": config.EmbedPathSDDOpenSpecContract,
		"DirCursorAgents":              config.DirCursorAgents,
		"DirOpenCodeAgents":            config.DirOpenCodeAgents,
	}

	want := map[string]string{
		"DirAtlas":                     ".atlas",
		"FileConfig":                   ".atlas/config.yaml",
		"FileLocal":                    ".atlas/local.yaml",
		"FileState":                    ".atlas/state.yaml",
		"FileMemoryDB":                 ".atlas/memory/atlas.sqlite",
		"FileCapsule":                  ".atlas/context/memory-capsule.md",
		"FileAssetsLock":               ".atlas/assets.lock.yaml",
		"FileAgentRegistry":            ".atlas/agent-registry.md",
		"FileRuntimeManifest":          ".atlas/runtime-manifest.yaml",
		"FileSDDOpenSpecContract":      ".atlas/contracts/sdd-openspec.md",
		"EmbedPathSDDOpenSpecContract": "contracts/sdd-openspec.md",
		"DirCursorAgents":              ".cursor/agents",
		"DirOpenCodeAgents":            ".opencode/agents",
	}

	for name, got := range cases {
		if got != want[name] {
			t.Fatalf("%s = %q, want %q", name, got, want[name])
		}
	}
}
