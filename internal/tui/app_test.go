package tui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestDefaultModelRoute(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.DefaultRoute})
	if m.Route() != tui.RouteDashboard {
		t.Fatalf("route = %v, want dashboard", m.Route())
	}
	if got := sidebarLabel(m, m.SidebarIndex()); got != "Dashboard" {
		t.Fatalf("sidebar selected = %q, want Dashboard", got)
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.DefaultRoute})
	m = mustModel(m.Update(tea.WindowSizeMsg{Width: 120, Height: 40}))
	if m.Width() != 120 || m.Height() != 40 {
		t.Fatalf("size = %dx%d, want 120x40", m.Width(), m.Height())
	}
}

func TestSidebarNavigationAndEnter(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{Route: tui.DefaultRoute}))
	start := m.SidebarIndex()

	m = mustModel(m.Update(key("down")))
	if m.SidebarIndex() != start+1 {
		t.Fatalf("down index = %d, want %d", m.SidebarIndex(), start+1)
	}

	m = mustModel(m.Update(key("up")))
	if m.SidebarIndex() != start {
		t.Fatalf("up index = %d, want %d", m.SidebarIndex(), start)
	}

	m = sized(tui.NewModel(tui.Options{Route: tui.DefaultRoute}))
	for sidebarLabel(m, m.SidebarIndex()) != "Doctor" {
		m = mustModel(m.Update(key("j")))
	}
	m, cmd := apply(m, key("enter"))
	m = applyCmd(t, m, cmd)
	if m.Route() != tui.RouteDoctor {
		t.Fatalf("enter route = %v, want doctor", m.Route())
	}
}

