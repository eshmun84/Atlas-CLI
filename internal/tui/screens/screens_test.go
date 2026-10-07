package screens_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
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
	if !strings.Contains(screens.Doctor(doctor.Report{}, workspace.DiscoveryResult{}), "Atlas Doctor") {
		t.Fatal("doctor")
	}
	if !strings.Contains(screens.RenderRuntimeRepair(screens.RepairView{
		Plan: workspace.RuntimeRepairPlan{Healthy: true},
	}), "Runtime Repair") {
		t.Fatal("repair")
	}
}

func TestInitPlanStep1NoPlanPreview(t *testing.T) {
	t.Parallel()

	view := screens.InitPlan(screens.InitView{
		RootPath:       "/tmp/demo",
		DetectedName:   "demo",
		DetectedMode:   "existing",
		AtlasState:     "Not initialized",
		DraftName:      "demo",
		ModeConfirmed:  "existing",
		ActiveField:    screens.InitFieldNext,
		ContentFocused: true,
	})

	for _, want := range []string{
		"Step 1 — Project Setup",
		"Project Name",
		"Project Mode",
		"[x] Existing Project",
		"[ ] New Project",
		"No files are written until Review → Apply.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{
		"Project Identity",
		"Project Detection",
		"Project name:",
		"Runtime artifact conflict gate",
		"Accept backup/quarantine and continue Init",
		"Recommended mode",
		"Plan Preview",
		"✓ New Project",
		"(x) Existing Project",
		"( ) New Project",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}
}

