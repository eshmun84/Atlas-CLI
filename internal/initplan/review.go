package initplan

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// Backup timestamp is never generated during review preview.
const BackupTimestampPlaceholder = "<timestamp>"

// ReviewInput is the in-memory state used to build a preview plan.
type ReviewInput struct {
	Root            string
	Draft           config.ConfigDraft
	MCP             config.MCPDraft
	Artifacts       []string
	AcceptHomeReset bool
}

// MaterializationPlan is a typed preview of init materialization.
// It never writes files.
type MaterializationPlan struct {
	ProjectName        string
	ProjectMode        string
	ProjectModeLabel   string
	ProjectRoot        string
	ProjectID          string
	Workflow           string
	SpecEngine         string
	TestingRequired    string
	ReviewRequired     string
	EvidenceRequired   string
	Adapters           string
	DeliveryPlatform   string
	DeliveryAssistance string
	GovernanceStorage  string
	MCPCount           int
	MCPEntries         []MCPPlanEntry
	Creates            []PlannedFile
	HomeWrites         []PlannedFile
	HomeReset          []PlannedFile
	HomeDataDetected   bool   // true only when presence is confirmed
	HomeDataPresence   string // present | absent | unknown
	HomeDataError      string
	AcceptHomeReset    bool
	ExistingArtifacts  []string
	Backups            []PlannedBackup
	Replacements       []PlannedReplacement
	Preservations      []PlannedPreservation
	GovernanceNote     string
	DeliveryPolicy     []string
	Warnings           []PlanWarning
	Blockers           []PlanBlocker
	PreviewOnly        bool
	ConfigApplyOnly    bool
	GitSafetyStatement string
}

// PlannedFile is one file or directory Atlas would create or update.
type PlannedFile struct {
	Path   string
	Kind   string
	Status string
}

