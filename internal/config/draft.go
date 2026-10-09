package config

import "strings"

// ConfigMode selects field mutability for Init vs Configure.
type ConfigMode string

const (
	ConfigModeInit      ConfigMode = "init"
	ConfigModeConfigure ConfigMode = "configure"
)

// FieldMutability classifies how a draft field may be changed in a given mode.
type FieldMutability string

const (
	FieldEditable          FieldMutability = "editable"
	FieldLocked            FieldMutability = "locked"
	FieldMigrationRequired FieldMutability = "migration_required"
	FieldAIManaged         FieldMutability = "ai_managed"
	FieldLocalOnly         FieldMutability = "local_only"
	FieldReadonly          FieldMutability = "readonly"
)

// FieldType is the UI/control type for a draft field.
type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypeBool     FieldType = "bool"
	FieldTypeChoice   FieldType = "choice"
	FieldTypeMulti    FieldType = "multi"
	FieldTypeReadonly FieldType = "readonly"
)

// LabeledOption is a visible selectable option for a draft field.
type LabeledOption struct {
	Value    string
	Label    string
	Disabled bool
}

// ConfigField is one typed configuration field with mode-specific mutability.
type ConfigField struct {
	Key                 string
	Label               string
	Description         string
	Value               string
	Default             string
	Options             []string
	DisabledValues      []string // visible but not selectable option values
	Type                FieldType
	InitMutability      FieldMutability
	ConfigureMutability FieldMutability
	Section             string
	Sensitive           bool
	Required            bool
}

// Mutability returns the effective mutability for mode.
func (f ConfigField) Mutability(mode ConfigMode) FieldMutability {
	switch mode {
	case ConfigModeConfigure:
		return f.ConfigureMutability
	default:
		return f.InitMutability
	}
}

// Editable reports whether the field can be changed in mode.
func (f ConfigField) Editable(mode ConfigMode) bool {
	return f.Mutability(mode) == FieldEditable
}

// VisibleOptions returns display options for the sectioned selector UI.
func (f ConfigField) VisibleOptions() []LabeledOption {
	switch f.Type {
	case FieldTypeBool:
		return boolOptions(f.Key)
	case FieldTypeChoice, FieldTypeMulti:
		out := make([]LabeledOption, 0, len(f.Options))
		for _, opt := range f.Options {
			out = append(out, LabeledOption{
				Value:    opt,
				Label:    optionDisplayLabel(f, opt),
				Disabled: f.OptionDisabled(opt),
			})
		}
		return out
	case FieldTypeReadonly, FieldTypeText:
		if strings.TrimSpace(f.Value) == "" {
			return nil
		}
		parts := SplitChips(f.Value)
		if len(parts) == 0 {
			return []LabeledOption{{Value: f.Value, Label: OptionLabel(f.Value)}}
		}
		out := make([]LabeledOption, 0, len(parts))
		for _, part := range parts {
			out = append(out, LabeledOption{Value: part, Label: OptionLabel(part)})
		}
		return out
	default:
		return nil
	}
}

func optionDisplayLabel(f ConfigField, value string) string {
	if f.Key == "adapters.selected" {
		name := OptionLabel(value)
		if f.OptionDisabled(value) {
			return name + "      Not available"
		}
		return name + "      Available"
	}
	return OptionLabel(value)
}

// OptionIndexOf returns the index of value among visible options, or 0.
func (f ConfigField) OptionIndexOf(value string) int {
	opts := f.VisibleOptions()
	for i, opt := range opts {
		if opt.Value == value {
			return i
		}
		if f.Type == FieldTypeBool && boolString(opt.Value) == boolString(value) {
			return i
		}
	}
	if f.Type == FieldTypeMulti {
		for i, opt := range opts {
			if ChipSelected(f.Value, opt.Value) {
				return i
			}
		}
	}
	return 0
}

// ConfigSection groups related draft fields.
type ConfigSection struct {
	Key         string
	Title       string
	Description string
	Fields      []ConfigField
}

// ConfigDraft is an in-memory configuration draft for Init / Configure.
type ConfigDraft struct {
	Mode     ConfigMode
	Sections []ConfigSection
}

// SelectorSections returns user-facing configuration sections.
func (d ConfigDraft) SelectorSections() []ConfigSection {
	out := make([]ConfigSection, 0, len(d.Sections))
	for _, section := range d.Sections {
		switch section.Key {
		case "governance", "adapters", "source_control", "mcp":
			out = append(out, section)
		}
	}
	return out
}

// OptionDisabled reports whether value is visible but not selectable.
func (f ConfigField) OptionDisabled(value string) bool {
	value = strings.TrimSpace(value)
	for _, disabled := range f.DisabledValues {
		if disabled == value {
			return true
		}
	}
	return false
}

// ProjectName returns project.name from the draft.
func (d ConfigDraft) ProjectName() string {
	if field, ok := d.FieldByKey("project.name"); ok {
		return field.Value
	}
	return "project"
}

// ProjectMode returns project.mode from the draft.
func (d ConfigDraft) ProjectMode() string {
	if field, ok := d.FieldByKey("project.mode"); ok {
		return field.Value
	}
	return ModeExisting
}

// ProjectModeLabel returns a human label for project.mode.
func (d ConfigDraft) ProjectModeLabel() string {
	return FormatProjectModeLabel(d.ProjectMode())
}

