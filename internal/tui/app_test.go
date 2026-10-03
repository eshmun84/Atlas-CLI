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
	if !contains(labels, "Init / Setup") || contains(labels, "Configure") {
		t.Fatalf("uninitialized sidebar = %v", labels)
	}

	writeValidAtlasConfig(t, root)
	initd := loadWorkspace(t, tui.Options{
		Route: tui.RouteDashboard, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	labels = sidebarLabels(initd)
	if !contains(labels, "Configure") || contains(labels, "Init / Setup") {
		t.Fatalf("initialized sidebar = %v", labels)
	}

	cfg := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	view := cfg.View()
	for _, want := range []string{"Configure", "Config path", "demo", "read-only"} {
		if !strings.Contains(view, want) {
			t.Fatalf("configure missing %q:\n%s", want, view)
		}
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

func TestInitNoArtifactsHidesGateAndNextConfirms(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	view := m.View()
	for _, want := range []string{
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
	if !m.InitStepConfirmed() {
		t.Fatal("expected step confirmed")
	}
	view = m.View()
	for _, want := range []string{
		"Step ready.",
		"Next wizard step is not implemented in this slice.",
		"No files were changed.",
		"Action",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing step confirmation %q:\n%s", want, view)
		}
	}
	assertNoMutation(t, root)
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not be created, stat err = %v", err)
	}
}

func TestInitContentFocusGlobalKeys(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = mustModel(m.Update(key("tab")))
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
	for _, banned := range []string{"Init / Setup", "Status", "Doctor", "Help", "Exit", "Dashboard", "Configure"} {
		if strings.Contains(view, banned) {
			t.Fatalf("error layout must not contain %q:\n%s", banned, view)
		}
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
}

func assertShell(t *testing.T, view, contentTitle string) {
	t.Helper()
	for _, want := range []string{"Atlas", "Dashboard", "Status", "Doctor", "Help", "Exit", contentTitle} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Configuration") || strings.Contains(view, "Assets") {
		t.Fatalf("ghost entries in view:\n%s", view)
	}
}

func assertInitWizardView(t *testing.T, view string) {
	t.Helper()
	for _, want := range []string{
		"Project Identity",
		"Project name:",
		"Project Detection",
		"Project Mode",
		"New project",
		"Existing project",
		"Runtime Artifacts",
		"Initialize Atlas — backup and replace",
		"Do not initialize Atlas",
		"Plan Preview",
		"No files will be created in this step.",
		"Action",
		"Next",
		"Tab focus",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in init view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Auto-detect") {
		t.Fatalf("Auto-detect must not appear:\n%s", view)
	}
	if idxName := strings.Index(view, "Project name:"); idxName < 0 {
		t.Fatal("missing project name label")
	}
	if !strings.Contains(view, "╭") && !strings.Contains(view, "┌") {
		t.Fatalf("expected bordered input/button chrome:\n%s", view)
	}
	planIdx := strings.Index(view, "Plan Preview")
	actionIdx := strings.Index(view, "Action")
	if planIdx < 0 || actionIdx < 0 || actionIdx < planIdx {
		t.Fatalf("Action must appear after Plan Preview: plan=%d action=%d", planIdx, actionIdx)
	}
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
