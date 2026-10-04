package screens_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
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
}

func TestInitPlanStep1NoPlanPreview(t *testing.T) {
	t.Parallel()

	view := screens.InitPlan(screens.InitView{
		RootPath:        "/tmp/demo",
		DetectedName:    "demo",
		DetectedMode:    "existing",
		RecommendedMode: "existing",
		DraftName:       "demo",
		ModeConfirmed:   "existing",
		Decision:        "initialize",
		Artifacts:       []string{"AGENTS.md"},
		ActiveField:     screens.InitFieldNext,
		ContentFocused:  true,
	})

	for _, want := range []string{
		"Step 1 — Project Setup",
		"Project Identity",
		"Project Mode",
		"Runtime Artifacts",
		"No files will be changed in this slice.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Plan Preview") {
		t.Fatal("step 1 must not contain Plan Preview")
	}
}

func TestConfigFormFinalSections(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "Atlas-CLI",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	})
	view := screens.RenderConfigForm(screens.ConfigFormView{
		Title:          "Initial Configuration",
		Subtitle:       "Step 2 — Initial Configuration",
		Draft:          draft,
		SectionIndex:   0,
		FieldIndex:     0,
		PanelFocus:     screens.ConfigPanelFields,
		ContentFocused: true,
		ShowBack:       true,
		ShowNext:       true,
		FooterNote:     "No files will be changed in this slice.",
		Width:          110,
	})

	for _, want := range []string{
		"Step 2 — Initial Configuration",
		"Project: Atlas-CLI · Existing project",
		"Sections",
		"Governance",
		"Adapters",
		"Source Control",
		"Memory",
		"Workflow",
		"[x] SDD",
		"Spec engine",
		"[x] OpenSpec",
		"[ ] None",
		"[x] Testing required",
		"[x] Review required",
		"[x] Evidence required",
		"No files will be changed in this slice.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, bannedVisual := range []string{"[ Yes ]", "[ No ]", "[ OpenSpec ]", "[ None ]"} {
		if strings.Contains(view, bannedVisual) {
			t.Fatalf("horizontal chips must not render %q:\n%s", bannedVisual, view)
		}
	}
	footer := screens.RenderActionFooter(screens.ActionFooterView{
		ShowBack: true,
		ShowNext: true,
		Width:    110,
	})
	backIdx := strings.Index(footer, "[ Back ]")
	nextIdx := strings.Index(footer, "[ Next ]")
	if backIdx < 0 || nextIdx < 0 || backIdx >= nextIdx {
		t.Fatalf("footer must render Back before Next:\n%s", footer)
	}
	if strings.Count(footer, "\n") > 0 {
		t.Fatalf("compact footer should be one line:\n%s", footer)
	}
	if strings.Contains(footer, "Action") {
		t.Fatalf("footer must not include Action heading:\n%s", footer)
	}
	for _, banned := range []string{
		"Project Stack",
		"Runtime entrypoint",
		"Skills / Registry",
		"Tech / Libraries",
		"Governed basic",
		"Review only",
		"ODD",
		"Generic AGENTS.md",
		"Custom",
		"Evidence retention",
		"Governance storage",
		"governance.enabled",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("Action heading must be removed:\n%s", view)
		}
	}

	// Governance must not show Atlas governance files / Local only storage.
	gov := screens.RenderConfigForm(screens.ConfigFormView{
		Draft:        draft,
		SectionIndex: sectionIndex(draft, "governance"),
		PanelFocus:   screens.ConfigPanelFields,
		ShowBack:     true,
		ShowNext:     true,
		Width:        110,
	})
	if strings.Contains(gov, "Atlas governance files") || strings.Contains(gov, "Local only") {
		t.Fatalf("governance storage must not appear in Governance:\n%s", gov)
	}

	adapters := screens.RenderConfigForm(screens.ConfigFormView{
		Draft:        draft,
		SectionIndex: sectionIndex(draft, "adapters"),
		PanelFocus:   screens.ConfigPanelFields,
		ShowBack:     true,
		ShowNext:     true,
		Width:        100,
	})
	for _, want := range []string{"[ ] Cursor", "[ ] OpenCode"} {
		if !strings.Contains(adapters, want) {
			t.Fatalf("adapters missing %q:\n%s", want, adapters)
		}
	}

	sc := screens.RenderConfigForm(screens.ConfigFormView{
		Draft:        draft,
		SectionIndex: sectionIndex(draft, "source_control"),
		PanelFocus:   screens.ConfigPanelFields,
		ShowBack:     true,
		ShowNext:     true,
		Width:        110,
	})
	for _, want := range []string{
		"[x] None", "[ ] Git local", "[ ] Git + GitHub", "origin",
		"[x] Manual", "[ ] Simple", "[ ] Main + develop", "[ ] Main + develop + staging",
		"Atlas governance files", "[x] Local only", "[ ] Versioned",
		"[ ] Delivery assist",
	} {
		if !strings.Contains(sc, want) {
			t.Fatalf("source control missing %q:\n%s", want, sc)
		}
	}

	mem := screens.RenderConfigForm(screens.ConfigFormView{
		Draft:        draft,
		SectionIndex: sectionIndex(draft, "memory"),
		PanelFocus:   screens.ConfigPanelFields,
		ShowBack:     true,
		ShowNext:     true,
		Width:        110,
	})
	for _, want := range []string{"[ ] SQLite", "[ ] Context Capsule", "[x] SQLite + Context Capsule"} {
		if !strings.Contains(mem, want) {
			t.Fatalf("memory missing %q:\n%s", want, mem)
		}
	}
	for _, banned := range []string{"Memory enabled", "Fixed MVP", "cannot be disabled"} {
		if strings.Contains(mem, banned) {
			t.Fatalf("memory unexpected %q:\n%s", banned, mem)
		}
	}
}

