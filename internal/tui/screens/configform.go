package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// Config panel focus within the sectioned selector.
const (
	ConfigPanelSections = "sections"
	ConfigPanelFields   = "fields"
	ConfigPanelFooter   = "footer"
)

var (
	cfgFormTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	cfgFormSection   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	cfgFormFocus     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	cfgFormMuted     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cfgFormBody      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	cfgFormOK        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	cfgFormBadgeMigr = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	cfgFormBadgeLock = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cfgFormBadgeAI   = lipgloss.NewStyle().Foreground(lipgloss.Color("183"))
	cfgFormBadgeRO   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cfgFormSelected  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33"))
)

// ConfigFormView options for shared Init Step 2 / Configure rendering.
type ConfigFormView struct {
	Title          string
	Subtitle       string
	Draft          config.ConfigDraft
	SectionIndex   int
	FieldIndex     int
	OptionIndex    int
	PanelFocus     string
	FooterIndex    int
	ContentFocused bool
	Confirmed      bool
	ConfirmLines   []string
	ShowBack       bool
	ShowNext       bool
	BackLabel      string
	NextLabel      string
	FooterNote     string
	Width          int
	NextDisabled   bool
	MCP            MCPView
}

// ConfigFocusRow is one focusable selectable row inside a section.
type ConfigFocusRow struct {
	FieldIndex  int
	OptionIndex int
}

// FocusRows returns selectable rows for the active section (bool/choice/multi).
func FocusRows(draft config.ConfigDraft, sectionIdx int) []ConfigFocusRow {
	sections := draft.SelectorSections()
	if sectionIdx < 0 || sectionIdx >= len(sections) {
		return nil
	}
	fields := sections[sectionIdx].Fields
	rows := make([]ConfigFocusRow, 0, 16)
	for fi, field := range fields {
		if !field.Editable(draft.Mode) && field.Type != config.FieldTypeBool &&
			field.Type != config.FieldTypeChoice && field.Type != config.FieldTypeMulti {
			continue
		}
		switch field.Type {
		case config.FieldTypeBool:
			if field.Editable(draft.Mode) {
				rows = append(rows, ConfigFocusRow{FieldIndex: fi, OptionIndex: 0})
			}
		case config.FieldTypeChoice, config.FieldTypeMulti:
			if !field.Editable(draft.Mode) {
				continue
			}
			opts := field.VisibleOptions()
			for oi := range opts {
				rows = append(rows, ConfigFocusRow{FieldIndex: fi, OptionIndex: oi})
			}
		}
	}
	return rows
}

// ClampSelectorState normalizes selector indices against the draft.
func ClampSelectorState(draft config.ConfigDraft, sectionIdx, fieldIdx, optionIdx int) (int, int, int) {
	sections := draft.SelectorSections()
	if len(sections) == 0 {
		return 0, 0, 0
	}
	if sectionIdx < 0 {
		sectionIdx = 0
	}
	if sectionIdx >= len(sections) {
		sectionIdx = len(sections) - 1
	}
	rows := FocusRows(draft, sectionIdx)
	if len(rows) == 0 {
		return sectionIdx, 0, 0
	}
	for _, row := range rows {
		if row.FieldIndex == fieldIdx && row.OptionIndex == optionIdx {
			return sectionIdx, fieldIdx, optionIdx
		}
	}
	return sectionIdx, rows[0].FieldIndex, rows[0].OptionIndex
}

// ActiveField returns the focused field in the selector, if any.
func ActiveField(draft config.ConfigDraft, sectionIdx, fieldIdx int) (config.ConfigField, bool) {
	sections := draft.SelectorSections()
	if sectionIdx < 0 || sectionIdx >= len(sections) {
		return config.ConfigField{}, false
	}
	fields := sections[sectionIdx].Fields
	if fieldIdx < 0 || fieldIdx >= len(fields) {
		return config.ConfigField{}, false
	}
	return fields[fieldIdx], true
}

