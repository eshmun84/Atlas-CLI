package initplan

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// Backup timestamp is never generated in this slice.
const BackupTimestampPlaceholder = "<timestamp>"

// ReviewInput is the in-memory state used to build a preview plan.
type ReviewInput struct {
	Draft     config.ConfigDraft
	MCP       config.MCPDraft
	Artifacts []string
}

// MaterializationPlan is a typed preview of future init materialization.
// It never writes files.
type MaterializationPlan struct {
	ProjectName       string
	ProjectMode       string
	ProjectModeLabel  string
	Workflow          string
	SpecEngine        string
	Adapters          string
	SourceControl     string
	BranchStrategy    string
	GovernanceStorage string
	MemoryStrategy    string
	MCPCount          int
	MCPEntries        []MCPPlanEntry
	Creates           []PlannedFile
	ExistingArtifacts []string
	Backups           []PlannedBackup
	Replacements      []PlannedReplacement
	Preservations     []PlannedPreservation
	GovernanceNote    string
	Warnings          []PlanWarning
	Blockers          []PlanBlocker
	PreviewOnly       bool
	ConfigApplyOnly   bool
}

// PlannedFile is one file or directory Atlas would create later.
type PlannedFile struct {
	Path   string
	Kind   string
	Status string
}

// PlannedBackup is a placeholder backup path under .atlas/backups/<timestamp>/.
type PlannedBackup struct {
	Path   string
	Source string
}

// PlannedReplacement is a runtime artifact Atlas would replace after backup.
type PlannedReplacement struct {
	Path string
}

// PlannedPreservation explains what Atlas will leave untouched.
type PlannedPreservation struct {
	Statement string
}

// PlanWarning is a non-blocking review note.
type PlanWarning struct {
	Message string
}

// PlanBlocker is a condition that prevents applying init.
type PlanBlocker struct {
	Message string
}

// MCPPlanEntry summarizes one in-memory MCP draft server.
type MCPPlanEntry struct {
	Name      string
	Kind      string
	Transport string
	Enabled   bool
	Status    string
}

