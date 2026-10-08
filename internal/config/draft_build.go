package config

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/delivery"
)

// ProjectSetupInput carries Step 1 values used to seed a ConfigDraft.
type ProjectSetupInput struct {
	ProjectName      string
	ProjectMode      string
	ProjectID        string
	DefaultRemote    string
	CursorDetected   bool
	OpenCodeDetected bool
	// DocsScaffold is set on Project Setup (Init Step 1); off by default.
	DocsScaffold bool

	// GitRepoDetected seeds Delivery platform from inspect/project Git evidence
	// (result.Git.IsRepo). It must not run git; callers pass an existing fact.
	// Explicit persisted source_control.mode always wins via ApplyProjectDocument.
	GitRepoDetected bool

	// Tool availability (LookPath only; never executed).
	ToolGitAvailable       bool
	ToolGHAvailable        bool
	ToolGlabAvailable      bool
	ToolBitbucketAvailable bool
	ToolOpenSpecAvailable  bool
	ToolCursorAvailable    bool
	ToolOpenCodeAvailable  bool
}

// DefaultSourceControlMode returns the Init/Configure draft seed for
// source_control.mode before any persisted document is applied.
//
// Ownership of Delivery mode meaning lives in internal/delivery.
// Precedence after seeding is handled by ApplyProjectDocument:
//
//	explicit Atlas config → detected Git repository → none
func DefaultSourceControlMode(gitRepoDetected bool) string {
	return delivery.DefaultMode(gitRepoDetected)
}

// NormalizeProjectMode maps Step 1 modes onto config project modes.
func NormalizeProjectMode(mode string) string {
	switch mode {
	case "new", ModeGreenfield:
		return ModeGreenfield
	case ModeExisting, "":
		return ModeExisting
	default:
		return mode
	}
}

// PersistProjectMode maps a draft/internal mode onto the persisted config.yaml value.
func PersistProjectMode(mode string) string {
	if NormalizeProjectMode(mode) == ModeGreenfield {
		return "new"
	}
	return ModeExisting
}

// BuildConfigDraft constructs a full in-memory configuration draft.
func BuildConfigDraft(mode ConfigMode, setup ProjectSetupInput) ConfigDraft {
	setup.ProjectMode = NormalizeProjectMode(setup.ProjectMode)
	if strings.TrimSpace(setup.ProjectName) == "" {
		setup.ProjectName = "project"
	}
	if setup.ProjectID == "" {
		setup.ProjectID = PreviewProjectID(setup.ProjectName)
	}

	remote := setup.DefaultRemote
	if remote == "" {
		remote = "origin"
	}

	// Availability is PATH tool presence only. Project artifacts like .cursor/ are
	// conflict surfaces handled by Init preflight, not adapter availability.
	// CursorDetected/OpenCodeDetected still seed selection for Apply/Configure inputs.
	cursorAvailable := setup.ToolCursorAvailable
	opencodeAvailable := setup.ToolOpenCodeAvailable
	adapters := ""
	if setup.CursorDetected {
		adapters = ToggleChip(adapters, "cursor")
	}
	if setup.OpenCodeDetected {
		adapters = ToggleChip(adapters, "opencode")
	}
	var adapterDisabled []string
	if !cursorAvailable {
		adapterDisabled = append(adapterDisabled, "cursor")
	}
	if !opencodeAvailable {
		adapterDisabled = append(adapterDisabled, "opencode")
	}

	adaptersField := field("adapters.selected", "", "", adapters, adapters, FieldTypeMulti, []string{"cursor", "opencode"}, FieldEditable, FieldEditable, false, false)
	adaptersField.DisabledValues = adapterDisabled

	deliveryPlatform := DefaultSourceControlMode(setup.GitRepoDetected)

	draft := ConfigDraft{
		Mode: mode,
		Sections: []ConfigSection{
			{
				// Hidden from SelectorSections. Name/mode come from Project Setup;
				// docs_scaffold is chosen on Project Setup (Init) and persisted for Apply.
				Key:         "project",
				Title:       "Project",
				Description: "Project identity from setup.",
				Fields: []ConfigField{
					field("project.name", "Project name", "", setup.ProjectName, setup.ProjectName, FieldTypeReadonly, nil, FieldReadonly, FieldLocked, true, false),
					field("project.mode", "Project mode", "", setup.ProjectMode, setup.ProjectMode, FieldTypeReadonly, nil, FieldReadonly, FieldLocked, true, false),
					field("project.docs_scaffold", "Project docs scaffold", "Optional developer-owned docs. Created once; existing docs are not overwritten.", boolText(setup.DocsScaffold), "false", FieldTypeBool, nil, FieldEditable, FieldEditable, false, false),
				},
			},
			{
				Key:         "governance",
				Title:       "Governance",
				Description: "Governance configuration for AI-assisted work. This records Atlas policy preferences — it does not execute OpenSpec CLI commands.",
				Fields: []ConfigField{
					field("governance.default_workflow", "Workflow", "SDD (Spec-Driven Development) workflow preference.", "sdd", "sdd", FieldTypeChoice, []string{"sdd"}, FieldEditable, FieldEditable, true, false),
					field("governance.spec_engine", "Spec engine", "OpenSpec is the preferred spec engine preference. Atlas does not run OpenSpec commands from this screen.", "openspec", "openspec", FieldTypeChoice, []string{"openspec", "none"}, FieldEditable, FieldEditable, true, false),
					field("governance.testing_required", "Testing required", "Quality gate: testing evidence is required when applicable.", "true", "true", FieldTypeBool, nil, FieldEditable, FieldEditable, true, false),
					field("governance.review_required", "Review required", "Quality gate: review is required when applicable.", "true", "true", FieldTypeBool, nil, FieldEditable, FieldEditable, true, false),
					field("governance.evidence_required", "Evidence required", "Quality gate: the agent must report verification evidence, commands, tests, smoke, results, and limitations when applicable.", "true", "true", FieldTypeBool, nil, FieldEditable, FieldEditable, true, false),
				},
			},
			{
				Key:         "adapters",
				Title:       "Adapters",
				Description: "Selection is saved to config. Runtime files update only via Init Apply or explicit Runtime Repair — not Configure Apply.",
				Fields: []ConfigField{
					adaptersField,
				},
			},
			{
				Key:         "source_control",
				Title:       "Delivery",
				Description: "",
				Fields: []ConfigField{
					field("source_control.mode", "Platform", "", deliveryPlatform, deliveryPlatform, FieldTypeChoice, []string{"none", "git_local", "git_github"}, FieldEditable, FieldEditable, true, false),
					field("source_control.governance_storage", "Governance files", "", "local_only", "local_only", FieldTypeChoice, []string{"local_only", "versioned"}, FieldEditable, FieldEditable, true, false),
					field("source_control.delivery_assist", "Assisted operations", "", "false", "false", FieldTypeBool, nil, FieldEditable, FieldEditable, false, false),
				},
			},
			{
				Key:         "mcp",
				Title:       "MCP",
				Description: "Preference/config only — not materialized, connected, authenticated, or verified.",
				Fields:      nil,
			},
			{
				// Hidden from SelectorSections; retained so persist/load keep remote + branch_strategy + memory.
				// Context Economy is a separate explicit flow. CodeGraph and Atlas Context Graph are not Configure choices.
				Key:         "compat",
				Title:       "Compatibility",
				Description: "Internal compatibility fields — not shown in Init/Configure selectors.",
				Fields: []ConfigField{
					field("source_control.default_remote", "Default remote", "Not an Init setup decision.", remote, remote, FieldTypeReadonly, nil, FieldReadonly, FieldReadonly, false, false),
					field("source_control.branch_strategy", "Branch strategy", "Not an Init setup decision. Atlas does not configure GitFlow.", "manual", "manual", FieldTypeReadonly, nil, FieldReadonly, FieldReadonly, false, false),
					field("memory.strategy", "Memory strategy", "Always-on local Atlas-managed memory; not an Init setup choice.", "sqlite_plus_context_capsule", "sqlite_plus_context_capsule", FieldTypeReadonly, nil, FieldReadonly, FieldReadonly, true, false),
					field("context.graph.enabled", "Atlas Context Graph preference", "Compatibility only. Atlas Context Graph is NOT IMPLEMENTED. Context Economy v0 is the separate file-based flow. CodeGraph is a future external provider, not selectable here.", "true", "true", FieldTypeReadonly, nil, FieldReadonly, FieldReadonly, false, false),
				},
			},
		},
	}
	SyncDevelopmentDelivery(&draft)
	return draft
}