func TestInitConflictPreflight(t *testing.T) {
	t.Parallel()

	view := screens.InitConflict(screens.InitConflictView{
		RootPath:       "/tmp/demo",
		Artifacts:      []string{"AGENTS.md", ".cursor"},
		ActiveField:    screens.InitFieldConflictRefresh,
		ContentFocused: true,
	})
	for _, want := range []string{
		"Runtime conflict",
		"Blocking warning",
		"Detected conflicts",
		"AGENTS.md",
		".cursor",
		"Manual cleanup steps",
		"1. Review the detected files/directories.",
		"2. Move them outside the project or back them up manually.",
		"3. Use Refresh / Re-check after the project is clean.",
		"Refresh / Re-check",
		"Exit / Back",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{
		"Project Mode",
		"Accept backup/quarantine",
		"Backup / quarantine acceptance",
		"Continue to Setup",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}

	clean := screens.InitConflict(screens.InitConflictView{
		RootPath:       "/tmp/demo",
		Artifacts:      nil,
		ActiveField:    screens.InitFieldConflictContinue,
		ContentFocused: true,
	})
	for _, want := range []string{
		"No runtime conflicts detected.",
		"Continue to Setup",
	} {
		if !strings.Contains(clean, want) {
			t.Fatalf("clean missing %q:\n%s", want, clean)
		}
	}
	if strings.Contains(clean, "Blocking warning") || strings.Contains(clean, "AGENTS.md") {
		t.Fatalf("clean state still shows blocking content:\n%s", clean)
	}
}

func TestRenderReview(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "Atlas-CLI",
		ProjectMode:         "new",
		ToolCursorAvailable: true,
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		t.Fatal("expected cursor select")
	}
	plan := initplan.BuildReview(initplan.ReviewInput{Draft: draft, MCP: config.EmptyMCPDraft()})
	view := screens.RenderReview(screens.ReviewView{Plan: plan, ContentFocused: true})
	for _, want := range []string{
		"Step 3 — Review / Materialization Plan",
		"Name: Atlas-CLI",
		"Mode: New project",
		".atlas/config.yaml",
		"AGENTS.md",
		".cursor/rules/atlas.mdc",
		"create/update on Apply",
		"No conflicting runtime surfaces detected.",
		"No Atlas-managed backups required",
		"Platform:",
		"Governance files:",
		"Assisted operations:",
		"Init performs no Git operations.",
		"No repository, branch, commit, push, pull request, merge or remote operation",
		"Runtime conflicts block Init and require manual cleanup",
		"Atlas Context Graph is not available in this slice.",
		"[content focus]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{
		"Memory:",
		"Context Economy:",
		"CodeGraph",
		"Development & Delivery",
		"planned for later",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
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
		FooterNote:     "No files are written until Review → Apply.",
		Width:          110,
	})

	for _, want := range []string{
		"Step 2 — Initial Configuration",
		"Project: Atlas-CLI · Existing project",
		"Sections",
		"Governance",
		"Adapters",
		"Delivery",
		"MCP",
		"Workflow",
		"[x] SDD",
		"Spec engine",
		"[x] OpenSpec",
		"[ ] None",
		"[x] Testing required",
		"[x] Review required",
		"[x] Evidence required",
		"No files are written until Review → Apply.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{"Memory", "Development &", "CodeGraph"} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}
	for _, section := range draft.SelectorSections() {
		if section.Key == "context" || section.Title == "Context" {
			t.Fatal("Init form must not expose Context section")
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

	adaptersDraft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:           "Atlas-CLI",
		ProjectMode:           "existing",
		ToolCursorAvailable:   true,
		ToolOpenCodeAvailable: false,
	})
	adapters := screens.RenderConfigForm(screens.ConfigFormView{
		Draft:        adaptersDraft,
		SectionIndex: sectionIndex(adaptersDraft, "adapters"),
		PanelFocus:   screens.ConfigPanelFields,
		ShowBack:     true,
		ShowNext:     true,
		Width:        100,
	})
	for _, want := range []string{"[ ] Cursor      Available", "[ ] OpenCode      Not available"} {
		if !strings.Contains(adapters, want) {
			t.Fatalf("adapters missing %q:\n%s", want, adapters)
		}
	}
	for _, banned := range []string{"Claude", "Codex", "not supported", "Select supported"} {
		if strings.Contains(adapters, banned) {
			t.Fatalf("adapters unexpected %q:\n%s", banned, adapters)
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
		"Delivery",
		"Platform",
		"[x] None", "[ ] Git local", "[ ] GitHub",
		"Governance files", "[x] Local only", "[ ] Versioned",
		"Assisted operations", "[ ] Enabled",
	} {
		if !strings.Contains(sc, want) {
			t.Fatalf("delivery missing %q:\n%s", want, sc)
		}
	}
	for _, banned := range []string{
		"Tools", "Policy", "Suggestions only.", "git ok", "gh ok", "OpenSpec", "CodeGraph",
		"Branch strategy", "Default remote", "GitFlow", "Development &", "Assistance",
	} {
		if strings.Contains(sc, banned) {
			t.Fatalf("delivery unexpected %q:\n%s", banned, sc)
		}
	}

	if sectionIndex(draft, "memory") >= 0 {
		t.Fatal("memory must not appear in Init selector sections")
	}
	if sectionIndex(draft, "context") >= 0 {
		t.Fatal("context must not appear in Init selector sections")
	}

	mcp := screens.RenderConfigForm(screens.ConfigFormView{
		Draft:          draft,
		SectionIndex:   sectionIndex(draft, "mcp"),
		PanelFocus:     screens.ConfigPanelFields,
		ContentFocused: true,
		ShowBack:       true,
		ShowNext:       true,
		Width:          110,
		MCP: screens.MCPView{
			Mode:           screens.MCPModeList,
			Draft:          config.DefaultMCPDraft(),
			ListFocus:      screens.MCPFocusBuiltins,
			ContentFocused: true,
		},
	})
	for _, want := range []string{"Built-in MCPs", "[ ] Jira", "[ ] Context7", "[ ] Chrome DevTools", "Custom MCPs", "[ Add MCP ]"} {
		if !strings.Contains(mcp, want) {
			t.Fatalf("init mcp section missing %q:\n%s", want, mcp)
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
		"Delivery",
		"MCP",
		"Close discards unsaved changes. Apply changes writes .atlas/config.yaml.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{"[ Next ]", "[ Close ]", "Project Stack", "Runtime entrypoint", "Skills / Registry", "Memory", "Development &", "CodeGraph"} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}
	for _, section := range draft.SelectorSections() {
		if section.Key == "context" {
			t.Fatal("Configure must not expose Context section")
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
		Draft: config.DefaultMCPDraft(),
	})
	for _, want := range []string{
		"MCP",
		"MCP selections record preferences only.",
		"NOT IMPLEMENTED",
		"Atlas is not initialized yet.",
		"draft-only until Init Apply",
		"Built-in MCPs",
		"[ ] Jira",
		"[ ] Context7",
		"[ ] Chrome DevTools",
		"Custom MCPs",
		"No custom MCPs configured yet.",
	} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview missing %q:\n%s", want, preview)
		}
	}
	if strings.Contains(preview, "[ ] Custom") || strings.Contains(preview, "[x] Custom") {
		t.Fatalf("custom must not be a built-in row:\n%s", preview)
	}

	add := screens.RenderMCP(screens.MCPView{
		Mode:              screens.MCPModeAdd,
		TransportFocus:    0,
		TransportSelected: config.MCPTransportStdio,
		AddFocus:          screens.MCPFocusName,
	})
	for _, want := range []string{"Add MCP", "Name", "Transport", "[x] stdio", "[ ] http", "[ ] sse", "Command or URL", "Arguments", "Environment references"} {
		if !strings.Contains(add, want) {
			t.Fatalf("add form missing %q:\n%s", want, add)
		}
	}
	for _, banned := range []string{"Kind", "[ ] Jira", "[ ] Context7", "[ ] Chrome DevTools", "[x] Custom"} {
		if strings.Contains(add, banned) {
			t.Fatalf("add form unexpected %q:\n%s", banned, add)
		}
	}

	draft := config.DefaultMCPDraft()
	draft.ToggleBuiltin(0)
	if _, err := draft.AddCustom("My Browser MCP", config.MCPTransportStdio, "", "", ""); err != nil {
		t.Fatal(err)
	}
	draft.ToggleCustom(0)
	active := screens.RenderMCP(screens.MCPView{
		Mode:           screens.MCPModeList,
		Draft:          draft,
		ActiveIndex:    0,
		Initialized:    true,
		ContentFocused: true,
		ListFocus:      screens.MCPFocusCustom,
	})
	for _, want := range []string{
		"[x] Jira",
		"My Browser MCP",
		"stdio",
		"[x]",
		"preference recorded",
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
	return -1
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
		Runtime: workspace.RuntimeHealth{
			SelectedAdapters: []string{"cursor"},
			ExpectedProjections: []workspace.ProjectionStatus{
				{Adapter: "cursor", Path: ".cursor/rules/atlas.mdc", Present: true},
			},
			ContextGraphReadable: true,
			ContextGraphEnabled:  true,
			AgentsMarkers: config.AgentsMarkers{
				BaseBegin: true, BaseEnd: true, UserBegin: true, UserEnd: true,
				AdapterBlocks: map[string]bool{"cursor": true},
				FoundAdapters: []string{"cursor"},
			},
			AgentsExists:        true,
			RuntimeMaterialized: true,
			Initialized:         true,
			ConfigExists:        true,
			ConfigLoads:         true,
			StateExists:         true,
			StateLoads:          true,
			BackupsDirExists:    true,
		},
	})
	for _, want := range []string{
		"Default remote: origin",
		"Remote default branch: main",
		"Project Technology",
		"Bubble Tea",
		"Atlas Runtime",
		"materialized",
		"Adapters",
		"Source Control / Delivery Tools",
		"Health",
		"MCP / External Context",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Remote URL: unknown") {
		t.Fatalf("empty remote must not display as unknown:\n%s", view)
	}
	if !strings.Contains(view, "Remote URL:") || !strings.Contains(view, "https://example.com/demo.git") {
		t.Fatalf("expected remote URL in status:\n%s", view)
	}
}

