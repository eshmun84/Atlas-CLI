package initplan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// Backup timestamp is never generated during review preview.
const BackupTimestampPlaceholder = "<timestamp>"

// ReviewInput is the in-memory state used to build a preview plan.
type ReviewInput struct {
	Root      string
	Draft     config.ConfigDraft
	MCP       config.MCPDraft
	Artifacts []string
}

// MaterializationPlan is a typed preview of init materialization.
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
	ContextGraph      string
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

// PlannedFile is one file or directory Atlas would create or update.
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

// BuildReview constructs a materialization plan for Init Apply.
// Atlas config files and allowlisted runtime entrypoints are applied in this slice.
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
		ContextGraph:      contextGraphLabel(draft),
		MCPCount:          in.MCP.ConfiguredCount(),
		MCPEntries:        mcpEntries(in.MCP),
		ExistingArtifacts: append([]string(nil), in.Artifacts...),
		PreviewOnly:       false,
		ConfigApplyOnly:   false,
	}

	const status = "create/update this slice"
	plan.Creates = []PlannedFile{
		{Path: config.FileConfig, Kind: "atlas", Status: status},
		{Path: config.FileLocal, Kind: "atlas", Status: status},
		{Path: config.FileState, Kind: "atlas", Status: status},
		{Path: config.FileAssetsLock, Kind: "atlas", Status: status},
		{Path: config.FileAgentRegistry, Kind: "atlas", Status: status},
		{Path: config.FileRuntimeManifest, Kind: "atlas", Status: status},
		{Path: config.FileSDDOpenSpecContract, Kind: "atlas", Status: status},
		{Path: config.DirBackups + "/", Kind: "atlas", Status: "create if needed"},
		{Path: config.FileAgentsMD, Kind: "runtime", Status: status},
	}
	if config.ChipSelected(adaptersValue, "cursor") {
		plan.Creates = append(plan.Creates, PlannedFile{
			Path:   config.FileCursorAtlasMDC,
			Kind:   "adapter",
			Status: status,
		})
		for _, path := range config.AtlasAgentRuntimePaths([]string{"cursor"}) {
			plan.Creates = append(plan.Creates, PlannedFile{
				Path:   path,
				Kind:   "agent",
				Status: status,
			})
		}
	}
	if config.ChipSelected(adaptersValue, "opencode") {
		plan.Creates = append(plan.Creates, PlannedFile{
			Path:   config.FileOpenCodeAtlas,
			Kind:   "adapter",
			Status: status,
		})
		for _, path := range config.AtlasAgentRuntimePaths([]string{"opencode"}) {
			plan.Creates = append(plan.Creates, PlannedFile{
				Path:   path,
				Kind:   "agent",
				Status: status,
			})
		}
	}

	replaceTargets := plannedReplaceTargets(in.Root, adaptersValue, in.Artifacts)
	backupRoot := config.DirBackups + "/" + BackupTimestampPlaceholder + "/"
	if len(replaceTargets) == 0 {
		plan.Backups = nil
		plan.Replacements = nil
	} else {
		plan.Backups = make([]PlannedBackup, 0, len(replaceTargets)+1)
		plan.Replacements = make([]PlannedReplacement, 0, len(replaceTargets))
		for _, target := range replaceTargets {
			plan.Backups = append(plan.Backups, PlannedBackup{
				Path:   backupRoot + target,
				Source: target,
			})
			plan.Replacements = append(plan.Replacements, PlannedReplacement{Path: target})
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
		{Statement: "Developer-owned non-Atlas agents under .cursor/agents/ and .opencode/agents/ are left untouched."},
		{Statement: "Skills are registry-first and are not copied into .cursor/skills or .opencode/skills."},
	}

	if storage == "versioned" {
		plan.GovernanceNote = "Atlas governance files may be versioned according to explicit Atlas policy."
	} else {
		plan.GovernanceNote = "Atlas governance files will stay local where appropriate and will be ignored by Git during future materialization."
	}

	plan.Warnings = []PlanWarning{
		{Message: "Apply writes Atlas configuration under .atlas/ and materializes compact runtime gateway files."},
		{Message: "Apply creates/updates Atlas Home (ATLAS_HOME or ~/.atlas) and mirrors bundled Atlas-owned assets."},
		{Message: "AGENTS.md is the project authority; Atlas agents are cataloged in .atlas/agent-registry.md with Home source paths."},
		{Message: "Skills remain registry-first; this slice does not vendor skills into adapter skill folders."},
		{Message: "Context Graph is a preference/context aid only; no graph engine, database, embeddings, index, capsules, or packs."},
		{Message: "Cursor/OpenCode entrypoints point at AGENTS.md and atlas-orchestrator; they must not bypass AGENTS.md."},
		{Message: "Existing Atlas-managed runtime targets are backed up under .atlas/backups/<timestamp>/ before replacement."},
		{Message: "CLAUDE.md, GEMINI.md, .agents/, .claude/, README.md, and .gitignore are not materialized."},
		{Message: "No Git operations are performed."},
		{Message: "Secrets and credentials are not stored."},
	}

	return plan
}

func plannedReplaceTargets(root, adaptersValue string, artifacts []string) []string {
	artifactSet := make(map[string]bool, len(artifacts))
	for _, artifact := range artifacts {
		artifactSet[filepath.ToSlash(artifact)] = true
	}

	var targets []string
	if artifactSet[config.FileAgentsMD] || fileExists(root, config.FileAgentsMD) {
		targets = append(targets, config.FileAgentsMD)
	}
	if config.ChipSelected(adaptersValue, "cursor") {
		if fileExists(root, config.FileCursorAtlasMDC) {
			targets = append(targets, config.FileCursorAtlasMDC)
		}
		for _, path := range config.AtlasAgentRuntimePaths([]string{"cursor"}) {
			if fileExists(root, path) {
				targets = append(targets, path)
			}
		}
	}
	if config.ChipSelected(adaptersValue, "opencode") {
		if fileExists(root, config.FileOpenCodeAtlas) {
			targets = append(targets, config.FileOpenCodeAtlas)
		}
		for _, path := range config.AtlasAgentRuntimePaths([]string{"opencode"}) {
			if fileExists(root, path) {
				targets = append(targets, path)
			}
		}
	}
	return targets
}

func fileExists(root, rel string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func contextGraphLabel(draft config.ConfigDraft) string {
	if strings.EqualFold(strings.TrimSpace(fieldValue(draft, "context.graph.enabled")), "false") {
		return "Disabled"
	}
	return "Enabled"
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