func TestSidebarEnterExitQuits(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{Route: tui.DefaultRoute}))
	for sidebarLabel(m, m.SidebarIndex()) != "Exit" {
		m = mustModel(m.Update(key("down")))
	}
	updated, cmd := m.Update(key("enter"))
	model := updated.(tui.Model)
	if !model.Quitting() {
		t.Fatal("enter on Exit should quit")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestSidebarInitVsConfigure(t *testing.T) {
	root := t.TempDir()
	uninit := loadWorkspace(t, tui.Options{
		Route: tui.RouteDashboard, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	labels := sidebarLabels(uninit)
	if !contains(labels, "Init / Setup") || contains(labels, "Configure") || !contains(labels, "MCP") {
		t.Fatalf("uninitialized sidebar = %v", labels)
	}
	if idx := indexOf(labels, "MCP"); idx < 0 || idx >= indexOf(labels, "Status") {
		t.Fatalf("MCP should appear before Status: %v", labels)
	}

	writeValidAtlasConfig(t, root)
	initd := loadWorkspace(t, tui.Options{
		Route: tui.RouteDashboard, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	labels = sidebarLabels(initd)
	if !contains(labels, "Configure") || contains(labels, "Init / Setup") || !contains(labels, "MCP") {
		t.Fatalf("initialized sidebar = %v", labels)
	}
	if idx := indexOf(labels, "MCP"); idx < 0 || idx <= indexOf(labels, "Configure") || idx >= indexOf(labels, "Status") {
		t.Fatalf("MCP should sit after Configure and before Status: %v", labels)
	}

	cfg := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	view := cfg.View()
	for _, want := range []string{
		"Configure",
		"Sections",
		"Governance",
		"Adapters",
		"Source Control",
		"Memory",
		"Project:",
		"[ Close ]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("configure missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{"[ Apply", "[ Next", "Runtime entrypoint", "Skills / Registry", "Project Stack"} {
		if strings.Contains(view, banned) {
			t.Fatalf("configure unexpected %q:\n%s", banned, view)
		}
	}
	if _, ok := cfg.ConfigDraft().FieldByKey("runtime.entrypoint"); ok {
		t.Fatal("runtime fields must not exist in draft")
	}
	// Sidebar should show Configure, not Init / Setup.
	if contains(sidebarLabels(cfg), "Init / Setup") || !contains(sidebarLabels(cfg), "Configure") {
		t.Fatalf("sidebar = %v", sidebarLabels(cfg))
	}
}

func TestMCPScreenInMemory(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteMCP, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	view := m.View()
	for _, want := range []string{
		"MCP",
		"No MCP integrations configured yet.",
		"No files will be changed in this slice.",
		"preview only",
		"[ Add MCP ]",
		"[ Close ]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("mcp missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{"[x] Jira", "[ ] Jira", "[ ] Context7", "Custom MCP"} {
		if strings.Contains(view, banned) {
			t.Fatalf("mcp must not pre-create %q:\n%s", banned, view)
		}
	}
	assertShell(t, view, "MCP")
	assertActionNearFooter(t, view)
	assertGlobalTopGap(t, view)

	m = mustModel(m.Update(key("tab")))
	m = mustModel(m.Update(key("enter"))) // Add MCP
	if m.MCPMode() != screens.MCPModeAdd {
		t.Fatalf("mode = %q, want add", m.MCPMode())
	}
	for _, want := range []string{"Add MCP", "Name", "Kind", "Connection", "Jira", "Context7", "Custom", "[ Cancel ]", "[ Add ]"} {
		if !strings.Contains(m.View(), want) {
			t.Fatalf("add form missing %q:\n%s", want, m.View())
		}
	}

	// Empty name rejected.
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if m.MCPMode() != screens.MCPModeAdd || m.MCPAddError() == "" {
		t.Fatalf("expected empty-name validation, mode=%q err=%q", m.MCPMode(), m.MCPAddError())
	}

	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("My Jira")}))
	m = mustModel(m.Update(key("down"))) // kind
	m = mustModel(m.Update(key("up")))
	m = mustModel(m.Update(key("up"))) // Jira
	m = mustModel(m.Update(key("enter")))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter"))) // Add
	if m.MCPMode() != screens.MCPModeList || len(m.MCPDraft().Servers) != 1 {
		t.Fatalf("after add mode=%q servers=%#v", m.MCPMode(), m.MCPDraft().Servers)
	}
	if m.MCPDraft().Servers[0].Name != "My Jira" || m.MCPDraft().Servers[0].Kind != "jira" {
		t.Fatalf("server = %#v", m.MCPDraft().Servers[0])
	}

	// Duplicate rejected then cancel.
	m = mustModel(m.Update(key("down"))) // Add MCP
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("My Jira")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if !strings.Contains(m.MCPAddError(), "already exists") {
		t.Fatalf("expected duplicate error, got %q", m.MCPAddError())
	}
	m = mcpGotoFooterAction(t, m, false)
	m = mustModel(m.Update(key("enter"))) // Cancel
	if m.MCPMode() != screens.MCPModeList || len(m.MCPDraft().Servers) != 1 {
		t.Fatalf("cancel failed mode=%q servers=%d", m.MCPMode(), len(m.MCPDraft().Servers))
	}

	if m.MCPListFocus() != screens.MCPFocusServers {
		m = mustModel(m.Update(key("up")))
	}
	m = mustModel(m.Update(key("enter")))
	if !m.MCPDraft().Servers[0].Enabled {
		t.Fatal("toggle enable failed")
	}
	m = mustModel(m.Update(key(" ")))
	if m.MCPDraft().Servers[0].Enabled {
		t.Fatal("toggle disable failed")
	}
	assertNoMutation(t, root)
}

func mcpGotoFooterAction(t *testing.T, m tui.Model, submit bool) tui.Model {
	t.Helper()
	for i := 0; i < 12; i++ {
		m = mustModel(m.Update(key("down")))
	}
	if submit {
		m = mustModel(m.Update(key("right")))
	} else {
		m = mustModel(m.Update(key("left")))
	}
	return m
}

func TestCompactConfigureFooter(t *testing.T) {
	root := t.TempDir()
	writeValidAtlasConfig(t, root)
	cfg := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	view := cfg.View()
	if !strings.Contains(view, "[ Close ]") {
		t.Fatalf("configure footer missing Close:\n%s", view)
	}
	if strings.Contains(view, "[ Next ]") || strings.Contains(view, "[ Back ]") {
		t.Fatalf("configure footer should be Close only:\n%s", view)
	}
	assertCompactFooter(t, view)
}

func assertCompactFooter(t *testing.T, view string) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("footer must not render Action heading:\n%s", view)
		}
	}
	assertActionNearFooter(t, view)
}

func assertActionNearFooter(t *testing.T, view string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	actionLine := -1
	footerLine := -1
	for i, line := range lines {
		if strings.Contains(line, "[ Close ]") || strings.Contains(line, "[ Next ]") || strings.Contains(line, "[ Add MCP ]") {
			actionLine = i
		}
		if strings.Contains(line, "q quit") || strings.Contains(line, "Tab focus") || strings.Contains(line, "Enter/q/Esc") {
			if footerLine < 0 || i > footerLine {
				footerLine = i
			}
		}
	}
	if actionLine < 0 {
		t.Fatalf("missing action row:\n%s", view)
	}
	if footerLine < 0 {
		t.Fatalf("missing global footer:\n%s", view)
	}
	if footerLine <= actionLine {
		t.Fatalf("footer should appear after action row:\n%s", view)
	}
	if strings.Count(view, "q quit") > 1 && strings.Count(view, "Tab focus") > 1 {
		t.Fatalf("footer appears duplicated:\n%s", view)
	}
	blanks := 0
	for _, line := range lines[actionLine+1 : footerLine] {
		trimmed := strings.TrimSpace(strings.Trim(line, "│"))
		if trimmed == "" {
			blanks++
		}
	}
	if blanks > 2 {
		t.Fatalf("large blank block between action row and footer (%d blank lines):\n%s", blanks, view)
	}
	if footerLine-actionLine > 4 {
		t.Fatalf("action row too far from footer (gap %d lines):\n%s", footerLine-actionLine, view)
	}
}

func TestContentFitKeepsActionNearFooter(t *testing.T) {
	root := t.TempDir()
	initM := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	initM = mustModel(initM.Update(tea.WindowSizeMsg{Width: 120, Height: 60}))
	assertActionNearFooter(t, initM.View())
	assertGlobalTopGap(t, initM.View())
	assertShell(t, initM.View(), "Init / Setup")
	if strings.Count(initM.View(), "q quit") != 1 {
		t.Fatalf("footer should appear once:\n%s", initM.View())
	}

	initM = mustModel(initM.Update(key("tab")))
	for initM.InitField() != screens.InitFieldNext {
		prev := initM.InitField()
		initM = mustModel(initM.Update(key("down")))
		if initM.InitField() == prev {
			t.Fatal("could not reach Next")
		}
	}
	initM = mustModel(initM.Update(key("enter")))
	assertActionNearFooter(t, initM.View())
	assertGlobalTopGap(t, initM.View())
	for _, line := range strings.Split(initM.View(), "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("step 2 Action heading:\n%s", initM.View())
		}
	}

	mcp := loadWorkspace(t, tui.Options{
		Route: tui.RouteMCP, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	mcp = mustModel(mcp.Update(tea.WindowSizeMsg{Width: 120, Height: 60}))
	assertActionNearFooter(t, mcp.View())
	assertGlobalTopGap(t, mcp.View())
	assertShell(t, mcp.View(), "MCP")

	writeValidAtlasConfig(t, root)
	cfg := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	cfg = mustModel(cfg.Update(tea.WindowSizeMsg{Width: 120, Height: 60}))
	assertActionNearFooter(t, cfg.View())
}

func TestTallTerminalDoesNotOverflow(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteHelp, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = mustModel(m.Update(tea.WindowSizeMsg{Width: 120, Height: 80}))
	view := m.View()
	if !strings.Contains(view, "Atlas Help") {
		t.Fatalf("help missing:\n%s", view)
	}
	if strings.Count(view, "\n")+1 > 80 {
		t.Fatalf("view height exceeds terminal 80")
	}
}

func TestDirectRouteSelectsSidebar(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteDoctor})
	if sidebarLabel(m, m.SidebarIndex()) != "Doctor" {
		t.Fatalf("sidebar = %q, want Doctor", sidebarLabel(m, m.SidebarIndex()))
	}
}

