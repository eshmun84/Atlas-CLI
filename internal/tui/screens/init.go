package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	initTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	initSection  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	initLabel    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	initSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33"))
	initOption   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	initWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	initMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	initFocus    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	initInputIdle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1).
			Width(36)

	initInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("245")).
				Foreground(lipgloss.Color("252")).
				Padding(0, 1).
				Width(36)
)

// Init field identifiers used by the TUI model and renderer.
const (
	InitFieldName = iota
	InitFieldModeNew
	InitFieldModeExisting
	InitFieldDecisionInit
	InitFieldDecisionCancel
	InitFieldNext
)

// Init wizard steps.
const (
	InitWizardStepProject = 1
	InitWizardStepConfig  = 2
	InitWizardStepReview  = 3
)

// InitView is the in-memory Init / Setup wizard draft for Step 1.
type InitView struct {
	RootPath        string
	DetectedName    string
	DetectedMode    string
	RecommendedMode string
	DraftName       string
	NameInputView   string
	ModeConfirmed   string
	Decision        string
	Artifacts       []string
	ActiveField     int
	ContentFocused  bool
}

// InitPlan renders Init / Setup Step 1 — Project Setup.
func InitPlan(view InitView) string {
	var b strings.Builder

	title := initTitle.Render("Init / Setup") + "  " + initMuted.Render("Step 1 — Project Setup")
	if view.ContentFocused {
		title += "  " + initFocus.Render("[content focus]")
	} else {
		title += "  " + initMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Project Identity"))
	writeNameField(&b, view)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Project Detection"))
	fmt.Fprintf(&b, "  Root: %s\n", view.RootPath)
	fmt.Fprintf(&b, "  Inferred name: %s\n", view.DetectedName)
	fmt.Fprintf(&b, "  Detected mode: %s\n", view.DetectedMode)
	fmt.Fprintf(&b, "  Recommended mode: %s\n\n", recommendedLabel(view.RecommendedMode))

	fmt.Fprintln(&b, initSection.Render("Project Mode"))
	writeInitOption(&b, view, InitFieldModeNew, "New project", view.ModeConfirmed == "new")
	writeInitOption(&b, view, InitFieldModeExisting, "Existing project", view.ModeConfirmed == "existing")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Runtime Artifacts"))
	if len(view.Artifacts) == 0 {
		fmt.Fprintln(&b, "  "+initMuted.Render("No existing runtime/adaptor artifacts detected."))
		fmt.Fprintln(&b, "  "+initMuted.Render("Atlas can initialize normally."))
	} else {
		fmt.Fprintln(&b, "  "+initWarn.Render("Existing runtime/adaptor artifacts detected:"))
		for _, path := range view.Artifacts {
			fmt.Fprintf(&b, "    - %s\n", path)
		}
		fmt.Fprintln(&b, "  "+initWarn.Render("Atlas will not merge existing agent/adaptor structures."))
		fmt.Fprintln(&b, "  "+initWarn.Render("Existing runtime artifacts will require backup-and-replace if init is later applied."))
		fmt.Fprintln(&b, "  "+initMuted.Render("No backup or replacement happens in this slice."))
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, initSection.Render("Initialization Decision"))
		writeInitOption(&b, view, InitFieldDecisionInit, "Initialize Atlas — backup and replace runtime artifacts", view.Decision == "initialize")
		writeInitOption(&b, view, InitFieldDecisionCancel, "Do not initialize Atlas", view.Decision == "cancel")
	}
	fmt.Fprintln(&b)

	if len(view.Artifacts) > 0 && view.Decision == "cancel" {
		fmt.Fprintln(&b, "  "+initWarn.Render("Initialization canceled — no files would be changed."))
	} else {
		fmt.Fprintln(&b, "  "+initMuted.Render("No files will be changed in this slice."))
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeNameField(b *strings.Builder, view InitView) {
	fmt.Fprintln(b, "  "+initLabel.Render("Project name:"))

	value := strings.TrimSpace(view.NameInputView)
	if value == "" {
		value = view.DraftName
	}
	focused := view.ContentFocused && view.ActiveField == InitFieldName
	box := initInputIdle.Render(value)
	prefix := "  "
	if focused {
		box = initInputFocused.Render(value)
		prefix = initFocus.Render("›") + " "
	}
	for i, line := range strings.Split(box, "\n") {
		if i == 0 {
			fmt.Fprintf(b, "%s%s\n", prefix, line)
			continue
		}
		fmt.Fprintf(b, "  %s\n", line)
	}
}

func writeInitOption(b *strings.Builder, view InitView, field int, label string, confirmed bool) {
	mark := "  " + label
	if confirmed {
		mark = "✓ " + label
	}
	if view.ContentFocused && view.ActiveField == field {
		fmt.Fprintf(b, "  %s\n", initSelected.Render("› "+mark+" "))
		return
	}
	fmt.Fprintf(b, "  %s\n", initOption.Render("  "+mark))
}

func recommendedLabel(mode string) string {
	switch mode {
	case "new":
		return "New project"
	case "existing":
		return "Existing project"
	default:
		return mode
	}
}