// FieldByKey finds a field by dotted key.
func (d ConfigDraft) FieldByKey(key string) (ConfigField, bool) {
	for _, section := range d.Sections {
		for _, field := range section.Fields {
			if field.Key == key {
				return field, true
			}
		}
	}
	return ConfigField{}, false
}

// SetValue updates a field value when editable in the draft mode.
func (d *ConfigDraft) SetValue(key, value string) bool {
	for si := range d.Sections {
		for fi := range d.Sections[si].Fields {
			field := &d.Sections[si].Fields[fi]
			if field.Key != key {
				continue
			}
			if !field.Editable(d.Mode) {
				return false
			}
			field.Value = value
			return true
		}
	}
	return false
}

// SelectOption sets a field to a visible option value when editable.
func (d *ConfigDraft) SelectOption(key, value string) bool {
	field, ok := d.FieldByKey(key)
	if !ok || !field.Editable(d.Mode) {
		return false
	}
	var okSet bool
	switch field.Type {
	case FieldTypeBool:
		okSet = d.SetValue(key, boolString(value))
	case FieldTypeChoice:
		allowed := false
		for _, opt := range field.Options {
			if opt == value {
				allowed = true
				break
			}
		}
		if !allowed || field.OptionDisabled(value) {
			return false
		}
		okSet = d.SetValue(key, value)
	case FieldTypeText:
		okSet = d.SetValue(key, value)
	case FieldTypeMulti:
		okSet = d.ToggleMulti(key, value)
	default:
		return false
	}
	if okSet && (key == "source_control.mode" || key == "source_control.governance_storage") {
		SyncDevelopmentDelivery(d)
	}
	return okSet
}

// ToggleMulti toggles a value in a multi-select field.
// UI activation must enforce OptionDisabled separately so tests/setup can seed values.
func (d *ConfigDraft) ToggleMulti(key, value string) bool {
	field, ok := d.FieldByKey(key)
	if !ok || field.Type != FieldTypeMulti || !field.Editable(d.Mode) {
		return false
	}
	return d.SetValue(key, ToggleChip(field.Value, value))
}

// ToggleBool toggles a bool field when editable.
func (d *ConfigDraft) ToggleBool(key string) bool {
	field, ok := d.FieldByKey(key)
	if !ok || field.Type != FieldTypeBool || !field.Editable(d.Mode) {
		return false
	}
	next := "true"
	if strings.EqualFold(field.Value, "true") {
		next = "false"
	}
	return d.SetValue(key, next)
}

// FormatProjectModeLabel maps a project mode value to UI copy.
func FormatProjectModeLabel(mode string) string {
	switch NormalizeProjectMode(mode) {
	case ModeGreenfield:
		return "New project"
	default:
		return "Existing project"
	}
}

// OptionLabel maps a stored option value to a visible label.
func OptionLabel(value string) string {
	switch value {
	case "sdd":
		return "SDD"
	case "openspec":
		return "OpenSpec"
	case "none":
		return "None"
	case "cursor":
		return "Cursor"
	case "opencode":
		return "OpenCode"
	case "git_local":
		return "Git local"
	case "git_github":
		return "GitHub"
	case "manual":
		return "Manual"
	case "simple":
		return "Simple"
	case "main_develop":
		return "Main + develop"
	case "main_develop_staging":
		return "Main + develop + staging"
	case "local_only":
		return "Local only"
	case "versioned":
		return "Versioned"
	case "sqlite":
		return "SQLite"
	case "context_capsule":
		return "Context Capsule"
	case "sqlite_plus_context_capsule":
		return "SQLite + Context Capsule"
	case "true":
		return "Yes"
	case "false":
		return "No"
	default:
		return value
	}
}

// SplitChips splits a comma-separated chip value.
func SplitChips(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// JoinChips joins chips with ", ".
func JoinChips(values []string) string {
	return strings.Join(values, ", ")
}

// ChipSelected reports whether value is present in a chip list (case-insensitive).
func ChipSelected(list, value string) bool {
	value = strings.TrimSpace(value)
	for _, part := range SplitChips(list) {
		if strings.EqualFold(part, value) {
			return true
		}
	}
	return false
}

// ToggleChip toggles value in a chip list.
func ToggleChip(list, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return list
	}
	parts := SplitChips(list)
	out := make([]string, 0, len(parts)+1)
	found := false
	for _, part := range parts {
		if strings.EqualFold(part, value) {
			found = true
			continue
		}
		out = append(out, part)
	}
	if !found {
		out = append(out, value)
	}
	return JoinChips(out)
}

func boolOptions(key string) []LabeledOption {
	switch key {
	case "governance.testing_required",
		"governance.review_required",
		"governance.evidence_required":
		return []LabeledOption{
			{Value: "true", Label: "Yes"},
			{Value: "false", Label: "No"},
		}
	case "source_control.delivery_assist":
		return []LabeledOption{
			{Value: "false", Label: "Disabled"},
			{Value: "true", Label: "Enabled"},
		}
	default:
		return []LabeledOption{
			{Value: "true", Label: "Enabled"},
			{Value: "false", Label: "Disabled"},
		}
	}
}

func boolString(v string) string {
	if strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") || strings.EqualFold(v, "enabled") {
		return "true"
	}
	return "false"
}