func TestConfigureViewFinalSections(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: config.ModeExisting,
	})
	view := screens.ConfigureView(screens.ConfigFormView{
		Draft: draft,
		Width: 96,
	})
	for _, want := range []string{
		"Configure",
		"Governance",
		"Adapters",
		"Source Control",
		"Memory",
		"Apply Configuration Changes is not implemented",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{"[ Apply", "[ Next", "[ Close ]", "Project Stack", "Runtime entrypoint", "Skills / Registry"} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}
	footer := screens.RenderActionFooter(screens.ActionFooterView{
		ShowBack:  true,
		BackLabel: "Close",
		Width:     96,
	})
	if !strings.Contains(footer, "[ Close ]") || strings.Contains(footer, "[ Next ]") {
		t.Fatalf("configure footer should be Close only:\n%s", footer)
	}
}

func TestRenderMCP(t *testing.T) {
	t.Parallel()

	preview := screens.RenderMCP(screens.MCPView{
		Mode:  screens.MCPModeList,
		Draft: config.EmptyMCPDraft(),
	})
	for _, want := range []string{
		"MCP",
		"Configure external MCP integrations for Atlas.",
		"No files will be changed in this slice.",
		"Atlas is not initialized yet.",
		"preview only",
		"No MCP integrations configured yet.",
	} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview missing %q:\n%s", want, preview)
		}
	}
	for _, banned := range []string{"[x] Jira", "[ ] Jira", "[x] Context7", "[ ] Context7", "Custom MCP"} {
		if strings.Contains(preview, banned) {
			t.Fatalf("empty MCP must not show default row %q:\n%s", banned, preview)
		}
	}

	add := screens.RenderMCP(screens.MCPView{
		Mode:         screens.MCPModeAdd,
		KindFocus:    2,
		KindSelected: config.MCPKindCustom,
		AddFocus:     screens.MCPFocusName,
	})
	for _, want := range []string{"Add MCP", "Name", "Kind", "Connection", "[ ] Jira", "[ ] Context7", "[x] Custom"} {
		if !strings.Contains(add, want) {
			t.Fatalf("add form missing %q:\n%s", want, add)
		}
	}

	draft := config.EmptyMCPDraft()
	if _, err := draft.AddServer("My Jira", config.MCPKindJira, ""); err != nil {
		t.Fatal(err)
	}
	draft.ToggleEnabled(0)
	active := screens.RenderMCP(screens.MCPView{
		Mode:           screens.MCPModeList,
		Draft:          draft,
		ActiveIndex:    0,
		Initialized:    true,
		ContentFocused: true,
		ListFocus:      screens.MCPFocusServers,
	})
	for _, want := range []string{
		"Configured integrations",
		"My Jira",
		"Jira",
		"not configured",
		"[x]",
		"in memory only",
	} {
		if !strings.Contains(active, want) {
			t.Fatalf("initialized view missing %q:\n%s", want, active)
		}
	}
}

func sectionIndex(draft config.ConfigDraft, key string) int {
	for i, section := range draft.SelectorSections() {
		if section.Key == key {
			return i
		}
	}
	return 0
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
		},
		Libraries: []workspace.Library{
			{Name: "Bubble Tea", Module: "github.com/charmbracelet/bubbletea"},
		},
		Atlas: workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
	})
	for _, want := range []string{"Default remote: origin", "Technologies", "Bubble Tea"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}