// SyncDevelopmentDelivery enforces governance-file versioning rules from delivery platform.
// Versioned is enabled only when a supported platform (GitHub) is selected.
func SyncDevelopmentDelivery(draft *ConfigDraft) {
	if draft == nil {
		return
	}
	mode := "none"
	if field, ok := draft.FieldByKey("source_control.mode"); ok {
		mode = strings.TrimSpace(field.Value)
	}
	for si := range draft.Sections {
		if draft.Sections[si].Key != "source_control" {
			continue
		}
		for fi := range draft.Sections[si].Fields {
			field := &draft.Sections[si].Fields[fi]
			if field.Key != "source_control.governance_storage" {
				continue
			}
			field.Options = []string{"local_only", "versioned"}
			field.Description = ""
			if mode == SourceControlGitGitHub {
				field.DisabledValues = nil
				if field.Value != GovernanceFilesLocalOnly && field.Value != GovernanceFilesVersioned {
					field.Value = GovernanceFilesLocalOnly
				}
			} else {
				field.DisabledValues = []string{"versioned"}
				field.Value = GovernanceFilesLocalOnly
			}
			return
		}
	}
}

func field(key, label, desc, value, def string, typ FieldType, options []string, initMut, confMut FieldMutability, required, sensitive bool) ConfigField {
	section := key
	if i := strings.IndexByte(key, '.'); i >= 0 {
		section = key[:i]
	}
	return ConfigField{
		Key:                 key,
		Label:               label,
		Description:         desc,
		Value:               value,
		Default:             def,
		Options:             options,
		Type:                typ,
		InitMutability:      initMut,
		ConfigureMutability: confMut,
		Section:             section,
		Required:            required,
		Sensitive:           sensitive,
	}
}

func slugID(name string) string {
	cleaned := strings.ToLower(strings.TrimSpace(name))
	cleaned = strings.ReplaceAll(cleaned, " ", "-")
	var b strings.Builder
	for _, r := range cleaned {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "project"
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// PreviewProjectID builds a non-persisted preview project id.
func PreviewProjectID(name string) string {
	return fmt.Sprintf("preview-%s", slugID(name))
}
