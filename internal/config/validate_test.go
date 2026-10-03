package config_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestValidate_MissingProjectName(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Project.Name = ""

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for missing project name")
	}
	if !strings.Contains(err.Error(), "project.name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidProjectMode(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Project.Mode = "legacy"

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid project mode")
	}
	if !strings.Contains(err.Error(), "project.mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidGovernanceStorageMode(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Governance.StorageMode = "cloud"

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid governance storage mode")
	}
	if !strings.Contains(err.Error(), "governance.storage_mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidMemoryProvider(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Memory.Provider = "redis"

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid memory provider")
	}
	if !strings.Contains(err.Error(), "memory.provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_MemoryEnabledEmptyProvider(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Memory.Provider = ""

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty memory provider")
	}
	if !strings.Contains(err.Error(), "memory.provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RegistryEnabledWithoutDefaultSource(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Registry.Enabled = true
	cfg.Registry.DefaultSource = ""

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for registry without default source")
	}
	if !strings.Contains(err.Error(), "registry.default_source") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_SourceControlEnabledWithoutProvider(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.SourceControl.Enabled = true
	cfg.SourceControl.Provider = ""

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for source control without provider")
	}
	if !strings.Contains(err.Error(), "source_control.provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_SpecEngineEnabledWithoutProvider(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.SpecEngine.Enabled = true
	cfg.SpecEngine.Provider = ""

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for spec engine without provider")
	}
	if !strings.Contains(err.Error(), "spec_engine.provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidMaterializationPolicies(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Materialization.OverwritePolicy = "force"
	if err := config.Validate(cfg); err == nil || !strings.Contains(err.Error(), "overwrite_policy") {
		t.Fatalf("expected overwrite_policy error, got %v", err)
	}

	cfg = config.DefaultConfig("demo")
	cfg.Materialization.BackupPolicy = "always"
	if err := config.Validate(cfg); err == nil || !strings.Contains(err.Error(), "backup_policy") {
		t.Fatalf("expected backup_policy error, got %v", err)
	}
}

func TestValidate_InvalidDefaultWorkflow(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig("demo")
	cfg.Orchestration.DefaultWorkflow = "chaos"

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid default workflow")
	}
	if !strings.Contains(err.Error(), "orchestration.default_workflow") {
		t.Fatalf("unexpected error: %v", err)
	}
}
