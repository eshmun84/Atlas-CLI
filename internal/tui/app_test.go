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
	m := tui.NewModel(tui.Options{Route: tui.RouteHome})
	if m.Route() != tui.RouteHome {
		t.Fatalf("route = %v, want home", m.Route())
	}
	if m.Width() != tui.MinWidth || m.Height() != tui.MinHeight {
		t.Fatalf("default size = %dx%d", m.Width(), m.Height())
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteHome})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(tui.Model)
	if model.Width() != 120 || model.Height() != 40 {
		t.Fatalf("size = %dx%d, want 120x40", model.Width(), model.Height())
	}
}

func TestHomeSelectionAndEnter(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteHome})
	if m.Selected() != 0 {
		t.Fatalf("selected = %d, want 0", m.Selected())
	}

	m = mustModel(m.Update(key("down")))
	if m.Selected() != 1 {
		t.Fatalf("down selected = %d, want 1", m.Selected())
	}

	m = mustModel(m.Update(key("up")))
	if m.Selected() != 0 {
		t.Fatalf("up selected = %d, want 0", m.Selected())
	}

	m = mustModel(m.Update(key("j")))
	m = mustModel(m.Update(key("j")))
	if m.Selected() != 2 {
		t.Fatalf("j selected = %d, want 2", m.Selected())
	}

	m = mustModel(m.Update(key("k")))
	if m.Selected() != 1 {
		t.Fatalf("k selected = %d, want 1", m.Selected())
	}

	updated, cmd := m.Update(key("enter"))
	model := updated.(tui.Model)
	if model.Route() != tui.RouteStatus {
		t.Fatalf("enter route = %v, want status", model.Route())
	}
	_ = cmd
}

func TestHomeEnterExitQuits(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteHome})
	for i := 0; i < len(tui.HomeItems)-1; i++ {
		m = mustModel(m.Update(key("down")))
	}
	item := tui.HomeItems[m.Selected()]
	if !item.Exit {
		t.Fatalf("expected Exit selected, got %q", item.Label)
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

func TestHelpKeys(t *testing.T) {
	for _, k := range []string{"h", "?"} {
		m := tui.NewModel(tui.Options{Route: tui.RouteHome})
		m = mustModel(m.Update(key(k)))
		if m.Route() != tui.RouteHelp {
			t.Fatalf("%q route = %v, want help", k, m.Route())
		}
	}
}

func TestBReturnsHome(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteStatus})
	m = mustModel(m.Update(key("b")))
	if m.Route() != tui.RouteHome {
		t.Fatalf("route = %v, want home", m.Route())
	}
}

func TestQQuits(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteHelp})
	updated, cmd := m.Update(key("q"))
	model := updated.(tui.Model)
	if !model.Quitting() {
		t.Fatal("q should quit")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestEscQuitsFromHome(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteHome})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := updated.(tui.Model)
	if !model.Quitting() {
		t.Fatal("esc from home should quit")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

func TestEscGoesBackFromChild(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteHelp})
	if m.Previous() != tui.RouteHome {
		t.Fatalf("previous = %v, want home", m.Previous())
	}
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.Route() != tui.RouteHome {
		t.Fatalf("esc back route = %v, want home", m.Route())
	}
	if m.Quitting() {
		t.Fatal("esc from child should not quit")
	}
}

func TestHomeView(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{Route: tui.RouteHome}))
	view := m.View()
	if !strings.Contains(view, "Atlas") {
		t.Fatalf("home missing Atlas:\n%s", view)
	}
	if !strings.Contains(view, "Init / Setup") {
		t.Fatalf("home missing Init / Setup:\n%s", view)
	}
}

func TestHelpView(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
	view := m.View()
	if !strings.Contains(view, "Atlas Help") {
		t.Fatalf("help missing title:\n%s", view)
	}
}

func TestInitView(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	view := m.View()
	if !strings.Contains(view, "Atlas Init Plan") {
		t.Fatalf("init missing title:\n%s", view)
	}
	if !strings.Contains(view, "No files were created.") {
		t.Fatalf("init missing dry-run note:\n%s", view)
	}
	assertNoMutation(t, root)
}

func TestStatusView(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteStatus,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	view := m.View()
	if !strings.Contains(view, "Atlas Status") {
		t.Fatalf("status missing title:\n%s", view)
	}
	assertNoMutation(t, root)
}

func TestDoctorView(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteDoctor,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	view := m.View()
	if !strings.Contains(view, "Atlas Doctor") {
		t.Fatalf("doctor missing title:\n%s", view)
	}
	assertNoMutation(t, root)
}

func TestErrorView(t *testing.T) {
	m := sized(tui.NewModel(tui.Options{
		Route:          tui.RouteError,
		UnknownCommand: "start",
	}))
	view := m.View()
	if !strings.Contains(view, "Unknown command") {
		t.Fatalf("error missing title:\n%s", view)
	}
	if !strings.Contains(view, "start") {
		t.Fatalf("error missing command:\n%s", view)
	}
}

func TestSmallTerminalView(t *testing.T) {
	m := tui.NewModel(tui.Options{Route: tui.RouteHome})
	m = mustModel(m.Update(tea.WindowSizeMsg{Width: 40, Height: 12}))
	view := m.View()
	if !strings.Contains(view, "larger terminal") {
		t.Fatalf("expected small-terminal message:\n%s", view)
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
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
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