func TestHelpKeys(t *testing.T) {
	for _, k := range []string{"h", "?"} {
		m := sized(tui.NewModel(tui.Options{Route: tui.DefaultRoute}))
		m = mustModel(m.Update(key(k)))
		if m.Route() != tui.RouteHelp {
			t.Fatalf("%q route = %v, want help", k, m.Route())
		}
	}
}

func TestBReturnsDefaultRoute(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
	m, cmd := apply(m, key("b"))
	m = applyCmd(t, m, cmd)
	if m.Route() != tui.DefaultRoute {
		t.Fatalf("route = %v, want dashboard", m.Route())
	}
}

func TestQAndCtrlCQuit(t *testing.T) {
	for _, k := range []tea.KeyMsg{key("q"), {Type: tea.KeyCtrlC}} {
		m := sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
		updated, cmd := m.Update(k)
		model := updated.(tui.Model)
		if !model.Quitting() {
			t.Fatalf("%v should quit", k)
		}
		if cmd == nil {
			t.Fatal("expected quit cmd")
		}
	}
}

func TestEscQuitsFromDefault(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{Route: tui.DefaultRoute}))
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(tui.Model)
	if !model.Quitting() {
		t.Fatal("esc from dashboard should quit")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestEscReturnsToDefaultFromChild(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
	m, cmd := apply(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = applyCmd(t, m, cmd)
	if m.Route() != tui.DefaultRoute {
		t.Fatalf("esc route = %v, want dashboard", m.Route())
	}
	if m.Quitting() {
		t.Fatal("esc from help should not quit")
	}
}

func TestContentScrolling(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteDoctor,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	m = mustModel(m.Update(tea.WindowSizeMsg{Width: 100, Height: 24}))
	if m.ContentOffset() != 0 {
		t.Fatalf("initial offset = %d", m.ContentOffset())
	}

	m = mustModel(m.Update(key("pgdown")))
	if m.ContentOffset() <= 0 {
		t.Fatalf("pgdown offset = %d, want > 0", m.ContentOffset())
	}
	afterDown := m.ContentOffset()

	m = mustModel(m.Update(key("pgup")))
	if m.ContentOffset() >= afterDown {
		t.Fatalf("pgup offset = %d, want < %d", m.ContentOffset(), afterDown)
	}

	m = mustModel(m.Update(key("end")))
	endOff := m.ContentOffset()
	if endOff <= 0 {
		t.Fatalf("end offset = %d, want > 0", endOff)
	}

	m = mustModel(m.Update(key("home")))
	if m.ContentOffset() != 0 {
		t.Fatalf("home offset = %d, want 0", m.ContentOffset())
	}
}

func TestShellViews(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n\ngo 1.22\n\nrequire github.com/charmbracelet/bubbletea v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# agents"), 0o644); err != nil {
		t.Fatal(err)
	}

	dash := loadWorkspace(t, tui.Options{
		Route: tui.RouteDashboard, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, dash.View(), "Dashboard")
	assertGlobalTopGap(t, dash.View())
	if !strings.Contains(dash.View(), "Suggested next action") {
		t.Fatalf("dashboard missing next action:\n%s", dash.View())
	}
	assertNoMutation(t, root)

	initM := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, initM.View(), "Init / Setup")
	assertInitWizardView(t, initM.View())
	assertNoMutation(t, root)

	status := loadWorkspace(t, tui.Options{
		Route: tui.RouteStatus, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, status.View(), "Atlas Status")
	for _, want := range []string{"Technologies", "Libraries", "Bubble Tea"} {
		if !strings.Contains(status.View(), want) {
			t.Fatalf("status missing %q:\n%s", want, status.View())
		}
	}

	doc := loadWorkspace(t, tui.Options{
		Route: tui.RouteDoctor, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, doc.View(), "Atlas Doctor")

	help := sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
	assertShell(t, help.View(), "Atlas Help")
}

func TestInitFocusNameModeDecision(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# agents"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	if m.Focus() != tui.FocusSidebar {
		t.Fatalf("initial focus = %v", m.Focus())
	}
	if strings.Contains(m.View(), "Auto-detect") {
		t.Fatal("Auto-detect must not be selectable")
	}
	if m.InitModeConfirmed() != tui.InitModeExisting {
		t.Fatalf("recommended confirmed mode = %q, want existing", m.InitModeConfirmed())
	}
	baseName := m.DraftName()
	if baseName == "" {
		t.Fatal("expected detected folder name")
	}

	m = mustModel(m.Update(key("tab")))
	if m.Focus() != tui.FocusContent {
		t.Fatalf("focus = %v, want content", m.Focus())
	}
	if m.InitField() != screens.InitFieldName {
		t.Fatalf("active field = %d, want name", m.InitField())
	}

	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")}))
	if !strings.HasSuffix(m.DraftName(), "X") {
		t.Fatalf("draft name = %q, want suffix X", m.DraftName())
	}

	// Cursor movement: edit inside the string.
	m = mustModel(m.Update(key("left")))
	pos := m.NameCursor()
	if pos != len(m.DraftName())-1 {
		t.Fatalf("cursor = %d, want %d", pos, len(m.DraftName())-1)
	}
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")}))
	name := m.DraftName()
	if !strings.Contains(name, "Y") || !strings.HasSuffix(name, "X") {
		t.Fatalf("inline edit failed: %q", name)
	}

	m = mustModel(m.Update(key("down"))) // mode new
	m = mustModel(m.Update(key("enter")))
	if m.InitModeConfirmed() != tui.InitModeNew {
		t.Fatalf("mode = %q, want new", m.InitModeConfirmed())
	}

	m = mustModel(m.Update(key("down"))) // mode existing
	m = mustModel(m.Update(key("enter")))
	if m.InitModeConfirmed() != tui.InitModeExisting {
		t.Fatalf("mode = %q, want existing", m.InitModeConfirmed())
	}

	// Move to cancel decision when artifacts exist.
	for m.InitField() != screens.InitFieldDecisionCancel {
		prev := m.InitField()
		m = mustModel(m.Update(key("down")))
		if m.InitField() == prev {
			t.Fatalf("could not reach cancel field, stuck at %d", prev)
		}
	}
	m = mustModel(m.Update(key("enter")))
	if m.InitDecision() != tui.InitDecisionCancel {
		t.Fatalf("decision = %q, want cancel", m.InitDecision())
	}
	m = mustModel(m.Update(key("home")))
	m = mustModel(m.Update(key("end")))
	if !strings.Contains(m.View(), "Initialization canceled") {
		t.Fatalf("missing canceled message:\n%s", m.View())
	}

	m = mustModel(m.Update(key("r")))
	if m.DraftName() != baseName || m.InitModeConfirmed() != tui.InitModeExisting || m.InitDecision() != tui.InitDecisionInitialize {
		t.Fatalf("reset failed: name=%q mode=%q decision=%q", m.DraftName(), m.InitModeConfirmed(), m.InitDecision())
	}
	assertNoMutation(t, root)

	m = mustModel(m.Update(key("tab")))
	if m.Focus() != tui.FocusSidebar {
		t.Fatalf("tab back focus = %v", m.Focus())
	}
}

func TestInitNoArtifactsHidesGateAndNextOpensStep2(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	if m.InitWizardStep() != screens.InitWizardStepProject {
		t.Fatalf("step = %d, want project setup", m.InitWizardStep())
	}
	view := m.View()
	for _, want := range []string{
		"Step 1 — Project Setup",
		"No existing runtime/adaptor artifacts detected.",
		"Atlas can initialize normally.",
		"Next",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{
		"Do not initialize Atlas",
		"Initialize Atlas — backup and replace",
		"Initialization Decision",
		"Plan Preview",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}

	m = mustModel(m.Update(key("tab")))
	for m.InitField() != screens.InitFieldNext {
		prev := m.InitField()
		m = mustModel(m.Update(key("down")))
		if m.InitField() == prev {
			t.Fatalf("could not reach Next, stuck at %d", prev)
		}
	}
	m = mustModel(m.Update(key("enter")))
	if m.InitWizardStep() != screens.InitWizardStepConfig {
		t.Fatalf("step = %d, want config", m.InitWizardStep())
	}
	view = m.View()
	for _, want := range []string{
		"Step 2 — Initial Configuration",
		"Project:",
		"Sections",
		"Governance",
		"Adapters",
		"Source Control",
		"Memory",
		"[ Back ]",
		"[ Next ]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	backIdx := strings.Index(view, "[ Back ]")
	nextIdx := strings.Index(view, "[ Next ]")
	if backIdx < 0 || nextIdx < 0 || backIdx >= nextIdx {
		t.Fatalf("Back must appear before Next in compact footer:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("Step 2 must not render Action heading:\n%s", view)
		}
	}
	for _, banned := range []string{
		"Project Stack", "Skills / Registry", "Tech / Libraries",
		"Generic AGENTS.md", "Runtime entrypoint", "Governed basic", "Review only",
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("unexpected %q:\n%s", banned, view)
		}
	}
	selector := m.ConfigDraft().SelectorSections()
	if len(selector) != 4 {
		t.Fatalf("selector sections = %d, want 4", len(selector))
	}
	if strings.Contains(view, "Plan Preview") {
		t.Fatal("step 2 must not show Plan Preview")
	}
	assertActionNearFooter(t, view)

	// Governance: arrows never mutate; Space/Enter selects Spec engine.
	m = mustModel(m.Update(key("enter")))
	workflow, _ := m.ConfigDraft().FieldByKey("governance.default_workflow")
	if workflow.Value != "sdd" {
		t.Fatalf("workflow = %q, want sdd", workflow.Value)
	}
	m = mustModel(m.Update(key("down"))) // OpenSpec row
	m = mustModel(m.Update(key("down"))) // None row
	engine, _ := m.ConfigDraft().FieldByKey("governance.spec_engine")
	if engine.Value != "openspec" {
		t.Fatalf("arrows mutated spec engine to %q", engine.Value)
	}
	m = mustModel(m.Update(key("enter")))
	engine, _ = m.ConfigDraft().FieldByKey("governance.spec_engine")
	if engine.Value != "none" {
		t.Fatalf("spec engine = %q, want none", engine.Value)
	}
	m = mustModel(m.Update(key("left"))) // sections
	engine, _ = m.ConfigDraft().FieldByKey("governance.spec_engine")
	if engine.Value != "none" {
		t.Fatalf("left mutated spec engine to %q", engine.Value)
	}
	m = mustModel(m.Update(key("enter"))) // back into governance
	engine, _ = m.ConfigDraft().FieldByKey("governance.spec_engine")
	if engine.Value != "none" {
		t.Fatalf("returning lost spec engine, got %q", engine.Value)
	}

	// Boolean toggles only on Space/Enter.
	for {
		field, ok := screens.ActiveField(m.ConfigDraft(), m.ConfigSectionIndex(), m.ConfigFieldIndex())
		if ok && field.Key == "governance.testing_required" {
			break
		}
		prevField, prevOpt := m.ConfigFieldIndex(), m.ConfigOptionIndex()
		m = mustModel(m.Update(key("down")))
		if m.ConfigFieldIndex() == prevField && m.ConfigOptionIndex() == prevOpt {
			t.Fatal("could not reach testing_required")
		}
	}
	before, _ := m.ConfigDraft().FieldByKey("governance.testing_required")
	m = mustModel(m.Update(key("left")))
	m = mustModel(m.Update(key("right")))
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("up")))
	afterNav, _ := m.ConfigDraft().FieldByKey("governance.testing_required")
	if afterNav.Value != before.Value {
		t.Fatalf("arrows mutated bool %q -> %q", before.Value, afterNav.Value)
	}
	// Re-focus testing_required if needed.
	for {
		field, ok := screens.ActiveField(m.ConfigDraft(), m.ConfigSectionIndex(), m.ConfigFieldIndex())
		if ok && field.Key == "governance.testing_required" && m.ConfigPanel() == screens.ConfigPanelFields {
			break
		}
		if m.ConfigPanel() == screens.ConfigPanelSections {
			m = mustModel(m.Update(key("enter")))
		}
		m = mustModel(m.Update(key("down")))
	}
	m = mustModel(m.Update(key(" ")))
	toggled, _ := m.ConfigDraft().FieldByKey("governance.testing_required")
	if toggled.Value == before.Value {
		t.Fatal("space should toggle testing_required")
	}

	// Adapters: multiselect only on Space/Enter.
	m = mustModel(m.Update(key("left")))
	m = gotoConfigSection(t, m, "adapters")
	m = mustModel(m.Update(key("enter")))
	adaptersBefore, _ := m.ConfigDraft().FieldByKey("adapters.selected")
	m = mustModel(m.Update(key("down")))
	adaptersMid, _ := m.ConfigDraft().FieldByKey("adapters.selected")
	if adaptersMid.Value != adaptersBefore.Value {
		t.Fatalf("arrow mutated adapters %q -> %q", adaptersBefore.Value, adaptersMid.Value)
	}
	m = mustModel(m.Update(key("enter")))
	adaptersAfter, _ := m.ConfigDraft().FieldByKey("adapters.selected")
	if adaptersAfter.Value == adaptersBefore.Value {
		t.Fatalf("expected adapter multiselect change, still %q", adaptersAfter.Value)
	}

	// Source Control: mode/branch/storage only on Space/Enter.
	m = mustModel(m.Update(key("left")))
	m = gotoConfigSection(t, m, "source_control")
	m = mustModel(m.Update(key("enter")))
	modeBefore, _ := m.ConfigDraft().FieldByKey("source_control.mode")
	m = mustModel(m.Update(key("down")))
	modeMid, _ := m.ConfigDraft().FieldByKey("source_control.mode")
	if modeMid.Value != modeBefore.Value {
		t.Fatalf("arrow mutated source control mode")
	}
	m = mustModel(m.Update(key("enter")))
	modeAfter, _ := m.ConfigDraft().FieldByKey("source_control.mode")
	if modeAfter.Value == modeBefore.Value {
		t.Fatalf("enter should change source control mode")
	}

	for {
		field, ok := screens.ActiveField(m.ConfigDraft(), m.ConfigSectionIndex(), m.ConfigFieldIndex())
		if ok && field.Key == "source_control.branch_strategy" {
			break
		}
		prevField, prevOpt := m.ConfigFieldIndex(), m.ConfigOptionIndex()
		m = mustModel(m.Update(key("down")))
		if m.ConfigFieldIndex() == prevField && m.ConfigOptionIndex() == prevOpt {
			t.Fatal("could not reach branch strategy")
		}
	}
	branchBefore, _ := m.ConfigDraft().FieldByKey("source_control.branch_strategy")
	m = mustModel(m.Update(key("down")))
	branchMid, _ := m.ConfigDraft().FieldByKey("source_control.branch_strategy")
	if branchMid.Value != branchBefore.Value {
		t.Fatal("arrow mutated branch strategy")
	}
	m = mustModel(m.Update(key("enter")))
	branchAfter, _ := m.ConfigDraft().FieldByKey("source_control.branch_strategy")
	if branchAfter.Value == branchBefore.Value {
		t.Fatal("enter should change branch strategy")
	}

	for {
		field, ok := screens.ActiveField(m.ConfigDraft(), m.ConfigSectionIndex(), m.ConfigFieldIndex())
		if ok && field.Key == "source_control.governance_storage" {
			break
		}
		prevField, prevOpt := m.ConfigFieldIndex(), m.ConfigOptionIndex()
		m = mustModel(m.Update(key("down")))
		if m.ConfigFieldIndex() == prevField && m.ConfigOptionIndex() == prevOpt {
			t.Fatal("could not reach Atlas governance files field")
		}
	}
	m = mustModel(m.Update(key("down"))) // Versioned focus
	storage, _ := m.ConfigDraft().FieldByKey("source_control.governance_storage")
	if storage.Value != "local_only" {
		t.Fatalf("arrow mutated governance files to %q", storage.Value)
	}
	m = mustModel(m.Update(key("enter")))
	storage, _ = m.ConfigDraft().FieldByKey("source_control.governance_storage")
	if storage.Value != "versioned" {
		t.Fatalf("governance files = %q, want versioned", storage.Value)
	}

	// Memory strategy only on Space/Enter; persists across navigate.
	m = mustModel(m.Update(key("left")))
	m = gotoConfigSection(t, m, "memory")
	m = mustModel(m.Update(key("enter")))
	mem, _ := m.ConfigDraft().FieldByKey("memory.strategy")
	if mem.Value != "sqlite_plus_context_capsule" {
		t.Fatalf("default memory = %q", mem.Value)
	}
	m = mustModel(m.Update(key("down"))) // Context Capsule focus
	mem, _ = m.ConfigDraft().FieldByKey("memory.strategy")
	if mem.Value != "sqlite_plus_context_capsule" {
		t.Fatalf("arrow mutated memory to %q", mem.Value)
	}
	m = mustModel(m.Update(key("up"))) // SQLite focus
	m = mustModel(m.Update(key("enter")))
	mem, _ = m.ConfigDraft().FieldByKey("memory.strategy")
	if mem.Value != "sqlite" {
		t.Fatalf("memory = %q, want sqlite", mem.Value)
	}
	m = mustModel(m.Update(key("left")))
	m = gotoConfigSection(t, m, "adapters")
	m = gotoConfigSection(t, m, "memory")
	memBack, _ := m.ConfigDraft().FieldByKey("memory.strategy")
	if memBack.Value != "sqlite" {
		t.Fatalf("memory strategy lost after navigate, got %q", memBack.Value)
	}

	// Locked project.name cannot change.
	draft := m.ConfigDraft()
	if draft.SelectOption("project.name", "hacked") {
		t.Fatal("project.name must not be editable")
	}

	// Move to footer Next and confirm Step 2.
	if m.ConfigPanel() == screens.ConfigPanelSections {
		m = mustModel(m.Update(key("enter")))
	}
	for m.ConfigPanel() != screens.ConfigPanelFooter {
		prevPanel := m.ConfigPanel()
		prevField := m.ConfigFieldIndex()
		prevOpt := m.ConfigOptionIndex()
		prevFooter := m.ConfigFooterIndex()
		m = mustModel(m.Update(key("down")))
		if m.ConfigPanel() == prevPanel &&
			m.ConfigFieldIndex() == prevField &&
			m.ConfigOptionIndex() == prevOpt &&
			m.ConfigFooterIndex() == prevFooter {
			t.Fatal("could not reach footer")
		}
	}
	if m.ConfigFooterIndex() != 1 {
		m = mustModel(m.Update(key("right")))
	}
	m = mustModel(m.Update(key("enter")))
	if !m.InitConfigConfirmed() {
		t.Fatal("expected config step confirmed")
	}
	m = mustModel(m.Update(key("end")))
	view = m.View()
	for _, want := range []string{
		"Configuration ready.",
		"Review / Materialization Plan is not implemented in this slice.",
		"No files were changed.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}

	if m.ConfigFooterIndex() != 0 {
		m = mustModel(m.Update(key("left")))
	}
	m = mustModel(m.Update(key("enter")))
	if m.InitWizardStep() != screens.InitWizardStepProject {
		t.Fatalf("back step = %d, want project", m.InitWizardStep())
	}
	if !strings.Contains(m.View(), "Step 1 — Project Setup") {
		t.Fatalf("expected step 1 after back:\n%s", m.View())
	}

	assertNoMutation(t, root)
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not be created, stat err = %v", err)
	}
}

func sectionIndexOf(m tui.Model, key string) int {
	for i, section := range m.ConfigDraft().SelectorSections() {
		if section.Key == key {
			return i
		}
	}
	return -1
}

func gotoConfigSection(t *testing.T, m tui.Model, sectionKey string) tui.Model {
	t.Helper()
	target := sectionIndexOf(m, sectionKey)
	if target < 0 {
		t.Fatalf("missing section %q", sectionKey)
	}
	if m.Focus() != tui.FocusContent {
		m = mustModel(m.Update(key("tab")))
	}
	for i := 0; i < 16 && m.ConfigPanel() != screens.ConfigPanelSections; i++ {
		prevPanel := m.ConfigPanel()
		prevOpt := m.ConfigOptionIndex()
		prevField := m.ConfigFieldIndex()
		prevFooter := m.ConfigFooterIndex()
		m = mustModel(m.Update(key("up")))
		if m.ConfigPanel() == screens.ConfigPanelSections {
			break
		}
		if m.ConfigPanel() == prevPanel && m.ConfigOptionIndex() == prevOpt && m.ConfigFieldIndex() == prevField && m.ConfigFooterIndex() == prevFooter {
			m = mustModel(m.Update(key("left")))
		}
	}
	if m.ConfigPanel() != screens.ConfigPanelSections {
		t.Fatalf("could not return to section list for %q (panel=%q focus=%v field=%d)", sectionKey, m.ConfigPanel(), m.Focus(), m.ConfigFieldIndex())
	}
	for m.ConfigSectionIndex() < target {
		prev := m.ConfigSectionIndex()
		m = mustModel(m.Update(key("down")))
		if m.ConfigSectionIndex() == prev {
			t.Fatalf("could not reach section %q (at %d)", sectionKey, prev)
		}
	}
	for m.ConfigSectionIndex() > target {
		prev := m.ConfigSectionIndex()
		m = mustModel(m.Update(key("up")))
		if m.ConfigSectionIndex() == prev {
			t.Fatalf("could not reach section %q (at %d)", sectionKey, prev)
		}
	}
	return m
}

func TestInitContentFocusGlobalKeys(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = mustModel(m.Update(key("tab")))
	// Leave the project-name text field so b remains a global dashboard shortcut.
	m = mustModel(m.Update(key("down")))
	m, cmd := apply(m, key("b"))
	m = applyCmd(t, m, cmd)
	if m.Route() != tui.DefaultRoute {
		t.Fatalf("b route = %v, want dashboard", m.Route())
	}
}

func TestErrorDialogView(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{
		Route:          tui.RouteError,
		UnknownCommand: "start",
	}))
	view := m.View()

	for _, want := range []string{
		"Atlas",
		"Enter/q/Esc salir",
		"Error",
		"atlas start",
		"Salir",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in error layout:\n%s", want, view)
		}
	}
	for _, banned := range []string{"Init / Setup", "Status", "Doctor", "Help", "Exit", "Dashboard", "Configure", "MCP"} {
		if strings.Contains(view, banned) {
			t.Fatalf("error layout must not contain %q:\n%s", banned, view)
		}
	}
	assertCompactErrorLayout(t, view)
	assertGlobalTopGap(t, view)
}