// RenderConfigForm renders the sectioned configuration option selector.
func RenderConfigForm(view ConfigFormView) string {
	var b strings.Builder
	width := view.Width
	if width < 40 {
		width = 72
	}

	title := cfgFormTitle.Render(view.Title)
	if view.Subtitle != "" {
		title += "  " + cfgFormMuted.Render(view.Subtitle)
	}
	if view.ContentFocused {
		title += "  " + cfgFormFocus.Render("[content focus]")
	} else {
		title += "  " + cfgFormMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)

	projectLine := fmt.Sprintf("Project: %s · %s", view.Draft.ProjectName(), view.Draft.ProjectModeLabel())
	fmt.Fprintln(&b, "  "+cfgFormBody.Render(projectLine))
	if view.FooterNote != "" {
		for _, line := range strings.Split(view.FooterNote, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fmt.Fprintln(&b, "  "+cfgFormMuted.Render(line))
		}
	}
	fmt.Fprintln(&b)

	for _, line := range view.ConfirmLines {
		fmt.Fprintln(&b, "  "+cfgFormOK.Render(line))
	}
	if view.Confirmed && len(view.ConfirmLines) == 0 {
		fmt.Fprintln(&b, "  "+cfgFormOK.Render("Configuration ready."))
	}
	if len(view.ConfirmLines) > 0 || view.Confirmed {
		fmt.Fprintln(&b)
	}

	sections := view.Draft.SelectorSections()
	sectionIdx, fieldIdx, optionIdx := ClampSelectorState(view.Draft, view.SectionIndex, view.FieldIndex, view.OptionIndex)

	leftWidth := 22
	if width < 60 {
		leftWidth = 18
	}
	rightWidth := width - leftWidth - 3
	if rightWidth < 20 {
		rightWidth = 20
	}

	left := renderSectionList(sections, sectionIdx, view.ContentFocused && view.PanelFocus == ConfigPanelSections, leftWidth)
	right := renderSectionDetail(view, sections, sectionIdx, fieldIdx, optionIdx, rightWidth)
	fmt.Fprint(&b, lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))

	return strings.TrimRight(b.String(), "\n")
}

// ActionFooterView describes compact shell action buttons for Init Step 2 / Configure / MCP.
type ActionFooterView struct {
	ShowBack       bool
	ShowNext       bool
	BackLabel      string
	NextLabel      string
	Confirmed      bool
	ContentFocused bool
	PanelFocus     string
	FooterIndex    int
	Width          int
	Help           string
	NextDisabled   bool
}

// RenderActionFooter renders a compact action row, optionally with a one-line help hint.
func RenderActionFooter(view ActionFooterView) string {
	width := view.Width
	if width < 1 {
		width = 1
	}
	actions := renderFooterBar(ConfigFormView{
		ShowBack:       view.ShowBack,
		ShowNext:       view.ShowNext,
		BackLabel:      view.BackLabel,
		NextLabel:      view.NextLabel,
		Confirmed:      view.Confirmed,
		ContentFocused: view.ContentFocused,
		PanelFocus:     view.PanelFocus,
		FooterIndex:    view.FooterIndex,
		NextDisabled:   view.NextDisabled,
	}, width)
	if strings.TrimSpace(view.Help) == "" {
		return strings.TrimRight(actions, "\n")
	}
	return strings.TrimRight(actions+"\n"+cfgFormMuted.Render(view.Help), "\n")
}

func renderSectionList(sections []config.ConfigSection, active int, focused bool, width int) string {
	var b strings.Builder
	fmt.Fprintln(&b, cfgFormSection.Render("Sections"))
	fmt.Fprintln(&b)
	for i, section := range sections {
		label := section.Title
		if i == active {
			if focused {
				fmt.Fprintln(&b, cfgFormSelected.Render("› "+label+" "))
			} else {
				fmt.Fprintln(&b, cfgFormBody.Render("› "+label))
			}
			continue
		}
		fmt.Fprintln(&b, cfgFormMuted.Render("  "+label))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.TrimRight(b.String(), "\n"))
}

func renderSectionDetail(view ConfigFormView, sections []config.ConfigSection, sectionIdx, fieldIdx, optionIdx, width int) string {
	var b strings.Builder
	if len(sections) == 0 {
		return cfgFormMuted.Render("No configuration sections.")
	}
	section := sections[sectionIdx]
	title := section.Title
	if section.Key == "mcp" && view.MCP.Mode == MCPModeAdd {
		title = "Add MCP"
	}
	fmt.Fprintln(&b, cfgFormSection.Render(title))
	if section.Description != "" {
		fmt.Fprintln(&b, cfgFormMuted.Render(section.Description))
	}
	fmt.Fprintln(&b)

	if section.Key == "mcp" {
		mcp := view.MCP
		mcp.ContentFocused = view.ContentFocused && view.PanelFocus == ConfigPanelFields
		fmt.Fprint(&b, RenderMCPPanel(mcp))
		return lipgloss.NewStyle().Width(width).Render(strings.TrimRight(b.String(), "\n"))
	}

	fieldsFocused := view.ContentFocused && view.PanelFocus == ConfigPanelFields
	for fi, field := range section.Fields {
		mut := field.Mutability(view.Draft.Mode)
		editable := field.Editable(view.Draft.Mode)

		switch field.Type {
		case config.FieldTypeBool:
			focused := fieldsFocused && fi == fieldIdx
			checked := strings.EqualFold(field.Value, "true")
			if field.Key == "source_control.delivery_assist" {
				fmt.Fprintln(&b, cfgFormBody.Render(field.Label))
				fmt.Fprintln(&b, "  "+renderCheckRow("Enabled", checked, focused, editable))
			} else {
				fmt.Fprintln(&b, "  "+renderCheckRow(field.Label, checked, focused, editable))
				if field.Description != "" {
					fmt.Fprintln(&b, "    "+cfgFormMuted.Render(field.Description))
				}
			}
			if note := mutabilityNote(mut); note != "" {
				fmt.Fprintln(&b, "    "+renderMutNote(mut, note))
			}
			fmt.Fprintln(&b)
		case config.FieldTypeReadonly:
			fmt.Fprintln(&b, cfgFormMuted.Render(field.Label))
			value := field.Value
			if value == "" {
				value = "—"
			}
			for _, line := range strings.Split(value, "\n") {
				fmt.Fprintln(&b, "  "+cfgFormBody.Render(line))
			}
			if field.Description != "" {
				fmt.Fprintln(&b, "  "+cfgFormMuted.Render(field.Description))
			}
			fmt.Fprintln(&b)
		default:
			if field.Label != "" {
				fmt.Fprintln(&b, cfgFormBody.Render(field.Label))
			}
			if field.Description != "" && strings.HasPrefix(field.Key, "governance.") {
				fmt.Fprintln(&b, "  "+cfgFormMuted.Render(field.Description))
			}
			if note := mutabilityNote(mut); note != "" {
				fmt.Fprintln(&b, "  "+renderMutNote(mut, note))
			}
			opts := field.VisibleOptions()
			for oi, opt := range opts {
				focused := fieldsFocused && fi == fieldIdx && oi == optionIdx
				checked := optionSelected(field, opt)
				fmt.Fprintln(&b, "  "+renderCheckRow(opt.Label, checked, focused, editable && !opt.Disabled))
			}
			fmt.Fprintln(&b)
		}
	}

	return lipgloss.NewStyle().Width(width).Render(strings.TrimRight(b.String(), "\n"))
}

