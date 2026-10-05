package config

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/version"
)

const (
	PersistSchemaVersion = 1

	MemoryStrategySQLite  = "sqlite"
	MemoryStrategyCapsule = "context_capsule"
	MemoryStrategyBoth    = "sqlite_plus_context_capsule"

	GovernanceFilesLocalOnly = "local_only"
	GovernanceFilesVersioned = "versioned"

	SourceControlGitLocal  = "git_local"
	SourceControlGitGitHub = "git_github"
)

// ProjectDocument is the persisted project-shared .atlas/config.yaml.
type ProjectDocument struct {
	Project       ProjectPersist       `yaml:"project"`
	Governance    GovernancePersist    `yaml:"governance"`
	Adapters      AdaptersPersist      `yaml:"adapters"`
	SourceControl SourceControlPersist `yaml:"source_control"`
	Memory        MemoryPersist        `yaml:"memory"`
	Context       ContextPersist       `yaml:"context"`
	MCP           MCPPersist           `yaml:"mcp"`
}

// ProjectPersist is the persisted project identity.
type ProjectPersist struct {
	Name string `yaml:"name"`
	Mode string `yaml:"mode"`
}

// GovernancePersist is the persisted governance section.
type GovernancePersist struct {
	Workflow         string `yaml:"workflow"`
	SpecEngine       string `yaml:"spec_engine"`
	TestingRequired  bool   `yaml:"testing_required"`
	ReviewRequired   bool   `yaml:"review_required"`
	EvidenceRequired bool   `yaml:"evidence_required"`
}

// AdaptersPersist is the persisted adapter selection.
type AdaptersPersist struct {
	Selected []string `yaml:"selected"`
}

// SourceControlPersist is the persisted source-control policy.
type SourceControlPersist struct {
	Mode            string `yaml:"mode"`
	DefaultRemote   string `yaml:"default_remote"`
	BranchStrategy  string `yaml:"branch_strategy"`
	GovernanceFiles string `yaml:"governance_files"`
	DeliveryAssist  bool   `yaml:"delivery_assist"`
}

// MemoryPersist is the persisted memory strategy.
type MemoryPersist struct {
	Strategy string `yaml:"strategy"`
}

// ContextPersist is the persisted context preferences.
type ContextPersist struct {
	Graph ContextGraphPersist `yaml:"graph"`
}

// ContextGraphPersist is the Context Graph preference.
// Enabled is a pointer so missing YAML defaults to true.
type ContextGraphPersist struct {
	Enabled *bool `yaml:"enabled"`
}

// ContextGraphEnabled reports whether Context Graph is enabled.
// Missing config defaults to true.
func (d ProjectDocument) ContextGraphEnabled() bool {
	if d.Context.Graph.Enabled == nil {
		return true
	}
	return *d.Context.Graph.Enabled
}

// MCPPersist is persisted MCP configuration without credentials.
type MCPPersist struct {
	Builtins MCPBuiltinsPersist `yaml:"builtins"`
	Custom   []MCPCustomPersist `yaml:"custom"`
}

// MCPBuiltinsPersist is a stable, ordered built-in MCP map.
type MCPBuiltinsPersist struct {
	Jira           MCPBuiltinPersist `yaml:"jira"`
	Context7       MCPBuiltinPersist `yaml:"context7"`
	ChromeDevTools MCPBuiltinPersist `yaml:"chrome_devtools"`
}

// MCPBuiltinPersist is one built-in MCP enablement flag.
type MCPBuiltinPersist struct {
	Enabled bool `yaml:"enabled"`
}

// MCPCustomPersist is one custom MCP entry. Credentials are never stored.
type MCPCustomPersist struct {
	Name                  string `yaml:"name"`
	Transport             string `yaml:"transport"`
	CommandOrURL          string `yaml:"command_or_url"`
	Arguments             string `yaml:"arguments"`
	EnvironmentReferences string `yaml:"environment_references"`
	Enabled               bool   `yaml:"enabled"`
}

// LocalDocument is machine-local .atlas/local.yaml. Secrets are never stored.
type LocalDocument struct {
	SchemaVersion     int                `yaml:"schema_version"`
	CredentialsStored bool               `yaml:"credentials_stored"`
	SourceControl     LocalSourceControl `yaml:"source_control"`
}

// LocalSourceControl holds local-only source-control display settings.
type LocalSourceControl struct {
	DefaultRemote string `yaml:"default_remote"`
}

// StateDocument is generated .atlas/state.yaml.
type StateDocument struct {
	SchemaVersion         int    `yaml:"schema_version"`
	Initialized           bool   `yaml:"initialized"`
	RuntimeMaterialized   bool   `yaml:"runtime_materialized"`
	RuntimeMaterializedAt string `yaml:"runtime_materialized_at,omitempty"`
	AppliedAt             string `yaml:"applied_at"`
	AtlasVersion          string `yaml:"atlas_version"`
	ProjectName           string `yaml:"project_name"`
}