func TestStatusRemoteDefaultBranchUnknownLocally(t *testing.T) {
	t.Parallel()

	view := stripANSI(screens.Status(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:           true,
			CurrentBranch:    "feature/x",
			DefaultRemote:    "origin",
			DefaultRemoteURL: "https://example.com/demo.git",
			// DefaultBranch empty: origin/main|develop|staging exist but origin/HEAD does not.
			Remotes: []workspace.GitRemote{{Name: "origin", URL: "https://example.com/demo.git"}},
		},
		Atlas: workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
	}))
	if !strings.Contains(view, "Remote default branch: unknown locally") {
		t.Fatalf("expected unknown locally, got:\n%s", view)
	}
	if strings.Contains(view, "Remote default branch: none") {
		t.Fatalf("must not show none when default remote exists:\n%s", view)
	}
	if strings.Contains(view, "Default branch:") {
		t.Fatalf("old Default branch label must be renamed:\n%s", view)
	}
}

func TestStatusRemoteDefaultBranchResolved(t *testing.T) {
	t.Parallel()

	view := stripANSI(screens.Status(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "feature/x",
			DefaultRemote: "origin",
			DefaultBranch: "main",
		},
		Atlas: workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
	}))
	if !strings.Contains(view, "Remote default branch: main") {
		t.Fatalf("expected main, got:\n%s", view)
	}
}

func TestStatusEmptyRemoteIsNone(t *testing.T) {
	t.Parallel()

	view := stripANSI(screens.Status(workspace.DiscoveryResult{
		RootPath: "/tmp/demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "main",
		},
		Atlas: workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
	}))
	for _, want := range []string{
		"Default remote:",
		"Remote URL:",
		"Remote default branch:",
		"Remotes:",
		"none",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "Remote default branch: none") {
		t.Fatalf("expected Remote default branch: none:\n%s", view)
	}
	if strings.Contains(view, "Default remote: unknown") ||
		strings.Contains(view, "Remote URL: unknown") ||
		strings.Contains(view, "Remote default branch: unknown locally") {
		t.Fatalf("empty git remote fields must display as none:\n%s", view)
	}
}