// PlannedBackup is a placeholder backup path under projects/<id>/backups/<timestamp>/.
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
	platform := fieldValue(draft, "source_control.mode")

	plan := MaterializationPlan{
		ProjectName:        draft.ProjectName(),
		ProjectMode:        draft.ProjectMode(),
		ProjectModeLabel:   draft.ProjectModeLabel(),
		ProjectRoot:        strings.TrimSpace(in.Root),
		Workflow:           fieldLabel(draft, "governance.default_workflow"),
		SpecEngine:         fieldLabel(draft, "governance.spec_engine"),
		TestingRequired:    yesNoLabel(fieldValue(draft, "governance.testing_required")),
		ReviewRequired:     yesNoLabel(fieldValue(draft, "governance.review_required")),
		EvidenceRequired:   yesNoLabel(fieldValue(draft, "governance.evidence_required")),
		Adapters:           adaptersDisplay(adaptersValue),
		DeliveryPlatform:   fieldLabel(draft, "source_control.mode"),
		DeliveryAssistance: deliveryAssistLabel(fieldValue(draft, "source_control.delivery_assist")),
		GovernanceStorage:  fieldLabel(draft, "source_control.governance_storage"),
		MCPCount:           in.MCP.ConfiguredCount(),
		MCPEntries:         mcpEntries(in.MCP),
		ExistingArtifacts:  append([]string(nil), in.Artifacts...),
		AcceptHomeReset:    in.AcceptHomeReset,
		DeliveryPolicy: []string{
			"Repository creation requires explicit request.",
			"Branch creation requires explicit request.",
			"Commit requires explicit request.",
			"Push requires explicit request.",
			"Pull request requires explicit request.",
			"Merge requires explicit request.",
		},
		PreviewOnly:        false,
		ConfigApplyOnly:    false,
		GitSafetyStatement: "No repository, branch, commit, push, pull request, merge or remote operation will be performed.",
	}

	plan.HomeDataPresence = "absent"
	if plan.ProjectRoot != "" {
		id, idErr := home.ProjectID(plan.ProjectRoot, plan.ProjectName)
		if idErr != nil {
			plan.HomeDataPresence = "unknown"
			plan.HomeDataError = idErr.Error()
		} else {
			plan.ProjectID = id
			homePath, homeErr := home.Resolve()
			if homeErr != nil {
				plan.HomeDataPresence = "unknown"
				plan.HomeDataError = homeErr.Error()
			} else {
				present, inspErr := home.InspectProjectDataPresence(homePath, id)
				if inspErr != nil {
					plan.HomeDataPresence = "unknown"
					plan.HomeDataError = inspErr.Error()
				} else if present {
					plan.HomeDataPresence = "present"
					plan.HomeDataDetected = true
				} else {
					plan.HomeDataPresence = "absent"
				}
			}
		}
	}

	const status = "create/update on Apply"
	plan.Creates = []PlannedFile{
		{Path: config.FileConfig, Kind: "atlas", Status: status},
		{Path: config.FileLocal, Kind: "atlas", Status: status},
		{Path: config.FileState, Kind: "atlas", Status: status},
		{Path: config.FileAssetsLock, Kind: "atlas", Status: status},
		{Path: config.FileAgentRegistry, Kind: "atlas", Status: status},
		{Path: config.FileRuntimeManifest, Kind: "atlas", Status: status},
		{Path: config.FileSDDOpenSpecContract, Kind: "atlas", Status: status},
		{Path: config.FileAgentsMD, Kind: "runtime", Status: status},
	}
	plan.HomeWrites = []PlannedFile{
		{Path: "Atlas Home (ATLAS_HOME or ~/.atlas)", Kind: "home", Status: "ensure + mirror bundled assets on Apply"},
	}
	if plan.ProjectID != "" {
		plan.HomeWrites = append(plan.HomeWrites, PlannedFile{
			Path:   "projects/" + plan.ProjectID + "/",
			Kind:   "home-project",
			Status: "ensure project-scoped local layout on Apply",
		})
	}
	if plan.HomeDataDetected {
		plan.HomeReset = []PlannedFile{{
			Path:   "projects/" + plan.ProjectID + "/",
			Kind:   "home-reset",
			Status: "delete local Atlas Home project data before init",
		}}
		if !plan.AcceptHomeReset {
			plan.Blockers = append(plan.Blockers, PlanBlocker{
				Message: "Home project data detected for this project. Accept reset of local Atlas data before Apply.",
			})
		}
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

	replaceTargets, unsafeRuntime := plannedReplaceTargets(in.Root, adaptersValue, in.Artifacts)
	for _, msg := range unsafeRuntime {
		plan.Blockers = append(plan.Blockers, PlanBlocker{Message: msg})
		plan.Warnings = append(plan.Warnings, PlanWarning{Message: msg})
	}
	backupRoot := BackupPlaceholderDir()
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
		{Statement: plan.GitSafetyStatement},
		{Statement: "Secrets and credentials are not stored."},
		{Statement: "Developer-owned non-Atlas agents under .cursor/agents/ and .opencode/agents/ are left untouched."},
		{Statement: "Skills are registry-first and are not copied into .cursor/skills or .opencode/skills."},
		{Statement: "Claude Code and Codex adapters are not materialized."},
		{Statement: "Atlas Home reset affects only projects/<project-id>/ for this canonical project."},
		{Statement: "No Git operations. Remotes, branches, and repo files outside Atlas Apply targets stay untouched."},
		{Statement: "Developer-owned project docs under docs/atlas/ are never overwritten by Runtime Repair."},
	}

	if platform == config.SourceControlGitGitHub && storage == "versioned" {
		plan.GovernanceNote = "Atlas governance files may be versioned according to explicit Atlas policy (GitHub selected). No Git operations run during Init."
	} else {
		plan.GovernanceNote = "Atlas governance files stay Local only. Versioning requires a supported delivery platform (GitHub)."
	}

	if boolFieldTrue(draft, "project.docs_scaffold") {
		plan.Creates = append(plan.Creates, PlannedFile{
			Path:   config.FileProjectDocsREADME,
			Kind:   "project-docs",
			Status: "create once if missing (developer-owned; never Runtime Repair)",
		})
	}

	plan.Warnings = []PlanWarning{
		{Message: "Apply is the only mutation step. Status and Doctor remain read-only."},
		{Message: "Apply writes Atlas configuration under .atlas/ and materializes compact runtime gateway files."},
		{Message: "Apply creates/updates Atlas Home (ATLAS_HOME or ~/.atlas) and mirrors bundled Atlas-owned assets."},
		{Message: "After Init, Configure Apply saves .atlas/config.yaml and reconciles MCP projections when MCP/adapters change; Runtime Repair rematerializes non-MCP runtime files; Context Economy Update refreshes context."},
		{Message: "Runtime conflicts block Init and require manual cleanup in this slice."},
		{Message: "MCP desired state is stored in Atlas config and materialized into selected agent MCP configs (Atlas-owned entries only)."},
		{Message: "MCP authentication, connection, and verification may still depend on the provider or agent. Secrets are not stored."},
		{Message: "Context Economy v0 is a separate explicit flow. CodeGraph is an optional externally installed Code Intelligence provider (not MCP; may be unavailable). Atlas Context Graph is NOT IMPLEMENTED."},
		{Message: "Init performs no Git operations."},
		{Message: plan.GitSafetyStatement},
		{Message: "Secrets and credentials are not stored."},
		{Message: "Optional project docs scaffold is developer-owned; Runtime Repair never overwrites docs/atlas/."},
	}

	if len(in.Artifacts) > 0 {
		plan.Warnings = append(plan.Warnings, PlanWarning{
			Message: "Conflicting runtime surfaces remain listed; Init Setup should not be reached until they are removed manually.",
		})
	}

	return plan
}