// AssetsLockDocument is .atlas/assets.lock.yaml. No assets are installed in this slice.
type AssetsLockDocument struct {
	SchemaVersion int      `yaml:"schema_version"`
	Assets        []string `yaml:"assets"`
}

// IsProjectDocument reports whether the YAML looks like persisted init config.
func (d ProjectDocument) IsProjectDocument() bool {
	return strings.TrimSpace(d.Governance.Workflow) != "" ||
		strings.TrimSpace(d.Memory.Strategy) != "" ||
		len(d.Adapters.Selected) > 0
}

// BuildProjectDocument maps in-memory drafts onto persisted config.yaml.
func BuildProjectDocument(draft ConfigDraft, mcp MCPDraft) ProjectDocument {
	selected := SplitChips(fieldValue(draft, "adapters.selected"))
	if selected == nil {
		selected = []string{}
	}
	custom := make([]MCPCustomPersist, 0, len(mcp.CustomServers))
	for _, server := range mcp.CustomServers {
		custom = append(custom, MCPCustomPersist{
			Name:                  server.Name,
			Transport:             string(NormalizeTransport(string(server.Transport))),
			CommandOrURL:          server.CommandOrURL,
			Arguments:             server.Arguments,
			EnvironmentReferences: server.EnvironmentReferences,
			Enabled:               server.Enabled,
		})
	}
	if custom == nil {
		custom = []MCPCustomPersist{}
	}

	remote := fieldValue(draft, "source_control.default_remote")
	if remote == "" {
		remote = "origin"
	}

	return ProjectDocument{
		Project: ProjectPersist{
			Name: draft.ProjectName(),
			Mode: PersistProjectMode(draft.ProjectMode()),
		},
		Governance: GovernancePersist{
			Workflow:         fieldValueOr(draft, "governance.default_workflow", WorkflowSDD),
			SpecEngine:       fieldValueOr(draft, "governance.spec_engine", SpecProviderOpenSpec),
			TestingRequired:  boolField(draft, "governance.testing_required", true),
			ReviewRequired:   boolField(draft, "governance.review_required", true),
			EvidenceRequired: boolField(draft, "governance.evidence_required", true),
		},
		Adapters: AdaptersPersist{Selected: selected},
		SourceControl: SourceControlPersist{
			Mode:            fieldValueOr(draft, "source_control.mode", SourceControlNone),
			DefaultRemote:   remote,
			BranchStrategy:  fieldValueOr(draft, "source_control.branch_strategy", "manual"),
			GovernanceFiles: fieldValueOr(draft, "source_control.governance_storage", GovernanceFilesLocalOnly),
			DeliveryAssist:  boolField(draft, "source_control.delivery_assist", false),
		},
		Memory: MemoryPersist{
			Strategy: fieldValueOr(draft, "memory.strategy", MemoryStrategyBoth),
		},
		Context: ContextPersist{
			Graph: ContextGraphPersist{
				Enabled: boolPtr(boolField(draft, "context.graph.enabled", true)),
			},
		},
		MCP: MCPPersist{
			Builtins: MCPBuiltinsPersist{
				Jira:           MCPBuiltinPersist{Enabled: builtinEnabled(mcp, MCPBuiltinJira)},
				Context7:       MCPBuiltinPersist{Enabled: builtinEnabled(mcp, MCPBuiltinContext7)},
				ChromeDevTools: MCPBuiltinPersist{Enabled: builtinEnabled(mcp, MCPBuiltinChromeDevTools)},
			},
			Custom: custom,
		},
	}
}

// BuildLocalDocument maps drafts onto persisted local.yaml.
func BuildLocalDocument(draft ConfigDraft) LocalDocument {
	remote := fieldValue(draft, "source_control.default_remote")
	if remote == "" {
		remote = "origin"
	}
	return LocalDocument{
		SchemaVersion:     PersistSchemaVersion,
		CredentialsStored: false,
		SourceControl: LocalSourceControl{
			DefaultRemote: remote,
		},
	}
}

// BuildStateDocument maps drafts onto persisted state.yaml.
func BuildStateDocument(draft ConfigDraft, appliedAt string, runtimeMaterialized bool) StateDocument {
	ver := version.Version
	if ver == "" {
		ver = "0.1.0"
	}
	state := StateDocument{
		SchemaVersion:       PersistSchemaVersion,
		Initialized:         true,
		RuntimeMaterialized: runtimeMaterialized,
		AppliedAt:           appliedAt,
		AtlasVersion:        ver,
		ProjectName:         draft.ProjectName(),
	}
	if runtimeMaterialized {
		state.RuntimeMaterializedAt = appliedAt
	}
	return state
}