// BuildReview constructs a materialization plan.
// Atlas config files can be applied in this slice; runtime files remain planned for later.
func BuildReview(in ReviewInput) MaterializationPlan {
	draft := in.Draft
	adaptersValue := fieldValue(draft, "adapters.selected")
	storage := fieldValue(draft, "source_control.governance_storage")

	plan := MaterializationPlan{
		ProjectName:       draft.ProjectName(),
		ProjectMode:       draft.ProjectMode(),
		ProjectModeLabel:  draft.ProjectModeLabel(),
		Workflow:          fieldLabel(draft, "governance.default_workflow"),
		SpecEngine:        fieldLabel(draft, "governance.spec_engine"),
		Adapters:          adaptersDisplay(adaptersValue),
		SourceControl:     fieldLabel(draft, "source_control.mode"),
		BranchStrategy:    fieldLabel(draft, "source_control.branch_strategy"),
		GovernanceStorage: fieldLabel(draft, "source_control.governance_storage"),
		MemoryStrategy:    fieldLabel(draft, "memory.strategy"),
		MCPCount:          in.MCP.ConfiguredCount(),
		MCPEntries:        mcpEntries(in.MCP),
		ExistingArtifacts: append([]string(nil), in.Artifacts...),
		PreviewOnly:       false,
		ConfigApplyOnly:   true,
	}

	plan.Creates = []PlannedFile{
		{Path: config.FileConfig, Kind: "atlas", Status: "create this slice"},
		{Path: config.FileLocal, Kind: "atlas", Status: "create this slice"},
		{Path: config.FileState, Kind: "atlas", Status: "create this slice"},
		{Path: config.FileAssetsLock, Kind: "atlas", Status: "create this slice"},
		{Path: config.DirBackups + "/", Kind: "atlas", Status: "create this slice"},
		{Path: "AGENTS.md", Kind: "runtime", Status: "planned for later"},
	}
	if config.ChipSelected(adaptersValue, "cursor") {
		plan.Creates = append(plan.Creates, PlannedFile{
			Path:   ".cursor/rules/atlas.mdc",
			Kind:   "adapter",
			Status: "planned for later",
		})
	}
	if config.ChipSelected(adaptersValue, "opencode") {
		plan.Creates = append(plan.Creates, PlannedFile{
			Path:   "OpenCode runtime adapter files",
			Kind:   "adapter",
			Status: "planned for later",
		})
	}

	backupRoot := config.DirBackups + "/" + BackupTimestampPlaceholder + "/"
	if len(in.Artifacts) == 0 {
		plan.Backups = nil
		plan.Replacements = nil
	} else {
		plan.Backups = make([]PlannedBackup, 0, len(in.Artifacts)+1)
		plan.Replacements = make([]PlannedReplacement, 0, len(in.Artifacts))
		for _, artifact := range in.Artifacts {
			plan.Backups = append(plan.Backups, PlannedBackup{
				Path:   backupRoot + artifact,
				Source: artifact,
			})
			plan.Replacements = append(plan.Replacements, PlannedReplacement{Path: artifact})
		}
		plan.Backups = append(plan.Backups, PlannedBackup{
			Path:   backupRoot + "manifest.json",
			Source: "manifest",
		})
	}

	plan.Preservations = []PlannedPreservation{
		{Statement: "Existing project source files are preserved."},
		{Statement: "README.md is preserved unless future explicit README integration is enabled."},
		{Statement: "Git history is not modified."},
		{Statement: "No commits are created."},
		{Statement: "No branches are created."},
		{Statement: "No remote operations are performed."},
		{Statement: "Secrets and credentials are not stored."},
	}

	if storage == "versioned" {
		plan.GovernanceNote = "Atlas governance files may be versioned according to explicit Atlas policy."
	} else {
		plan.GovernanceNote = "Atlas governance files will stay local where appropriate and will be ignored by Git during future materialization."
	}

	plan.Warnings = []PlanWarning{
		{Message: "Apply writes Atlas configuration under .atlas/ only."},
		{Message: "Runtime files such as AGENTS.md are not created in this slice."},
		{Message: "Existing runtime artifacts are not backed up or replaced in this slice."},
		{Message: "No Git operations are performed."},
		{Message: "Secrets and credentials are not stored."},
	}

	return plan
}

func fieldValue(draft config.ConfigDraft, key string) string {
	field, ok := draft.FieldByKey(key)
	if !ok {
		return ""
	}
	return field.Value
}

func fieldLabel(draft config.ConfigDraft, key string) string {
	value := fieldValue(draft, key)
	if value == "" {
		return "—"
	}
	return config.OptionLabel(value)
}

func adaptersDisplay(value string) string {
	parts := config.SplitChips(value)
	if len(parts) == 0 {
		return "none"
	}
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		labels = append(labels, config.OptionLabel(part))
	}
	return strings.Join(labels, ", ")
}

func mcpEntries(draft config.MCPDraft) []MCPPlanEntry {
	out := make([]MCPPlanEntry, 0, draft.ConfiguredCount())
	for _, item := range draft.Builtins {
		if !item.Enabled {
			continue
		}
		out = append(out, MCPPlanEntry{
			Name:    item.Name,
			Kind:    "built-in",
			Enabled: true,
			Status:  "will persist in .atlas/config.yaml",
		})
	}
	for _, server := range draft.CustomServers {
		out = append(out, MCPPlanEntry{
			Name:      server.Name,
			Kind:      "custom",
			Transport: server.Transport.TransportLabel(),
			Enabled:   server.Enabled,
			Status:    "will persist in .atlas/config.yaml",
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BackupPlaceholderDir is the rendered backup root used in review copy.
func BackupPlaceholderDir() string {
	return fmt.Sprintf("%s/%s/", config.DirBackups, BackupTimestampPlaceholder)
}