func assertCompactErrorLayout(t *testing.T, view string) {
	t.Helper()
	if strings.Count(view, "Enter/q/Esc salir") != 1 {
		t.Fatalf("error footer should appear once:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	start, end := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "╭") && start < 0 {
			start = i
		}
		if strings.Contains(line, "╰") {
			end = i
		}
	}
	if start < 0 || end <= start {
		t.Fatalf("missing error panel:\n%s", view)
	}
	maxBlank, cur := 0, 0
	for _, line := range lines[start+1 : end] {
		trimmed := strings.TrimSpace(strings.Trim(line, "│"))
		if trimmed == "" {
			cur++
			if cur > maxBlank {
				maxBlank = cur
			}
			continue
		}
		cur = 0
	}
	if maxBlank > 3 {
		t.Fatalf("huge blank block inside error panel (%d consecutive blank lines):\n%s", maxBlank, view)
	}
	blockH := end - start + 1
	if blockH > 24 {
		t.Fatalf("error panel height %d is not content-fit:\n%s", blockH, view)
	}
}

func TestErrorDialogKeysQuitOnly(t *testing.T) {
	for _, k := range []tea.KeyMsg{key("enter"), key("q"), {Type: tea.KeyEsc}, {Type: tea.KeyCtrlC}} {
		m := sized(tui.NewModel(tui.Options{Route: tui.RouteError, UnknownCommand: "change new"}))
		updated, cmd := m.Update(k)
		model := updated.(tui.Model)
		if !model.Quitting() {
			t.Fatalf("%v should quit error dialog", k)
		}
		if cmd == nil {
			t.Fatalf("%v expected quit cmd", k)
		}
	}

	m := sized(tui.NewModel(tui.Options{Route: tui.RouteError, UnknownCommand: "start"}))
	for _, k := range []tea.KeyMsg{key("h"), key("?"), key("b"), key("up"), key("down"), key("j"), key("k")} {
		updated, _ := m.Update(k)
		model := updated.(tui.Model)
		if model.Quitting() {
			t.Fatalf("%v must not quit", k)
		}
		if model.Route() != tui.RouteError {
			t.Fatalf("%v changed route to %v", k, model.Route())
		}
	}
}

