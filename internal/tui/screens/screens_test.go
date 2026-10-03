package screens_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestScreenTitles(t *testing.T) {
	t.Parallel()

	if !strings.Contains(screens.Help(), "Atlas Help") {
		t.Fatal("help")
	}
	if !strings.Contains(screens.Status(workspace.DiscoveryResult{RootPath: "/tmp"}), "Atlas Status") {
		t.Fatal("status")
	}
	if !strings.Contains(screens.Doctor(doctor.Report{}), "Atlas Doctor") {
		t.Fatal("doctor")
	}
	if !strings.Contains(screens.Dashboard(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Atlas:    workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
	}), "Dashboard") {
		t.Fatal("dashboard")
	}
	if !strings.Contains(screens.Configure(workspace.DiscoveryResult{
		Atlas: workspace.AtlasStatus{State: workspace.AtlasStateInitialized, ConfigPath: "/tmp/.atlas/config.yaml"},
	}), "Configure") {
		t.Fatal("configure")
	}
}

func TestDashboardView(t *testing.T) {
	t.Parallel()

	view := screens.Dashboard(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Atlas:    workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
		Git:      workspace.GitInfo{IsRepo: true, CurrentBranch: "develop", DefaultRemote: "origin", DefaultBranch: "main"},
		Technologies: []workspace.Technology{
			{Name: "Go", Source: "go.mod", Confidence: workspace.ConfidenceHigh},
			{Name: "Go modules", Source: "go.mod", Confidence: workspace.ConfidenceHigh},
		},
		Libraries: []workspace.Library{{Name: "Bubble Tea", Module: "github.com/charmbracelet/bubbletea"}},
	})
	for _, want := range []string{
		"Dashboard",
		"Atlas state",
		"Not initialized",
		"Suggested next action",
		"Run Init / Setup",
		"Technologies",
		"Libraries",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

func TestInitPlanWizardStep(t *testing.T) {
	t.Parallel()

	view := screens.InitPlan(screens.InitView{
		Plan: initplan.Plan{
			RootPath:    "/tmp/demo",
			ProjectName: "demo",
			ProjectMode: "existing",
			Steps: []initplan.Step{
				{Status: initplan.StatusCreate, Path: "AGENTS.md", Reason: "project runtime governance entrypoint"},
			},
		},
		DetectedName:    "demo",
		DetectedMode:    "existing",
		RecommendedMode: "existing",
		DraftName:       "demo",
		ModeConfirmed:   "existing",
		Decision:        "initialize",
		Artifacts:       []string{"AGENTS.md", ".cursor"},
		ActiveField:     screens.InitFieldModeExisting,
		ContentFocused:  true,
	})

	for _, want := range []string{
		"Init / Setup",
		"Project Identity",
		"Project name:",
		"Project Detection",
		"Detected mode",
		"Recommended mode",
		"Project Mode",
		"New project",
		"Existing project",
		"Runtime Artifacts",
		"backup and replace",
		"Initialize Atlas",
		"Do not initialize Atlas",
		"Plan Preview",
		"No files will be created in this step.",
		"Action",
		"Next",
		"[content focus]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in init view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Auto-detect") {
		t.Fatalf("Auto-detect must not appear as an option:\n%s", view)
	}
	assertNameFieldLooksLikeInput(t, view)
	assertActionAfterPlanPreview(t, view)
	assertNextLooksLikeButton(t, view)
}

func TestInitPlanNoArtifactsHidesDecision(t *testing.T) {
	t.Parallel()

	view := screens.InitPlan(screens.InitView{
		Plan:            initplan.Plan{ProjectName: "demo", ProjectMode: "greenfield"},
		DetectedName:    "demo",
		DetectedMode:    "greenfield",
		RecommendedMode: "new",
		DraftName:       "demo",
		ModeConfirmed:   "new",
		Decision:        "initialize",
		Artifacts:       nil,
		ActiveField:     screens.InitFieldNext,
		ContentFocused:  true,
		StepConfirmed:   true,
	})
	for _, want := range []string{
		"No existing runtime/adaptor artifacts detected.",
		"Atlas can initialize normally.",
		"Next",
		"Step ready.",
		"Next wizard step is not implemented in this slice.",
		"No files were changed.",
		"Action",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	assertNameFieldLooksLikeInput(t, view)
	assertActionAfterPlanPreview(t, view)
	assertNextLooksLikeButton(t, view)
	for _, banned := range []string{
		"Do not initialize Atlas",
		"Initialize Atlas — backup and replace",
		"Initialization Decision",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q when no artifacts:\n%s", banned, view)
		}
	}
}

func TestInitPlanCanceledDecision(t *testing.T) {
	t.Parallel()

	view := screens.InitPlan(screens.InitView{
		Plan:            initplan.Plan{ProjectName: "demo", ProjectMode: "existing"},
		DetectedName:    "demo",
		DetectedMode:    "existing",
		RecommendedMode: "existing",
		DraftName:       "demo",
		ModeConfirmed:   "existing",
		Decision:        "cancel",
		Artifacts:       []string{"AGENTS.md"},
		ContentFocused:  true,
	})
	if !strings.Contains(view, "Initialization canceled") {
		t.Fatalf("expected canceled message:\n%s", view)
	}
}

func assertNameFieldLooksLikeInput(t *testing.T, view string) {
	t.Helper()
	labelIdx := strings.Index(view, "Project name:")
	if labelIdx < 0 {
		t.Fatal("missing Project name label")
	}
	// Label line should not include the draft value on the same "Project name: value" style.
	lineEnd := strings.Index(view[labelIdx:], "\n")
	if lineEnd < 0 {
		t.Fatal("missing newline after Project name label")
	}
	labelLine := view[labelIdx : labelIdx+lineEnd]
	if strings.Contains(labelLine, "demo") {
		t.Fatalf("label should be separate from value: %q", labelLine)
	}
	if !strings.Contains(view, "╭") && !strings.Contains(view, "┌") {
		t.Fatalf("expected input border:\n%s", view)
	}
}

func assertActionAfterPlanPreview(t *testing.T, view string) {
	t.Helper()
	planIdx := strings.Index(view, "Plan Preview")
	noticeIdx := strings.Index(view, "No files")
	actionIdx := strings.Index(view, "Action")
	if planIdx < 0 || actionIdx < 0 {
		t.Fatalf("missing Plan Preview/Action markers")
	}
	if actionIdx < planIdx {
		t.Fatalf("Action before Plan Preview")
	}
	if noticeIdx >= 0 && !(planIdx < noticeIdx && noticeIdx < actionIdx) {
		t.Fatalf("no-mutation notice should sit between Plan Preview and Action")
	}
}

func assertNextLooksLikeButton(t *testing.T, view string) {
	t.Helper()
	if !strings.Contains(view, "Next") {
		t.Fatal("missing Next")
	}
	// Rounded border button chrome from Lip Gloss.
	if !strings.Contains(view, "╭") && !strings.Contains(view, "┌") {
		t.Fatalf("expected button border:\n%s", view)
	}
}

func TestStatusGitTechLibraries(t *testing.T) {
	t.Parallel()

	view := screens.Status(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:           true,
			CurrentBranch:    "feature/x",
			DefaultRemote:    "origin",
			DefaultRemoteURL: "https://example.com/demo.git",
			DefaultBranch:    "main",
		},
		Technologies: []workspace.Technology{
			{Name: "Go", Source: "go.mod", Confidence: "high"},
			{Name: "Go modules", Source: "go.mod", Confidence: "high"},
		},
		Libraries: []workspace.Library{
			{Name: "Bubble Tea", Module: "github.com/charmbracelet/bubbletea"},
			{Name: "Lip Gloss", Module: "github.com/charmbracelet/lipgloss"},
			{Name: "Bubbles", Module: "github.com/charmbracelet/bubbles"},
		},
		Atlas: workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
	})
	for _, want := range []string{
		"Default remote: origin",
		"Default branch: main",
		"Technologies",
		"Libraries",
		"Bubble Tea",
		"Lip Gloss",
		"Bubbles",
		"Go modules",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}
