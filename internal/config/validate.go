package config

import (
	"fmt"
	"strings"
)

// Allowed project modes.
const (
	ModeGreenfield = "greenfield"
	ModeExisting   = "existing"
)

// Allowed governance storage modes.
const (
	StorageModeLocal   = "local"
	StorageModeTracked = "tracked"
	StorageModeHybrid  = "hybrid"
)

// Allowed orchestration workflows.
const (
	WorkflowSDD        = "sdd"
	WorkflowGuided     = "guided"
	WorkflowReviewOnly = "review-only"
	WorkflowNone       = "none"
)

// Allowed spec engine providers.
const (
	SpecProviderOpenSpec = "openspec"
	SpecProviderNone     = "none"
	SpecProviderOther    = "other"
)

// Allowed memory providers.
const (
	MemoryProviderSQLite = "sqlite"
)

// Allowed assets sources.
const (
	AssetsSourceEmbedded = "embedded"
	AssetsSourceRegistry = "registry"
	AssetsSourceMixed    = "mixed"
)

// Allowed source control providers.
const (
	SourceControlGit    = "git"
	SourceControlGitHub = "github"
	SourceControlNone   = "none"
)

// Allowed materialization overwrite policies.
const (
	OverwriteAsk                 = "ask"
	OverwriteSkip                = "skip"
	OverwriteOverwrite           = "overwrite"
	OverwriteBackupThenOverwrite = "backup-then-overwrite"
)

// Allowed materialization backup policies.
const (
	BackupNone            = "none"
	BackupBeforeOverwrite = "before-overwrite"
)

// Validate checks that cfg conforms to the current Atlas MVP rules.
func Validate(cfg Config) error {
	var errs []string

	if strings.TrimSpace(cfg.Project.Name) == "" {
		errs = append(errs, "project.name is required")
	}

	if !oneOf(cfg.Project.Mode, ModeGreenfield, ModeExisting) {
		errs = append(errs, fmt.Sprintf("project.mode %q is invalid", cfg.Project.Mode))
	}

	if !oneOf(cfg.Governance.StorageMode, StorageModeLocal, StorageModeTracked, StorageModeHybrid) {
		errs = append(errs, fmt.Sprintf("governance.storage_mode %q is invalid", cfg.Governance.StorageMode))
	}

	if !oneOf(cfg.Orchestration.DefaultWorkflow, WorkflowSDD, WorkflowGuided, WorkflowReviewOnly, WorkflowNone) {
		errs = append(errs, fmt.Sprintf("orchestration.default_workflow %q is invalid", cfg.Orchestration.DefaultWorkflow))
	}

	if cfg.Memory.Enabled {
		if strings.TrimSpace(cfg.Memory.Provider) == "" {
			errs = append(errs, "memory.provider is required when memory is enabled")
		} else if cfg.Memory.Provider != MemoryProviderSQLite {
			errs = append(errs, fmt.Sprintf("memory.provider %q is invalid; only %q is supported", cfg.Memory.Provider, MemoryProviderSQLite))
		}
	}

	if cfg.SourceControl.Enabled && strings.TrimSpace(cfg.SourceControl.Provider) == "" {
		errs = append(errs, "source_control.provider is required when source_control is enabled")
	}

	if cfg.SpecEngine.Enabled && strings.TrimSpace(cfg.SpecEngine.Provider) == "" {
		errs = append(errs, "spec_engine.provider is required when spec_engine is enabled")
	}

	if cfg.Registry.Enabled && strings.TrimSpace(cfg.Registry.DefaultSource) == "" {
		errs = append(errs, "registry.default_source is required when registry is enabled")
	}

	if !oneOf(cfg.Materialization.OverwritePolicy, OverwriteAsk, OverwriteSkip, OverwriteOverwrite, OverwriteBackupThenOverwrite) {
		errs = append(errs, fmt.Sprintf("materialization.overwrite_policy %q is invalid", cfg.Materialization.OverwritePolicy))
	}

	if !oneOf(cfg.Materialization.BackupPolicy, BackupNone, BackupBeforeOverwrite) {
		errs = append(errs, fmt.Sprintf("materialization.backup_policy %q is invalid", cfg.Materialization.BackupPolicy))
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid config: %s", strings.Join(errs, "; "))
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