func boolPtr(v bool) *bool {
	return &v
}

// BuildAssetsLockDocument returns an empty installed-assets lock.
func BuildAssetsLockDocument() AssetsLockDocument {
	return AssetsLockDocument{
		SchemaVersion: PersistSchemaVersion,
		Assets:        []string{},
	}
}

// ValidateProjectDocument checks persisted config.yaml before write/load.
func ValidateProjectDocument(doc ProjectDocument) error {
	var errs []string
	if strings.TrimSpace(doc.Project.Name) == "" {
		errs = append(errs, "project.name is required")
	}
	mode := PersistProjectMode(doc.Project.Mode)
	if mode != "new" && mode != ModeExisting {
		errs = append(errs, fmt.Sprintf("project.mode %q is invalid", doc.Project.Mode))
	}
	if !oneOf(doc.Governance.Workflow, WorkflowSDD) {
		errs = append(errs, fmt.Sprintf("governance.workflow %q is invalid", doc.Governance.Workflow))
	}
	if !oneOf(doc.Governance.SpecEngine, SpecProviderOpenSpec, SpecProviderNone) {
		errs = append(errs, fmt.Sprintf("governance.spec_engine %q is invalid", doc.Governance.SpecEngine))
	}
	if !oneOf(doc.SourceControl.Mode, SourceControlNone, SourceControlGitLocal, SourceControlGitGitHub) {
		errs = append(errs, fmt.Sprintf("source_control.mode %q is invalid", doc.SourceControl.Mode))
	}
	if !oneOf(doc.SourceControl.BranchStrategy, "manual", "simple", "main_develop", "main_develop_staging") {
		errs = append(errs, fmt.Sprintf("source_control.branch_strategy %q is invalid", doc.SourceControl.BranchStrategy))
	}
	if !oneOf(doc.SourceControl.GovernanceFiles, GovernanceFilesLocalOnly, GovernanceFilesVersioned) {
		errs = append(errs, fmt.Sprintf("source_control.governance_files %q is invalid", doc.SourceControl.GovernanceFiles))
	}
	if !oneOf(doc.Memory.Strategy, MemoryStrategySQLite, MemoryStrategyCapsule, MemoryStrategyBoth) {
		errs = append(errs, fmt.Sprintf("memory.strategy %q is invalid", doc.Memory.Strategy))
	}
	for i, server := range doc.MCP.Custom {
		if strings.TrimSpace(server.Name) == "" {
			errs = append(errs, fmt.Sprintf("mcp.custom[%d].name is required", i))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid config: %s", strings.Join(errs, "; "))
}

// ToMCPDraft maps persisted MCP settings onto an in-memory MCPDraft.
func (d ProjectDocument) ToMCPDraft() MCPDraft {
	draft := DefaultMCPDraft()
	for i := range draft.Builtins {
		enabled := false
		switch draft.Builtins[i].ID {
		case MCPBuiltinJira:
			enabled = d.MCP.Builtins.Jira.Enabled
		case MCPBuiltinContext7:
			enabled = d.MCP.Builtins.Context7.Enabled
		case MCPBuiltinChromeDevTools:
			enabled = d.MCP.Builtins.ChromeDevTools.Enabled
		}
		draft.Builtins[i].Enabled = enabled
		if enabled {
			draft.Builtins[i].Status = MCPStatusInMemoryOnly
		} else {
			draft.Builtins[i].Status = MCPStatusNotConfigured
		}
	}
	draft.CustomServers = make([]MCPServerDraft, 0, len(d.MCP.Custom))
	for i, server := range d.MCP.Custom {
		draft.CustomServers = append(draft.CustomServers, MCPServerDraft{
			ID:                    fmt.Sprintf("custom-%d", i+1),
			Name:                  server.Name,
			Transport:             NormalizeTransport(server.Transport),
			CommandOrURL:          server.CommandOrURL,
			Arguments:             server.Arguments,
			EnvironmentReferences: server.EnvironmentReferences,
			Enabled:               server.Enabled,
			Status:                MCPStatusInMemoryOnly,
		})
	}
	return draft
}

// ApplyProjectDocument overlays persisted field values onto an existing ConfigDraft.
func ApplyProjectDocument(draft *ConfigDraft, doc ProjectDocument) {
	if draft == nil {
		return
	}
	setDraftValue(draft, "project.name", doc.Project.Name)
	setDraftValue(draft, "project.mode", NormalizeProjectMode(doc.Project.Mode))
	setDraftValue(draft, "governance.default_workflow", doc.Governance.Workflow)
	setDraftValue(draft, "governance.spec_engine", doc.Governance.SpecEngine)
	setDraftValue(draft, "governance.testing_required", boolText(doc.Governance.TestingRequired))
	setDraftValue(draft, "governance.review_required", boolText(doc.Governance.ReviewRequired))
	setDraftValue(draft, "governance.evidence_required", boolText(doc.Governance.EvidenceRequired))
	selected := doc.Adapters.Selected
	if selected == nil {
		selected = []string{}
	}
	setDraftValue(draft, "adapters.selected", JoinChips(selected))
	setDraftValue(draft, "source_control.mode", doc.SourceControl.Mode)
	setDraftValue(draft, "source_control.default_remote", doc.SourceControl.DefaultRemote)
	setDraftValue(draft, "source_control.branch_strategy", doc.SourceControl.BranchStrategy)
	setDraftValue(draft, "source_control.governance_storage", doc.SourceControl.GovernanceFiles)
	setDraftValue(draft, "source_control.delivery_assist", boolText(doc.SourceControl.DeliveryAssist))
	setDraftValue(draft, "memory.strategy", doc.Memory.Strategy)
	setDraftValue(draft, "context.graph.enabled", boolText(doc.ContextGraphEnabled()))
}

func setDraftValue(draft *ConfigDraft, key, value string) {
	for si := range draft.Sections {
		for fi := range draft.Sections[si].Fields {
			if draft.Sections[si].Fields[fi].Key == key {
				draft.Sections[si].Fields[fi].Value = value
				return
			}
		}
	}
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// ToConfig maps a persisted project document onto the internal Config model.
func (d ProjectDocument) ToConfig() Config {
	cfg := DefaultConfig(d.Project.Name)
	cfg.Project.Mode = NormalizeProjectMode(d.Project.Mode)
	cfg.Governance.TestingRequired = d.Governance.TestingRequired
	cfg.Governance.ReviewRequired = d.Governance.ReviewRequired
	cfg.Governance.EvidenceRequired = d.Governance.EvidenceRequired
	switch d.SourceControl.GovernanceFiles {
	case GovernanceFilesVersioned:
		cfg.Governance.StorageMode = StorageModeTracked
	default:
		cfg.Governance.StorageMode = StorageModeLocal
	}
	cfg.Orchestration.DefaultWorkflow = d.Governance.Workflow
	cfg.SpecEngine.Provider = d.Governance.SpecEngine
	cfg.SpecEngine.Enabled = d.Governance.SpecEngine != SpecProviderNone && d.Governance.SpecEngine != ""
	cfg.Adapters.Cursor = containsString(d.Adapters.Selected, "cursor")
	cfg.Adapters.OpenCode = containsString(d.Adapters.Selected, "opencode")
	cfg.SourceControl.Mode = d.SourceControl.Mode
	switch d.SourceControl.Mode {
	case SourceControlGitLocal:
		cfg.SourceControl.Enabled = true
		cfg.SourceControl.Provider = SourceControlGit
		cfg.SourceControl.GitEnabled = true
	case SourceControlGitGitHub:
		cfg.SourceControl.Enabled = true
		cfg.SourceControl.Provider = SourceControlGitHub
		cfg.SourceControl.GitEnabled = true
		cfg.SourceControl.GitHubEnabled = true
	default:
		cfg.SourceControl.Enabled = false
		cfg.SourceControl.Provider = SourceControlNone
	}
	cfg.SourceControl.DeliveryEnabled = d.SourceControl.DeliveryAssist
	cfg.Memory.Enabled = true
	cfg.Memory.Provider = MemoryProviderSQLite
	switch d.Memory.Strategy {
	case MemoryStrategySQLite:
		cfg.Memory.CapsuleEnabled = false
	case MemoryStrategyCapsule:
		cfg.Memory.CapsuleEnabled = true
	default:
		cfg.Memory.CapsuleEnabled = true
	}
	cfg.ExternalContextProviders.JiraEnabled = d.MCP.Builtins.Jira.Enabled
	cfg.ExternalContextProviders.MCPEnabled = d.MCP.Builtins.Jira.Enabled ||
		d.MCP.Builtins.Context7.Enabled ||
		d.MCP.Builtins.ChromeDevTools.Enabled ||
		len(d.MCP.Custom) > 0
	return cfg
}

func fieldValue(draft ConfigDraft, key string) string {
	field, ok := draft.FieldByKey(key)
	if !ok {
		return ""
	}
	return field.Value
}

func fieldValueOr(draft ConfigDraft, key, fallback string) string {
	value := strings.TrimSpace(fieldValue(draft, key))
	if value == "" {
		return fallback
	}
	return value
}

func boolField(draft ConfigDraft, key string, fallback bool) bool {
	value := strings.TrimSpace(fieldValue(draft, key))
	if value == "" {
		return fallback
	}
	return strings.EqualFold(value, "true")
}

func builtinEnabled(mcp MCPDraft, id MCPBuiltinID) bool {
	for _, item := range mcp.Builtins {
		if item.ID == id {
			return item.Enabled
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
