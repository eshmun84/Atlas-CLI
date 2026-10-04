package config

import (
	"fmt"
	"strings"
)

// ProjectSetupInput carries Step 1 values used to seed a ConfigDraft.
type ProjectSetupInput struct {
	ProjectName      string
	ProjectMode      string
	ProjectID        string
	DefaultRemote    string
	CursorDetected   bool
	OpenCodeDetected bool
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

	adapters := ""
	if setup.CursorDetected {
		adapters = ToggleChip(adapters, "cursor")
	}
	if setup.OpenCodeDetected {
		adapters = ToggleChip(adapters, "opencode")
	}

	return ConfigDraft{
		Mode: mode,
		Sections: []ConfigSection{
			{
				Key:         "project",
				Title:       "Project",
				Description: "Project identity from setup (context only).",
				Fields: []ConfigField{
					field("project.name", "Project name", "", setup.ProjectName, setup.ProjectName, FieldTypeReadonly, nil, FieldReadonly, FieldLocked, true, false),
					field("project.mode", "Project mode", "", setup.ProjectMode, setup.ProjectMode, FieldTypeReadonly, nil, FieldReadonly, FieldLocked, true, false),
				},
			},
			{
				Key:         "governance",
				Title:       "Governance",
				Description: "Atlas always governs AI-assisted work. Evidence required means the agent must report verification evidence when applicable.",
				Fields: []ConfigField{
					field("governance.default_workflow", "Workflow", "", "sdd", "sdd", FieldTypeChoice, []string{"sdd"}, FieldEditable, FieldEditable, true, false),
					field("governance.spec_engine", "Spec engine", "", "openspec", "openspec", FieldTypeChoice, []string{"openspec", "none"}, FieldEditable, FieldEditable, true, false),
					field("governance.testing_required", "Testing required", "", "true", "true", FieldTypeBool, nil, FieldEditable, FieldEditable, true, false),
					field("governance.review_required", "Review required", "", "true", "true", FieldTypeBool, nil, FieldEditable, FieldEditable, true, false),
					field("governance.evidence_required", "Evidence required", "The agent must report verification evidence, commands, tests, smoke, results, and limitations when applicable.", "true", "true", FieldTypeBool, nil, FieldEditable, FieldEditable, true, false),
				},
			},
			{
				Key:         "adapters",
				Title:       "Adapters",
				Description: "Select the tools Atlas should prepare runtime instructions for.",
				Fields: []ConfigField{
					field("adapters.selected", "Adapters", "", adapters, adapters, FieldTypeMulti, []string{"cursor", "opencode"}, FieldEditable, FieldEditable, false, false),
				},
			},
			{
				Key:         "source_control",
				Title:       "Source Control",
				Description: "Define how Atlas may assist with Git-related workflow. Atlas never runs Git operations without explicit approval.",
				Fields: []ConfigField{
					field("source_control.mode", "Source control mode", "", "none", "none", FieldTypeChoice, []string{"none", "git_local", "git_github"}, FieldEditable, FieldEditable, true, false),
					field("source_control.default_remote", "Default remote", "Display only in this slice.", remote, remote, FieldTypeReadonly, nil, FieldReadonly, FieldReadonly, false, false),
					field("source_control.branch_strategy", "Branch strategy", "", "manual", "manual", FieldTypeChoice, []string{"manual", "simple", "main_develop", "main_develop_staging"}, FieldEditable, FieldEditable, false, false),
					field("source_control.governance_storage", "Atlas governance files", "Local only keeps Atlas governance/spec/runtime files local where appropriate. Versioned allows those files under explicit Atlas policy. Also covers Spec Engine storage.", "local_only", "local_only", FieldTypeChoice, []string{"local_only", "versioned"}, FieldEditable, FieldEditable, true, false),
					field("source_control.delivery_assist", "Delivery assist", "Even when enabled, Atlas never runs Git operations without explicit approval.", "false", "false", FieldTypeBool, nil, FieldEditable, FieldEditable, false, false),
				},
			},
			{
				Key:         "memory",
				Title:       "Memory",
				Description: "Atlas always has memory. SQLite stores structured local Atlas memory. Context Capsule gives agents compact context derived from project memory. SQLite + Context Capsule is the recommended default.",
				Fields: []ConfigField{
					field(
						"memory.strategy",
						"Memory strategy",
						"",
						"sqlite_plus_context_capsule",
						"sqlite_plus_context_capsule",
						FieldTypeChoice,
						[]string{"sqlite", "context_capsule", "sqlite_plus_context_capsule"},
						FieldEditable,
						FieldEditable,
						true,
						false,
					),
				},
			},
			{
				Key:         "mcp",
				Title:       "MCP",
				Description: "Configure external MCP integrations for Atlas.",
				Fields:      nil,
			},
		},
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