func TestStatusHealthMatchesDoctorReport(t *testing.T) {
	t.Parallel()

	result := workspace.DiscoveryResult{
		RootPath: "/tmp/existing-demo",
		Git: workspace.GitInfo{
			IsRepo:        true,
			CurrentBranch: "main",
		},
		Files: workspace.FileInfo{HasReadme: true},
		Atlas: workspace.AtlasStatus{State: workspace.AtlasStateNotInitialized},
		Runtime: workspace.RuntimeHealth{
			ConfigExists: false,
			Home: home.Status{
				Path:           "/tmp/atlas-home-incomplete",
				Exists:         true,
				Writable:       true,
				LayoutComplete: false,
				MissingAssets:  []string{"agents/base.md", "contracts/sdd-openspec.md"},
			},
		},
		Tools: []workspace.ToolInfo{
			{Name: "git", Available: true},
			{Name: "openspec", Available: false},
		},
	}
	report := doctor.Evaluate(result)
	pass, warn, fail := report.Counts()
	if warn == 0 {
		t.Fatalf("fixture must produce doctor warnings; report=%#v", report.Checks)
	}

	status := stripANSI(screens.StatusWithReport(result, report))
	docView := stripANSI(screens.Doctor(report, result))

	statusPass, statusWarn, statusFail := parseHealthCounts(t, status)
	docPass, docWarn, docFail := parseHealthCounts(t, docView)
	if statusPass != pass || statusWarn != warn || statusFail != fail {
		t.Fatalf("status counts=%d/%d/%d want doctor %d/%d/%d\n%s", statusPass, statusWarn, statusFail, pass, warn, fail, status)
	}
	if docPass != pass || docWarn != warn || docFail != fail {
		t.Fatalf("doctor view counts=%d/%d/%d want %d/%d/%d\n%s", docPass, docWarn, docFail, pass, warn, fail, docView)
	}
	if statusWarn == 0 {
		t.Fatalf("status must not show WARNING 0 when doctor has warnings:\n%s", status)
	}
	if !strings.Contains(status, report.ResultLabel()) {
		t.Fatalf("status missing result label %q:\n%s", report.ResultLabel(), status)
	}
	if report.ResultLabel() != "ready with warnings" {
		t.Fatalf("result label = %q, want ready with warnings", report.ResultLabel())
	}
	if !strings.Contains(status, "Needs attention:") {
		t.Fatalf("status missing compact Needs attention list:\n%s", status)
	}
	// Status stays executive: no full Doctor section dump.
	if strings.Contains(status, "Overall Health") {
		t.Fatalf("status must not duplicate Doctor Overall Health section:\n%s", status)
	}
}

func TestStatusHealthMatchesDoctorLiveWorkspace(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir()) // missing/incomplete home; never touch ~/.atlas

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)

	result, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(result)
	_, warn, _ := report.Counts()
	if warn == 0 {
		t.Fatalf("uninitialized existing project should warn; checks=%#v", report.Checks)
	}

	status := stripANSI(screens.Status(result))
	docView := stripANSI(screens.Doctor(report, result))
	statusPass, statusWarn, statusFail := parseHealthCounts(t, status)
	docPass, docWarn, docFail := parseHealthCounts(t, docView)
	if statusPass != docPass || statusWarn != docWarn || statusFail != docFail {
		t.Fatalf("status=%d/%d/%d doctor=%d/%d/%d", statusPass, statusWarn, statusFail, docPass, docWarn, docFail)
	}
	if statusWarn == 0 {
		t.Fatal("status WARNING count must mirror doctor warnings")
	}
	if !strings.Contains(status, "ready with warnings") {
		t.Fatalf("status missing ready with warnings:\n%s", status)
	}
	assertTreeUnchanged(t, root, before)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func parseHealthCounts(t *testing.T, view string) (pass, warn, fail int) {
	t.Helper()
	// Status: "PASS: N   WARNING: N   ERROR: N"
	// Doctor: "PASS: N" / "WARNING: N" / "ERROR: N" on separate lines.
	re := regexp.MustCompile(`PASS:\s*(\d+)[\s\S]*?WARNING:\s*(\d+)[\s\S]*?ERROR:\s*(\d+)`)
	m := re.FindStringSubmatch(view)
	if len(m) != 4 {
		t.Fatalf("could not parse health counts from view:\n%s", view)
	}
	pass, _ = strconv.Atoi(m[1])
	warn, _ = strconv.Atoi(m[2])
	fail, _ = strconv.Atoi(m[3])
	return pass, warn, fail
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[rel+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertTreeUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotTree(t, root)
	if len(before) != len(after) {
		t.Fatalf("tree size changed: before=%d after=%d", len(before), len(after))
	}
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("mutated %s", path)
		}
	}
}
