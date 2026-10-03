package config

import "github.com/eshmun84/Atlas-CLI/internal/version"

// DefaultConfig returns the MVP default Atlas configuration for a project.
func DefaultConfig(projectName string) Config {
	return Config{
		Atlas: AtlasConfig{
			Version: version.Version,
		},
		Project: ProjectConfig{
			Name:         projectName,
			Mode:         ModeGreenfield,
			Technologies: []string{},
		},
		Governance: GovernanceConfig{
			Enabled:              true,
			StorageMode:          StorageModeLocal,
			RequestRouterEnabled: true,
			TestingRequired:      true,
			ReviewRequired:       true,
			EvidenceRequired:     true,
		},
		Orchestration: OrchestrationConfig{
			DefaultWorkflow:    WorkflowSDD,
			SupportedWorkflows: []string{WorkflowSDD, WorkflowGuided},
		},
		SpecEngine: SpecEngineConfig{
			Enabled:  false,
			Provider: SpecProviderNone,
		},
		Adapters: AdaptersConfig{
			Cursor:   false,
			OpenCode: false,
			Codex:    false,
		},
		SourceControl: SourceControlConfig{
			Enabled:  false,
			Provider: SourceControlNone,
		},
		Memory: MemoryConfig{
			Enabled:        true,
			Provider:       MemoryProviderSQLite,
			SQLitePath:     FileMemoryDB,
			CapsuleEnabled: true,
			CapsulePath:    FileCapsule,
			GraphEnabled:   false,
		},
		Assets: AssetsConfig{
			Source:             AssetsSourceEmbedded,
			CoreAssetsEmbedded: true,
			RegistryEnabled:    false,
			Installed:          []string{},
		},
		Registry: RegistryConfig{
			Enabled:                false,
			AllowRemoteSources:     false,
			CommunityAssetsEnabled: false,
		},
		ExternalContextProviders: ExternalContextProvidersConfig{
			MCPEnabled:          false,
			JiraEnabled:         false,
			GitHubIssuesEnabled: false,
			ConfluenceEnabled:   false,
			GoogleDriveEnabled:  false,
		},
		Materialization: MaterializationConfig{
			DryRunRequired:  true,
			OverwritePolicy: OverwriteAsk,
			BackupPolicy:    BackupBeforeOverwrite,
		},
	}
}
