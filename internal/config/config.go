package config

// Config is the root Atlas configuration model.
type Config struct {
	Atlas                    AtlasConfig                    `yaml:"atlas"`
	Project                  ProjectConfig                  `yaml:"project"`
	Governance               GovernanceConfig               `yaml:"governance"`
	Orchestration            OrchestrationConfig            `yaml:"orchestration"`
	SpecEngine               SpecEngineConfig               `yaml:"spec_engine"`
	Adapters                 AdaptersConfig                 `yaml:"adapters"`
	SourceControl            SourceControlConfig            `yaml:"source_control"`
	Memory                   MemoryConfig                   `yaml:"memory"`
	Assets                   AssetsConfig                   `yaml:"assets"`
	Registry                 RegistryConfig                 `yaml:"registry"`
	ExternalContextProviders ExternalContextProvidersConfig `yaml:"external_context_providers"`
	Materialization          MaterializationConfig          `yaml:"materialization"`
}

// AtlasConfig holds Atlas runtime identity metadata.
type AtlasConfig struct {
	Version string `yaml:"version"`
}

// ProjectConfig describes the target software project.
type ProjectConfig struct {
	Name            string   `yaml:"name"`
	Description     string   `yaml:"description"`
	Mode            string   `yaml:"mode"`
	Type            string   `yaml:"type"`
	PrimaryLanguage string   `yaml:"primary_language"`
	Technologies    []string `yaml:"technologies"`
}

// GovernanceConfig controls Atlas governance behavior.
type GovernanceConfig struct {
	Enabled              bool   `yaml:"enabled"`
	StorageMode          string `yaml:"storage_mode"`
	RequestRouterEnabled bool   `yaml:"request_router_enabled"`
	TestingRequired      bool   `yaml:"testing_required"`
	ReviewRequired       bool   `yaml:"review_required"`
	EvidenceRequired     bool   `yaml:"evidence_required"`
}

// OrchestrationConfig controls workflow selection and planning.
type OrchestrationConfig struct {
	DefaultWorkflow      string   `yaml:"default_workflow"`
	SupportedWorkflows   []string `yaml:"supported_workflows"`
	ProportionalPlanning bool     `yaml:"proportional_planning"`
	SDDRequiredByDefault bool     `yaml:"sdd_required_by_default"`
}

// SpecEngineConfig controls optional specification-engine integration.
type SpecEngineConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Provider      string `yaml:"provider"`
	Required      bool   `yaml:"required"`
	StorageMode   string `yaml:"storage_mode"`
	InstallPolicy string `yaml:"install_policy"`
}

// AdaptersConfig toggles IDE/agent adapters.
type AdaptersConfig struct {
	Cursor   bool `yaml:"cursor"`
	OpenCode bool `yaml:"opencode"`
	Codex    bool `yaml:"codex"`
}

// SourceControlConfig controls Git/GitHub automation settings.
type SourceControlConfig struct {
	Enabled          bool   `yaml:"enabled"`
	Provider         string `yaml:"provider"`
	Mode             string `yaml:"mode"`
	GitEnabled       bool   `yaml:"git_enabled"`
	GitHubEnabled    bool   `yaml:"github_enabled"`
	BranchingEnabled bool   `yaml:"branching_enabled"`
	SigningRequired  bool   `yaml:"signing_required"`
	DeliveryEnabled  bool   `yaml:"delivery_enabled"`
}

// MemoryConfig controls local Atlas memory.
type MemoryConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Provider       string `yaml:"provider"`
	SQLitePath     string `yaml:"sqlite_path"`
	CapsuleEnabled bool   `yaml:"capsule_enabled"`
	CapsulePath    string `yaml:"capsule_path"`
	GraphEnabled   bool   `yaml:"graph_enabled"`
}

// AssetsConfig controls how Atlas assets are sourced and installed.
type AssetsConfig struct {
	Source             string   `yaml:"source"`
	CoreAssetsEmbedded bool     `yaml:"core_assets_embedded"`
	RegistryEnabled    bool     `yaml:"registry_enabled"`
	Installed          []string `yaml:"installed"`
}

// RegistryConfig controls remote/community asset registry access.
type RegistryConfig struct {
	Enabled                bool   `yaml:"enabled"`
	DefaultSource          string `yaml:"default_source"`
	AllowRemoteSources     bool   `yaml:"allow_remote_sources"`
	CommunityAssetsEnabled bool   `yaml:"community_assets_enabled"`
}

// ExternalContextProvidersConfig toggles external context integrations.
type ExternalContextProvidersConfig struct {
	MCPEnabled          bool `yaml:"mcp_enabled"`
	JiraEnabled         bool `yaml:"jira_enabled"`
	GitHubIssuesEnabled bool `yaml:"github_issues_enabled"`
	ConfluenceEnabled   bool `yaml:"confluence_enabled"`
	GoogleDriveEnabled  bool `yaml:"google_drive_enabled"`
}

// MaterializationConfig controls how project files are written.
type MaterializationConfig struct {
	DryRunRequired  bool   `yaml:"dry_run_required"`
	OverwritePolicy string `yaml:"overwrite_policy"`
	BackupPolicy    string `yaml:"backup_policy"`
}