func plannedReplaceTargets(root, adaptersValue string, artifacts []string) (targets []string, unsafe []string) {
	artifactSet := make(map[string]bool, len(artifacts))
	for _, artifact := range artifacts {
		artifactSet[filepath.ToSlash(artifact)] = true
	}

	consider := func(rel string) {
		present, err := atlasOwnedRuntimePresent(root, rel)
		if err != nil {
			unsafe = append(unsafe, "unsafe Atlas runtime path (not an ordinary replace target): "+rel+": "+err.Error())
			return
		}
		if present || artifactSet[rel] {
			targets = append(targets, rel)
		}
	}

	consider(config.FileAgentsMD)
	if config.ChipSelected(adaptersValue, "cursor") {
		consider(config.FileCursorAtlasMDC)
		for _, path := range config.AtlasAgentRuntimePaths([]string{"cursor"}) {
			consider(path)
		}
	}
	if config.ChipSelected(adaptersValue, "opencode") {
		consider(config.FileOpenCodeAtlas)
		for _, path := range config.AtlasAgentRuntimePaths([]string{"opencode"}) {
			consider(path)
		}
	}
	return targets, unsafe
}

// atlasOwnedRuntimePresent reports a regular contained Atlas runtime file.
// missing => false,nil; unsafe symlink/non-regular => false,error.
func atlasOwnedRuntimePresent(root, rel string) (bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return false, nil
	}
	return config.AtlasOwnedFileExists(root, rel)
}

func fieldValue(draft config.ConfigDraft, key string) string {
	field, ok := draft.FieldByKey(key)
	if !ok {
		return ""
	}
	return field.Value
}

func boolFieldTrue(draft config.ConfigDraft, key string) bool {
	switch strings.ToLower(strings.TrimSpace(fieldValue(draft, key))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
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
		return "none selected"
	}
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		labels = append(labels, config.OptionLabel(part))
	}
	return strings.Join(labels, ", ")
}

func yesNoLabel(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "true") {
		return "Yes"
	}
	return "No"
}

func deliveryAssistLabel(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "true") {
		return "Enabled"
	}
	return "Disabled"
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
			Status:  config.StatusLabel(item.Status),
		})
	}
	for _, server := range draft.CustomServers {
		out = append(out, MCPPlanEntry{
			Name:      server.Name,
			Kind:      "custom",
			Transport: server.Transport.TransportLabel(),
			Enabled:   server.Enabled,
			Status:    config.StatusLabel(server.Status),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BackupPlaceholderDir is the rendered Home-backed backup root used in review copy.
func BackupPlaceholderDir() string {
	return fmt.Sprintf("projects/<project-id>/backups/%s/", BackupTimestampPlaceholder)
}
