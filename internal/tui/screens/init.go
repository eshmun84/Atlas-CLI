package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

var (
	initTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	initSection   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	initLabel     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	initSelected  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33"))
	initOption    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	initConfirmed = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	initWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	initMuted     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	initFocus     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	initCreate    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	initSkip      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	initFuture    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

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

	initButtonIdle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 2)

	initButtonFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("33")).
				Bold(true).
				Padding(0, 2)

	initButtonDone = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("42")).
			Foreground(lipgloss.Color("42")).
			Padding(0, 2)
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

// InitView is the in-memory Init / Setup wizard draft.
type InitView struct {
	Plan            initplan.Plan
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
	StepConfirmed   bool
}

// InitPlan renders the Init / Setup identity, mode, and runtime artifact gate.
func InitPlan(view InitView) string {
	var b strings.Builder

	title := initTitle.Render("Init / Setup")
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
	fmt.Fprintf(&b, "  Root: %s\n", view.Plan.RootPath)
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
		fmt.Fprintln(&b, "  "+initWarn.Render("Existing artifacts can contradict Atlas governance."))
		fmt.Fprintln(&b, "  "+initWarn.Render("When materialization is implemented, Atlas will back them up and replace them."))
		fmt.Fprintln(&b, "  "+initMuted.Render("No backup or replacement happens in this slice."))
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, initSection.Render("Initialization Decision"))
		writeInitOption(&b, view, InitFieldDecisionInit, "Initialize Atlas — backup and replace runtime artifacts", view.Decision == "initialize")
		writeInitOption(&b, view, InitFieldDecisionCancel, "Do not initialize Atlas", view.Decision == "cancel")
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Plan Preview"))
	if len(view.Artifacts) > 0 && view.Decision == "cancel" {
		fmt.Fprintln(&b, "  "+initMuted.Render("Initialization inactive. Plan preview suppressed."))
		fmt.Fprintln(&b, "  "+initMuted.Render("No materialization, backup, or replacement would occur."))
	} else {
		fmt.Fprintf(&b, "  Name: %s\n", view.Plan.ProjectName)
		fmt.Fprintf(&b, "  Mode: %s\n", view.Plan.ProjectMode)
		fmt.Fprintf(&b, "  Root: %s\n", view.Plan.RootPath)
		fmt.Fprintf(&b, "  Draft mode: %s → %s\n\n", modeLabel(view.ModeConfirmed), view.Plan.ProjectMode)
		if len(view.Plan.Steps) == 0 {
			fmt.Fprintln(&b, "  none")
		} else {
			for _, step := range view.Plan.Steps {
				style := initCreate
				switch step.Status {
				case initplan.StatusSkipExisting:
					style = initSkip
				case initplan.StatusFuture, initplan.StatusWarning:
					style = initFuture
				}
				fmt.Fprintf(&b, "  %s %s — %s\n", style.Render(step.Status), step.Path, step.Reason)
			}
		}
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, initSection.Render("Future AGENTS.md policy"))
		fmt.Fprintln(&b, "  "+initMuted.Render("Atlas-generated AGENTS.md will contain an Atlas-managed section."))
		fmt.Fprintln(&b, "  "+initMuted.Render("Developer custom content should go into a user section."))
		fmt.Fprintln(&b, "  "+initMuted.Render("On first init, an existing AGENTS.md is backed up and replaced."))
		fmt.Fprintln(&b, "  "+initMuted.Render("On later reconfigure/update, Atlas preserves the user section."))
	}
	fmt.Fprintln(&b)

	if len(view.Artifacts) > 0 && view.Decision == "cancel" {
		fmt.Fprintln(&b, "  "+initWarn.Render("Initialization canceled — no files would be changed."))
	}
	if view.StepConfirmed {
		fmt.Fprintln(&b, "  "+initConfirmed.Render("Step ready."))
		fmt.Fprintln(&b, "  "+initMuted.Render("Next wizard step is not implemented in this slice."))
		fmt.Fprintln(&b, "  "+initMuted.Render("No files were changed."))
	} else {
		fmt.Fprintln(&b, "  "+initMuted.Render("No files will be created in this step."))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, initSection.Render("Action"))
	writeNextButton(&b, view)

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

func writeNextButton(b *strings.Builder, view InitView) {
	focused := view.ContentFocused && view.ActiveField == InitFieldNext
	label := "Next"
	style := initButtonIdle
	prefix := "  "
	switch {
	case focused:
		style = initButtonFocused
		prefix = initFocus.Render("›") + " "
	case view.StepConfirmed:
		style = initButtonDone
		label = "Next ✓"
	}
	btn := style.Render(label)
	for i, line := range strings.Split(btn, "\n") {
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

func modeLabel(mode string) string {
	return recommendedLabel(mode)
}