func TestSmallTerminalView(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.DefaultRoute})
	m = mustModel(m.Update(tea.WindowSizeMsg{Width: 40, Height: 12}))
	view := m.View()
	if !strings.Contains(view, "larger terminal") {
		t.Fatalf("expected small-terminal message:\n%s", view)
	}
	if !strings.Contains(view, "quit") {
		t.Fatalf("small terminal must keep quit hint:\n%s", view)
	}
}

func TestMinHeightOmitsTopGap(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteDashboard, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = mustModel(m.Update(tea.WindowSizeMsg{Width: 80, Height: tui.MinHeight}))
	view := m.View()
	if !strings.Contains(view, "Atlas") {
		t.Fatalf("missing header:\n%s", view)
	}
	if !strings.Contains(view, "q quit") {
		t.Fatalf("missing footer:\n%s", view)
	}
	first := strings.Split(view, "\n")[0]
	if strings.TrimSpace(first) == "" {
		t.Fatalf("min-height view should omit top gap:\n%s", view)
	}
}

func assertShell(t *testing.T, view, contentTitle string) {
	t.Helper()
	for _, want := range []string{"Atlas", "Dashboard", "MCP", "Status", "Doctor", "Help", "Exit", contentTitle} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in view:\n%s", want, view)
		}
	}
	// Reject old placeholder sidebar entries only (not "Initial Configuration" copy).
	if strings.Contains(view, "› Configuration") || strings.Contains(view, "  Configuration ") || strings.Contains(view, "Assets") {
		t.Fatalf("ghost entries in view:\n%s", view)
	}
}

