package tui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestDefaultModelRoute(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.DefaultRoute})
	if m.Route() != tui.RouteStatus {
		t.Fatalf("route = %v, want status", m.Route())
	}
	if got := sidebarLabel(m.SidebarIndex()); got != "Status" {
		t.Fatalf("sidebar selected = %q, want Status", got)
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

	// Move to Doctor (index 2) and enter.
	m = sized(tui.NewModel(tui.Options{Route: tui.DefaultRoute}))
	for sidebarLabel(m.SidebarIndex()) != "Doctor" {
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
	for sidebarLabel(m.SidebarIndex()) != "Exit" {
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

func TestDirectRouteSelectsSidebar(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteDoctor})
	if sidebarLabel(m.SidebarIndex()) != "Doctor" {
		t.Fatalf("sidebar = %q, want Doctor", sidebarLabel(m.SidebarIndex()))
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
		t.Fatalf("route = %v, want default status", m.Route())
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
		t.Fatal("esc from status should quit")
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
		t.Fatalf("esc route = %v, want status", m.Route())
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
	// Force a short viewport so doctor content scrolls.
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

	status := loadWorkspace(t, tui.Options{
		Route: tui.RouteStatus, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, status.View(), "Atlas Status")
	assertNoMutation(t, root)

	initM := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, initM.View(), "Atlas Init Plan")
	assertNoMutation(t, root)

	doc := loadWorkspace(t, tui.Options{
		Route: tui.RouteDoctor, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, doc.View(), "Atlas Doctor")

	help := sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
	assertShell(t, help.View(), "Atlas Help")
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
	for _, banned := range []string{"Init / Setup", "Status", "Doctor", "Help", "Exit", "Atlas Help"} {
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
	for _, want := range []string{"Atlas", "Init / Setup", "Status", "Doctor", "Help", "Exit", contentTitle} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Configuration") || strings.Contains(view, "Assets") {
		t.Fatalf("ghost entries in view:\n%s", view)
	}
}

func sized(m tui.Model) tui.Model {
	return mustModel(m.Update(tea.WindowSizeMsg{Width: 100, Height: 32}))
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
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
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

func sidebarLabel(index int) string {
	if index < 0 || index >= len(tui.SidebarItems) {
		return ""
	}
	return tui.SidebarItems[index].Label
}

func assertNoMutation(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not exist, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not exist, stat err = %v", err)
	}
}