func renderCheckRow(label string, checked, focused, editable bool) string {
	mark := "[ ]"
	if checked {
		mark = "[x]"
	}
	line := mark + " " + label
	switch {
	case focused:
		return cfgFormSelected.Render("› " + line + " ")
	case !editable:
		return cfgFormMuted.Render("  " + line)
	case checked:
		return cfgFormOK.Render("  " + line)
	default:
		return cfgFormBody.Render("  " + line)
	}
}

func optionSelected(field config.ConfigField, opt config.LabeledOption) bool {
	switch field.Type {
	case config.FieldTypeMulti:
		return config.ChipSelected(field.Value, opt.Value)
	case config.FieldTypeBool:
		return strings.EqualFold(field.Value, "true")
	default:
		return opt.Value == field.Value
	}
}

func mutabilityNote(m config.FieldMutability) string {
	switch m {
	case config.FieldLocked:
		return "locked"
	case config.FieldMigrationRequired:
		return "migration required"
	case config.FieldAIManaged:
		return "ai-managed"
	case config.FieldLocalOnly:
		return "local-only"
	case config.FieldReadonly:
		return "readonly"
	default:
		return ""
	}
}

func renderMutNote(m config.FieldMutability, note string) string {
	switch m {
	case config.FieldMigrationRequired:
		return cfgFormBadgeMigr.Render(note)
	case config.FieldAIManaged:
		return cfgFormBadgeAI.Render(note)
	case config.FieldLocked:
		return cfgFormBadgeLock.Render(note)
	default:
		return cfgFormBadgeRO.Render(note)
	}
}

func renderFooterBar(view ConfigFormView, width int) string {
	backLabel := view.BackLabel
	if backLabel == "" {
		backLabel = "Back"
	}
	nextLabel := view.NextLabel
	if nextLabel == "" {
		nextLabel = "Next"
	}

	footerFocused := view.ContentFocused && view.PanelFocus == ConfigPanelFooter
	backFocused := footerFocused && view.FooterIndex == 0 && view.ShowBack
	nextFocused := footerFocused && view.ShowNext && ((view.ShowBack && view.FooterIndex == 1) || (!view.ShowBack && view.FooterIndex == 0))

	back := ""
	if view.ShowBack {
		back = footerButton(backLabel, backFocused, false, false)
	}
	next := ""
	if view.ShowNext {
		done := view.Confirmed && !view.NextDisabled
		label := nextLabel
		if done {
			label = nextLabel + " ✓"
		}
		next = footerButton(label, nextFocused, done, view.NextDisabled)
	}

	if back == "" && next == "" {
		return ""
	}
	if next == "" {
		return back
	}
	if back == "" {
		return next
	}

	gap := width - lipgloss.Width(back) - lipgloss.Width(next)
	if gap < 2 {
		return back + "  " + next
	}
	return back + strings.Repeat(" ", gap) + next
}

func footerButton(label string, focused, done, disabled bool) string {
	text := "[ " + label + " ]"
	if focused {
		return cfgFormSelected.Render(text)
	}
	if disabled {
		return cfgFormMuted.Render(text)
	}
	if done {
		return cfgFormOK.Render(text)
	}
	return cfgFormBody.Render(text)
}