func assertGlobalTopGap(t *testing.T, view string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("view too short for top gap:\n%s", view)
	}
	if strings.TrimSpace(lines[0]) != "" {
		t.Fatalf("expected one blank top gap line:\n%s", view)
	}
	if strings.TrimSpace(lines[1]) == "" {
		t.Fatalf("top gap must not be duplicated:\n%s", view)
	}
	headerAt := -1
	for i := 1; i < len(lines) && i < 4; i++ {
		if strings.Contains(lines[i], "Atlas") {
			headerAt = i
			break
		}
	}
	if headerAt < 0 {
		t.Fatalf("Atlas header should follow top gap immediately:\n%s", view)
	}
}

func assertInitWizardView(t *testing.T, view string) {
	t.Helper()
	for _, want := range []string{
		"Step 1 — Project Setup",
		"Project Identity",
		"Project name:",
		"Project Detection",
		"Project Mode",
		"New project",
		"Existing project",
		"Runtime Artifacts",
		"Initialize Atlas — backup and replace",
		"Do not initialize Atlas",
		"No files will be changed in this slice.",
		"[ Next ]",
		"Tab focus",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in init view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Auto-detect") {
		t.Fatalf("Auto-detect must not appear:\n%s", view)
	}
	if strings.Contains(view, "Plan Preview") {
		t.Fatalf("step 1 must not show Plan Preview:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("step 1 must not render Action heading:\n%s", view)
		}
	}
	if !strings.Contains(view, "╭") && !strings.Contains(view, "┌") {
		t.Fatalf("expected bordered input chrome:\n%s", view)
	}
	assertActionNearFooter(t, view)
	assertGlobalTopGap(t, view)
}

func sized(m tui.Model) tui.Model {
	return mustModel(m.Update(tea.WindowSizeMsg{Width: 120, Height: 60}))
}

func loadWorkspace(t *testing.T, opts tui.Options) tui.Model {
	t.Helper()
	m := sized(tui.NewModel(opts))
	return applyCmd(t, m, m.Init())
}

func apply(m tui.Model, msg tea.Msg) (tui.Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(tui.Model), cmd
}

func applyCmd(t *testing.T, m tui.Model, cmd tea.Cmd) tui.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	updated, _ := m.Update(msg)
	return updated.(tui.Model)
}

func mustModel(updated tea.Model, _ tea.Cmd) tui.Model {
	return updated.(tui.Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func sidebarLabel(m tui.Model, index int) string {
	items := m.Sidebar()
	if index < 0 || index >= len(items) {
		return ""
	}
	return items[index].Label
}

func sidebarLabels(m tui.Model) []string {
	items := m.Sidebar()
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.Label
	}
	return labels
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func indexOf(list []string, want string) int {
	for i, item := range list {
		if item == want {
			return i
		}
	}
	return -1
}

func writeValidAtlasConfig(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig("demo")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".atlas", "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertNoMutation(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not be created, stat err = %v", err)
	}
}
