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
	initOK       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))

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
	InitFieldConflictRefresh
	InitFieldConflictExit
	InitFieldConflictContinue
	InitFieldNext
)

// Init wizard steps.
const (
	InitWizardStepConflict = 0
	InitWizardStepProject  = 1
	InitWizardStepConfig   = 2
	InitWizardStepReview   = 3
)

// InitView is the in-memory Init / Setup wizard draft for Step 1.
type InitView struct {
	RootPath       string
	DetectedName   string
	DetectedMode   string
	AtlasState     string
	DraftName      string
	NameInputView  string
	ModeConfirmed  string
	ActiveField    int
	ContentFocused bool
}

// InitConflictView is the blocking runtime-conflict preflight before Project Setup.
type InitConflictView struct {
	RootPath       string
	Artifacts      []string
	ActiveField    int
	ContentFocused bool
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

	fmt.Fprintln(&b, initSection.Render("Project Name"))
	writeNameField(&b, view)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Project Mode"))
	fmt.Fprintln(&b, "  "+initMuted.Render("←/→ focus · Space/Enter select · Atlas does not own GitFlow."))
	writeInitModeOptions(&b, view,
		initModeChoice{InitFieldModeNew, "New Project", view.ModeConfirmed == "new"},
		initModeChoice{InitFieldModeExisting, "Existing Project", view.ModeConfirmed == "existing"},
	)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "  "+initMuted.Render("No files are written until Review → Apply."))
	return strings.TrimRight(b.String(), "\n")
}

// InitConflict renders the blocking runtime artifact conflict preflight.
func InitConflict(view InitConflictView) string {
	var b strings.Builder

	if len(view.Artifacts) == 0 {
		title := initTitle.Render("Init / Setup") + "  " + initOK.Render("Ready")
		if view.ContentFocused {
			title += "  " + initFocus.Render("[content focus]")
		} else {
			title += "  " + initMuted.Render("[sidebar focus]")
		}
		fmt.Fprintln(&b, title)
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, initOK.Render("No runtime conflicts detected."))
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "  "+initMuted.Render("Continue to Setup enters Project Setup. Exit / Back leaves without changes."))
		return strings.TrimRight(b.String(), "\n")
	}

	title := initTitle.Render("Init / Setup") + "  " + initWarn.Render("Runtime conflict")
	if view.ContentFocused {
		title += "  " + initFocus.Render("[content focus]")
	} else {
		title += "  " + initMuted.Render("[sidebar focus]")
	}
	fmt.Fprintln(&b, title)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Blocking warning"))
	fmt.Fprintln(&b, "  "+initWarn.Render("Conflicting runtime surfaces were detected in this project."))
	fmt.Fprintln(&b, "  "+initWarn.Render("Atlas Init cannot continue until the project is cleaned manually."))
	fmt.Fprintln(&b, "  "+initMuted.Render("Refresh / Re-check only re-scans the project. It does not delete, move, backup, or write files."))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Detected conflicts"))
	fmt.Fprintf(&b, "  Root: %s\n", view.RootPath)
	for _, path := range view.Artifacts {
		fmt.Fprintf(&b, "  - %s\n", path)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Manual cleanup steps"))
	fmt.Fprintln(&b, "  1. Review the detected files/directories.")
	fmt.Fprintln(&b, "  2. Move them outside the project or back them up manually.")
	fmt.Fprintln(&b, "  3. Use Refresh / Re-check after the project is clean.")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "  "+initMuted.Render("Allowed actions: Refresh / Re-check, or Exit / Back."))
	return strings.TrimRight(b.String(), "\n")
}

type initModeChoice struct {
	field     int
	label     string
	confirmed bool
}

func writeInitModeOptions(b *strings.Builder, view InitView, choices ...initModeChoice) {
	parts := make([]string, 0, len(choices))
	for _, choice := range choices {
		mark := "[ ]"
		if choice.confirmed {
			mark = "[x]"
		}
		line := mark + " " + choice.label
		focused := view.ContentFocused && view.ActiveField == choice.field
		switch {
		case focused:
			parts = append(parts, initSelected.Render("› "+line+" "))
		case choice.confirmed:
			parts = append(parts, initOK.Render(line))
		default:
			parts = append(parts, initOption.Render(line))
		}
	}
	fmt.Fprintf(b, "  %s\n", strings.Join(parts, "  "))
}

func writeNameField(b *strings.Builder, view InitView) {
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
