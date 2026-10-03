package config_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestClassifyKey(t *testing.T) {
	t.Parallel()

	cases := map[string]config.Mutability{
		"project.name":                           config.Immutable,
		"project.mode":                           config.Immutable,
		"atlas.version":                          config.Immutable,
		"adapters":                               config.Mutable,
		"adapters.cursor":                        config.Mutable,
		"source_control":                         config.Mutable,
		"source_control.git_enabled":             config.Mutable,
		"assets":                                 config.Mutable,
		"registry":                               config.Mutable,
		"external_context_providers":             config.Mutable,
		"external_context_providers.mcp_enabled": config.Mutable,
		"materialization":                        config.Mutable,
		"materialization.overwrite_policy":       config.Mutable,
		"governance.storage_mode":                config.MigrationRequired,
		"spec_engine.provider":                   config.MigrationRequired,
		"memory.provider":                        config.MigrationRequired,
		"orchestration.default_workflow":         config.MigrationRequired,
		"project.description":                    config.Unknown,
		"governance.enabled":                     config.Unknown,
	}

	for path, want := range cases {
		got := config.ClassifyKey(path)
		if got != want {
			t.Fatalf("ClassifyKey(%q) = %q, want %q", path, got, want)
		}
	}
}
