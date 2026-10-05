package tui_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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
	if !contains(labels, "Init / Setup") || contains(labels, "Configure") || contains(labels, "MCP") || contains(labels, "Runtime Repair") {
		t.Fatalf("uninitialized sidebar = %v", labels)
	}

	writeValidAtlasConfig(t, root)
	initd := loadWorkspace(t, tui.Options{
		Route: tui.RouteDashboard, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	labels = sidebarLabels(initd)
	if !contains(labels, "Configure") || contains(labels, "Init / Setup") || contains(labels, "MCP") {
		t.Fatalf("initialized sidebar = %v", labels)
	}
	if !contains(labels, "Runtime Repair") {
		t.Fatalf("initialized sidebar missing Runtime Repair: %v", labels)
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
		"Context",
		"MCP",
		"Project:",
		"[ Close ]",
		"[ Apply changes ]",
		"Close discards unsaved changes. Apply changes writes .atlas/config.yaml.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("configure missing %q:\n%s", want, view)
		}
	}
	for _, banned := range []string{"[ Next ]", "Runtime entrypoint", "Skills / Registry", "Project Stack"} {
		if strings.Contains(view, banned) {
			t.Fatalf("configure unexpected %q:\n%s", banned, view)
		}
	}
	if _, ok := cfg.ConfigDraft().FieldByKey("runtime.entrypoint"); ok {
		t.Fatal("runtime fields must not exist in draft")
	}
	// Sidebar should show Configure, not Init / Setup or MCP.
	if contains(sidebarLabels(cfg), "Init / Setup") || contains(sidebarLabels(cfg), "MCP") || !contains(sidebarLabels(cfg), "Configure") {
		t.Fatalf("sidebar = %v", sidebarLabels(cfg))
	}
}

func TestConfigureMCPLoadsAndEditsInMemory(t *testing.T) {
	root := t.TempDir()
	writePersistedAtlasConfig(t, root, func(doc *config.ProjectDocument) {
		doc.MCP.Builtins.Jira.Enabled = true
		doc.MCP.Builtins.Context7.Enabled = true
		doc.MCP.Builtins.ChromeDevTools.Enabled = true
		doc.MCP.Custom = []config.MCPCustomPersist{{
			Name:         "Internal Docs",
			Transport:    "http",
			CommandOrURL: "https://example.local/mcp",
			Enabled:      true,
		}}
	})

	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	if contains(sidebarLabels(m), "MCP") {
		t.Fatalf("sidebar must not list MCP: %v", sidebarLabels(m))
	}
	m = gotoConfigSection(t, m, "mcp")
	m = mustModel(m.Update(key("enter")))
	view := m.View()
	for _, want := range []string{
		"MCP",
		"Built-in MCPs",
		"[x] Jira",
		"[x] Context7",
		"[x] Chrome DevTools",
		"Internal Docs",
		"[ Add MCP ]",
		"[ Apply changes ]",
		"Close discards unsaved changes. Apply changes writes .atlas/config.yaml.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("configure mcp missing %q:\n%s", want, view)
		}
	}
	if !m.MCPDraft().Builtins[0].Enabled || !m.MCPDraft().Builtins[1].Enabled || !m.MCPDraft().Builtins[2].Enabled {
		t.Fatalf("loaded builtins = %#v", m.MCPDraft().Builtins)
	}
	if len(m.MCPDraft().CustomServers) != 1 || m.MCPDraft().CustomServers[0].Name != "Internal Docs" {
		t.Fatalf("loaded custom = %#v", m.MCPDraft().CustomServers)
	}

	jiraBefore := m.MCPDraft().Builtins[0].Enabled
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("up")))
	if m.MCPDraft().Builtins[0].Enabled != jiraBefore {
		t.Fatal("arrows mutated configure mcp")
	}
	m = mustModel(m.Update(key("enter"))) // toggle jira off
	if m.MCPDraft().Builtins[0].Enabled {
		t.Fatal("expected jira toggled off in memory")
	}
	if !strings.Contains(m.View(), "[ Apply changes ]") || !strings.Contains(m.View(), "[ Close ]") {
		t.Fatalf("Apply changes must stay visible after toggle:\n%s", m.View())
	}

	before, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m = mcpGotoAddButton(t, m)
	m = mustModel(m.Update(key("enter")))
	if !strings.Contains(m.View(), "[ Cancel ]") || !strings.Contains(m.View(), "[ Add ]") {
		t.Fatalf("add form actions missing:\n%s", m.View())
	}
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Temp MCP")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if len(m.MCPDraft().CustomServers) != 2 {
		t.Fatalf("custom after add = %#v", m.MCPDraft().CustomServers)
	}
	afterAdd := m.View()
	for _, want := range []string{"[ Close ]", "[ Apply changes ]", "Temp MCP", "Apply changes (below) saves"} {
		if !strings.Contains(afterAdd, want) {
			t.Fatalf("after Add MCP missing %q:\n%s", want, afterAdd)
		}
	}
	if strings.Contains(afterAdd, "[ Cancel ]") && strings.Contains(afterAdd, "[ Add ]") && !strings.Contains(afterAdd, "[ Apply changes ]") {
		t.Fatal("Add form actions must not replace Apply changes after return to list")
	}
	after, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("configure edits must not persist config.yaml before Apply changes")
	}

	m = gotoInitFooterAction(t, m, false) // Close
	m, cmd := apply(m, key("enter"))
	m = applyCmd(t, m, cmd)
	if m.Route() != tui.DefaultRoute {
		t.Fatalf("close route = %v", m.Route())
	}
	closed, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(closed) {
		t.Fatal("Close without Apply must discard MCP edits")
	}
	assertRuntimeNotMaterialized(t, root)
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("AGENTS.md must not be created")
	}
}

func TestConfigureMCPApplyChangesAlwaysVisible(t *testing.T) {
	root := t.TempDir()
	writeValidAtlasConfig(t, root)
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertActionNearFooter(t, m.View())
	if !strings.Contains(m.View(), "[ Close ]") || !strings.Contains(m.View(), "[ Apply changes ]") {
		t.Fatalf("configure root missing actions:\n%s", m.View())
	}

	m = gotoConfigSection(t, m, "mcp")
	m = mustModel(m.Update(key("enter")))
	// Select Chrome DevTools (index 2).
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("enter")))
	if !m.MCPDraft().Builtins[2].Enabled {
		t.Fatal("chrome should be selected")
	}
	view := m.View()
	for _, want := range []string{"[x] Chrome DevTools", "[ Close ]", "[ Apply changes ]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("after chrome toggle missing %q:\n%s", want, view)
		}
	}
	assertActionInContentPanel(t, view, 120)

	m = mcpGotoAddButton(t, m)
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Smoke Custom")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	view = m.View()
	for _, want := range []string{"Smoke Custom", "[ Close ]", "[ Apply changes ]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("after custom add missing %q:\n%s", want, view)
		}
	}
}

func TestConfigureApplyChangesPersistsMCP(t *testing.T) {
	root := t.TempDir()
	writePersistedAtlasConfig(t, root, func(doc *config.ProjectDocument) {
		doc.MCP.Builtins.Jira.Enabled = true
		doc.MCP.Builtins.Context7.Enabled = false
		doc.MCP.Builtins.ChromeDevTools.Enabled = false
	})

	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = gotoConfigSection(t, m, "mcp")
	m = mustModel(m.Update(key("enter")))
	if !strings.Contains(m.View(), "[ Apply changes ]") {
		t.Fatalf("Apply changes missing on MCP open:\n%s", m.View())
	}
	if !m.MCPDraft().Builtins[0].Enabled {
		t.Fatal("jira should load enabled")
	}
	m = mustModel(m.Update(key("enter"))) // disable jira
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("enter"))) // enable context7
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("enter"))) // enable chrome
	m = mcpGotoAddButton(t, m)
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Ops Docs")}))
	m = mustModel(m.Update(key("down"))) // transport
	m = mustModel(m.Update(key("down"))) // http
	m = mustModel(m.Update(key("enter")))
	m = mcpGotoAddField(t, m, screens.MCPFocusConn)
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://ops.example/mcp")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if len(m.MCPDraft().CustomServers) != 1 {
		t.Fatalf("custom = %#v", m.MCPDraft().CustomServers)
	}
	// enable custom
	if m.MCPListFocus() != screens.MCPFocusCustom {
		for i := 0; i < 8 && m.MCPListFocus() != screens.MCPFocusCustom; i++ {
			m = mustModel(m.Update(key("down")))
		}
	}
	m = mustModel(m.Update(key("enter")))

	m = gotoInitFooterAction(t, m, true) // Apply changes
	m = mustModel(m.Update(key("enter")))
	if m.ConfigureNotice() != "Configuration changes saved." {
		t.Fatalf("notice = %q", m.ConfigureNotice())
	}
	if !strings.Contains(m.View(), "Configuration changes saved.") {
		t.Fatalf("missing saved notice in view:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "[ Apply changes ]") || !strings.Contains(m.View(), "[ Close ]") {
		t.Fatalf("missing configure actions:\n%s", m.View())
	}

	raw, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"jira:",
		"context7:",
		"chrome_devtools:",
		"Ops Docs",
		"transport: http",
		"command_or_url: https://ops.example/mcp",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("persisted config missing %q:\n%s", want, text)
		}
	}
	for _, banned := range []string{"password", "token", "secret", "api_key"} {
		if strings.Contains(strings.ToLower(text), banned) {
			t.Fatalf("credentials leaked %q:\n%s", banned, text)
		}
	}
	doc, err := config.LoadProjectDocument(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.MCP.Builtins.Jira.Enabled {
		t.Fatal("jira should be persisted disabled")
	}
	if !doc.MCP.Builtins.Context7.Enabled || !doc.MCP.Builtins.ChromeDevTools.Enabled {
		t.Fatalf("builtins = %#v", doc.MCP.Builtins)
	}
	if len(doc.MCP.Custom) != 1 || !doc.MCP.Custom[0].Enabled || doc.MCP.Custom[0].Name != "Ops Docs" {
		t.Fatalf("custom = %#v", doc.MCP.Custom)
	}

	assertRuntimeNotMaterialized(t, root)
	for _, rel := range []string{"AGENTS.md", ".cursor", ".opencode", ".agents", ".claude", "CLAUDE.md", "GEMINI.md", "README.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist", rel)
		}
	}

	// Reopen Configure — must load saved MCP.
	m, cmd := apply(m, key("b"))
	m = applyCmd(t, m, cmd)
	m = loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	if m.MCPDraft().Builtins[0].Enabled {
		t.Fatal("reopen: jira should stay disabled")
	}
	if !m.MCPDraft().Builtins[1].Enabled || !m.MCPDraft().Builtins[2].Enabled {
		t.Fatalf("reopen builtins = %#v", m.MCPDraft().Builtins)
	}
	if len(m.MCPDraft().CustomServers) != 1 || m.MCPDraft().CustomServers[0].Name != "Ops Docs" {
		t.Fatalf("reopen custom = %#v", m.MCPDraft().CustomServers)
	}
	m = gotoConfigSection(t, m, "mcp")
	view := m.View()
	for _, want := range []string{"[ ] Jira", "[x] Context7", "[x] Chrome DevTools", "Ops Docs"} {
		if !strings.Contains(view, want) {
			t.Fatalf("reopen view missing %q:\n%s", want, view)
		}
	}
}

func mcpGotoAddButton(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	if m.Focus() != tui.FocusContent {
		m = mustModel(m.Update(key("tab")))
	}
	for i := 0; i < 16 && m.MCPListFocus() != screens.MCPFocusAddBtn; i++ {
		m = mustModel(m.Update(key("down")))
	}
	if m.MCPListFocus() != screens.MCPFocusAddBtn {
		t.Fatalf("could not reach Add MCP, focus=%q", m.MCPListFocus())
	}
	return m
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

func mcpGotoAddField(t *testing.T, m tui.Model, focus string) tui.Model {
	t.Helper()
	for i := 0; i < 12 && m.MCPAddFocus() != focus; i++ {
		m = mustModel(m.Update(key("down")))
	}
	if m.MCPAddFocus() != focus {
		t.Fatalf("could not reach add field %q, focus=%q", focus, m.MCPAddFocus())
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

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func isShellActionLine(vis string) bool {
	has := func(s string) bool { return strings.Contains(vis, s) }
	switch {
	case has("[ Back ]") && has("[ Next ]"):
		return true
	case has("[ Back ]") && has("[ Apply config ]"):
		return true
	case has("[ Close ]") && has("[ Apply changes ]"):
		return true
	case has("[ Add MCP ]") && has("[ Close ]"):
		return true
	case has("[ Cancel ]") && has("[ Add ]"):
		return true
	case has("[ Close ]") && !has("[ Add MCP ]") && !has("[ Apply changes ]"):
		return true
	case has("[ Next ]") && !has("[ Back ]"):
		return true
	default:
		return false
	}
}

func assertActionInContentPanel(t *testing.T, view string, totalWidth int) {
	t.Helper()
	side := 20
	if totalWidth >= 100 {
		side = 24
	}
	found := false
	for _, line := range strings.Split(view, "\n") {
		vis := stripANSI(line)
		if !isShellActionLine(vis) {
			continue
		}
		found = true
		col := -1
		for _, tok := range []string{"[ Back ]", "[ Close ]", "[ Add MCP ]", "[ Cancel ]", "[ Next ]", "[ Apply changes ]", "[ Apply config ]"} {
			if i := strings.Index(vis, tok); i >= 0 && (col < 0 || i < col) {
				col = i
			}
		}
		if col < side {
			t.Fatalf("action row starts at visual col %d, want >= %d (right panel, not shell edge):\n%s", col, side, vis)
		}
		backIdx := strings.Index(vis, "[ Back ]")
		if backIdx < 0 {
			backIdx = strings.Index(vis, "[ Close ]")
		}
		nextIdx := strings.Index(vis, "[ Next ]")
		if nextIdx < 0 {
			nextIdx = strings.Index(vis, "[ Apply config ]")
		}
		if nextIdx < 0 {
			nextIdx = strings.Index(vis, "[ Apply changes ]")
		}
		if backIdx >= 0 && nextIdx >= 0 && backIdx > nextIdx {
			t.Fatalf("Back must appear before Next/Apply:\n%s", vis)
		}
	}
	if !found {
		t.Fatalf("missing content-panel action row:\n%s", view)
	}
}

func assertActionNearFooter(t *testing.T, view string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	actionLine := -1
	footerLine := -1
	for i, line := range lines {
		vis := stripANSI(line)
		if isShellActionLine(vis) {
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
	if actionLine > 0 {
		prev := strings.TrimSpace(strings.Trim(stripANSI(lines[actionLine-1]), "│"))
		if prev != "" {
			t.Fatalf("expected a blank line between content and action row:\n%s", view)
		}
	}
	blanks := 0
	for _, line := range lines[actionLine+1 : footerLine] {
		trimmed := strings.TrimSpace(strings.Trim(stripANSI(line), "│"))
		if trimmed == "" {
			blanks++
		}
	}
	if blanks < 1 && footerLine-actionLine < 2 {
		t.Fatalf("expected visual separation between action row and footer:\n%s", view)
	}
	if blanks > 3 {
		t.Fatalf("large blank block between action row and footer (%d blank lines):\n%s", blanks, view)
	}
	if footerLine-actionLine > 5 {
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
	assertActionInContentPanel(t, initM.View(), 120)
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
	assertActionInContentPanel(t, initM.View(), 120)
	assertGlobalTopGap(t, initM.View())
	for _, line := range strings.Split(initM.View(), "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("step 2 Action heading:\n%s", initM.View())
		}
	}

	initM = gotoInitFooterAction(t, initM, true)
	initM = mustModel(initM.Update(key("enter")))
	if initM.InitWizardStep() != screens.InitWizardStepReview {
		t.Fatalf("step = %d, want review", initM.InitWizardStep())
	}
	assertActionNearFooter(t, initM.View())
	assertActionInContentPanel(t, initM.View(), 120)
	assertGlobalTopGap(t, initM.View())
	for _, line := range strings.Split(initM.View(), "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("step 3 Action heading:\n%s", initM.View())
		}
	}

	writeValidAtlasConfig(t, root)
	cfg := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	cfg = mustModel(cfg.Update(tea.WindowSizeMsg{Width: 120, Height: 60}))
	assertActionNearFooter(t, cfg.View())
	assertActionInContentPanel(t, cfg.View(), 120)
	assertGlobalTopGap(t, cfg.View())
	assertShell(t, cfg.View(), "Configure")
	cfg = gotoConfigSection(t, cfg, "mcp")
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
	if m.Quitting() {
		t.Fatal("b must not quit")
	}
}

func TestGlobalQAndCtrlCQuit(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(root, "configured")
	if err := os.MkdirAll(configured, 0o755); err != nil {
		t.Fatal(err)
	}
	writeValidAtlasConfig(t, configured)

	type screen struct {
		name string
		load func(t *testing.T) tui.Model
	}
	screensToQuit := []screen{
		{"dashboard", func(t *testing.T) tui.Model {
			return loadWorkspace(t, tui.Options{Route: tui.RouteDashboard, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover})
		}},
		{"init-step-1", func(t *testing.T) tui.Model {
			return loadWorkspace(t, tui.Options{Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover})
		}},
		{"init-step-2", func(t *testing.T) tui.Model {
			m := loadWorkspace(t, tui.Options{Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover})
			return gotoInitStep2(t, m)
		}},
		{"review", func(t *testing.T) tui.Model {
			m := loadWorkspace(t, tui.Options{Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover})
			return gotoInitReview(t, m)
		}},
		{"configure", func(t *testing.T) tui.Model {
			return loadWorkspace(t, tui.Options{Route: tui.RouteConfigure, Getwd: func() (string, error) { return configured, nil }, Discover: workspace.Discover})
		}},
		{"configure-mcp", func(t *testing.T) tui.Model {
			m := loadWorkspace(t, tui.Options{Route: tui.RouteConfigure, Getwd: func() (string, error) { return configured, nil }, Discover: workspace.Discover})
			return gotoConfigSection(t, m, "mcp")
		}},
		{"status", func(t *testing.T) tui.Model {
			return loadWorkspace(t, tui.Options{Route: tui.RouteStatus, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover})
		}},
		{"doctor", func(t *testing.T) tui.Model {
			return loadWorkspace(t, tui.Options{Route: tui.RouteDoctor, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover})
		}},
		{"runtime-repair", func(t *testing.T) tui.Model {
			return loadWorkspace(t, tui.Options{Route: tui.RouteRuntimeRepair, Getwd: func() (string, error) { return configured, nil }, Discover: workspace.Discover})
		}},
		{"help", func(t *testing.T) tui.Model {
			return sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
		}},
		{"error", func(t *testing.T) tui.Model {
			return sized(tui.NewModel(tui.Options{Route: tui.RouteError, UnknownCommand: "start"}))
		}},
	}

	for _, sc := range screensToQuit {
		for _, k := range []tea.KeyMsg{key("q"), {Type: tea.KeyCtrlC}} {
			m := sc.load(t)
			route := m.Route()
			step := m.InitWizardStep()
			updated, cmd := m.Update(k)
			model := updated.(tui.Model)
			if !model.Quitting() {
				t.Fatalf("%s %v should quit", sc.name, k)
			}
			if model.Route() != route {
				t.Fatalf("%s %v routed to %v, want stay on %v", sc.name, k, model.Route(), route)
			}
			if model.InitWizardStep() != step {
				t.Fatalf("%s %v changed wizard step %d -> %d", sc.name, k, step, model.InitWizardStep())
			}
			if cmd == nil {
				t.Fatalf("%s %v expected quit cmd", sc.name, k)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("%s %v cmd is not tea.Quit", sc.name, k)
			}
		}
	}
}

func TestQDoesNotNavigateToDashboard(t *testing.T) {
	root := t.TempDir()
	writeValidAtlasConfig(t, root)
	for _, route := range []tui.Route{tui.RouteInitPlan, tui.RouteConfigure, tui.RouteStatus, tui.RouteHelp} {
		m := loadWorkspace(t, tui.Options{
			Route: route, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
		})
		if route == tui.RouteHelp {
			m = sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
		}
		if route == tui.RouteInitPlan {
			m = gotoInitStep2(t, m)
		}
		updated, _ := m.Update(key("q"))
		model := updated.(tui.Model)
		if !model.Quitting() {
			t.Fatalf("q on %v should quit", route)
		}
		if model.Route() == tui.DefaultRoute && route != tui.DefaultRoute {
			t.Fatalf("q on %v must not navigate to dashboard", route)
		}
	}

	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = gotoInitReview(t, m)
	updated, _ := m.Update(key("q"))
	model := updated.(tui.Model)
	if model.Route() != tui.RouteInitPlan || !model.Quitting() {
		t.Fatalf("q on review route=%v quitting=%v", model.Route(), model.Quitting())
	}
}

func TestQInsertsInTextInputs(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = mustModel(m.Update(key("tab")))
	before := m.DraftName()
	m = mustModel(m.Update(key("q")))
	if m.Quitting() {
		t.Fatal("q in project name must not quit")
	}
	if m.DraftName() != before+"q" {
		t.Fatalf("project name = %q, want %q", m.DraftName(), before+"q")
	}

	writeValidAtlasConfig(t, root)
	m = loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = gotoConfigSection(t, m, "mcp")
	m = mustModel(m.Update(key("enter")))
	m = mcpGotoAddButton(t, m)
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(key("q")))
	if m.Quitting() {
		t.Fatal("q in MCP name must not quit")
	}
	if m.MCPAddName() != "q" {
		t.Fatalf("mcp name = %q, want q", m.MCPAddName())
	}

	m = mcpGotoAddField(t, m, screens.MCPFocusConn)
	m = mustModel(m.Update(key("q")))
	if m.Quitting() {
		t.Fatal("q in MCP command/url must not quit")
	}
	if m.MCPAddConn() != "q" {
		t.Fatalf("mcp conn = %q, want q", m.MCPAddConn())
	}

	m = mcpGotoAddField(t, m, screens.MCPFocusArgs)
	m = mustModel(m.Update(key("q")))
	if m.Quitting() {
		t.Fatal("q in MCP args must not quit")
	}
	if m.MCPAddArgs() != "q" {
		t.Fatalf("mcp args = %q, want q", m.MCPAddArgs())
	}

	m = mcpGotoAddField(t, m, screens.MCPFocusEnv)
	m = mustModel(m.Update(key("q")))
	if m.Quitting() {
		t.Fatal("q in MCP env must not quit")
	}
	if m.MCPAddEnv() != "q" {
		t.Fatalf("mcp env = %q, want q", m.MCPAddEnv())
	}
}

func TestCtrlCQuitsWhileEditingText(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = mustModel(m.Update(key("tab")))
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model := updated.(tui.Model)
	if !model.Quitting() || cmd == nil {
		t.Fatal("ctrl+c while editing project name should quit")
	}

	writeValidAtlasConfig(t, root)
	m = loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = gotoConfigSection(t, m, "mcp")
	m = mustModel(m.Update(key("enter")))
	m = mcpGotoAddButton(t, m)
	m = mustModel(m.Update(key("enter")))
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tui.Model)
	if !model.Quitting() || cmd == nil {
		t.Fatal("ctrl+c while editing MCP name should quit")
	}
}

func TestEscBackCloseUnchanged(t *testing.T) {
	root := t.TempDir()
	writeValidAtlasConfig(t, root)
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m, cmd := apply(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = applyCmd(t, m, cmd)
	if m.Route() != tui.DefaultRoute || m.Quitting() {
		t.Fatalf("esc from Configure route=%v quitting=%v", m.Route(), m.Quitting())
	}

	m = loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = gotoInitFooterAction(t, m, false)
	m, cmd = apply(m, key("enter"))
	m = applyCmd(t, m, cmd)
	if m.Route() != tui.DefaultRoute || m.Quitting() {
		t.Fatalf("Configure Close route=%v quitting=%v", m.Route(), m.Quitting())
	}

	m = loadWorkspace(t, tui.Options{
		Route: tui.RouteInitPlan, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	m = gotoInitStep2(t, m)
	m = gotoInitFooterAction(t, m, false)
	m = mustModel(m.Update(key("enter")))
	if m.InitWizardStep() != screens.InitWizardStepProject {
		t.Fatalf("Back should return to step 1, got %d", m.InitWizardStep())
	}
	if m.Quitting() {
		t.Fatal("Back must not quit")
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
	for _, want := range []string{"Atlas Runtime", "Technologies", "Libraries", "Bubble Tea", "runtime_materialized"} {
		if !strings.Contains(status.View(), want) {
			t.Fatalf("status missing %q:\n%s", want, status.View())
		}
	}
	assertNoMutation(t, root)

	doc := loadWorkspace(t, tui.Options{
		Route: tui.RouteDoctor, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	assertShell(t, doc.View(), "Atlas Doctor")
	assertNoMutation(t, root)

	help := sized(tui.NewModel(tui.Options{Route: tui.RouteHelp}))
	assertShell(t, help.View(), "Atlas Help")
}

func TestStatusDoctorRuntimeHealthNoMutation(t *testing.T) {
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:    "demo",
		ProjectMode:    "existing",
		DefaultRemote:  "origin",
		CursorDetected: true,
	})
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	before := snapshotDir(t, root)

	status := loadWorkspace(t, tui.Options{
		Route: tui.RouteStatus, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	view := status.View()
	for _, want := range []string{
		"Atlas Runtime",
		"Initialized",
		"runtime_materialized",
		".cursor/rules/atlas.mdc",
		"Context Graph",
		"AGENTS markers",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("status missing %q:\n%s", want, view)
		}
	}
	assertSnapshotUnchanged(t, root, before)

	doc := loadWorkspace(t, tui.Options{
		Route: tui.RouteDoctor, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	docView := doc.View()
	for _, want := range []string{"PASS", "atlas config", "agents markers", "adapter projection cursor"} {
		if !strings.Contains(docView, want) {
			t.Fatalf("doctor missing %q:\n%s", want, docView)
		}
	}
	if strings.Contains(docView, "FAIL") {
		t.Fatalf("doctor unexpected FAIL:\n%s", docView)
	}
	assertSnapshotUnchanged(t, root, before)
}

func TestRuntimeRepairTUIApplyAndNoAutoMutation(t *testing.T) {
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:    "demo",
		ProjectMode:    "existing",
		DefaultRemote:  "origin",
		CursorDetected: true,
	})
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	statusBefore := snapshotDir(t, root)
	status := loadWorkspace(t, tui.Options{
		Route: tui.RouteStatus, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	if !strings.Contains(status.View(), "Atlas Status") {
		t.Fatal("status")
	}
	assertSnapshotUnchanged(t, root, statusBefore)

	doc := loadWorkspace(t, tui.Options{
		Route: tui.RouteDoctor, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	if !strings.Contains(doc.View(), "FAIL") {
		t.Fatalf("doctor should fail missing AGENTS:\n%s", doc.View())
	}
	assertSnapshotUnchanged(t, root, statusBefore)

	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteRuntimeRepair, Getwd: func() (string, error) { return root, nil }, Discover: workspace.Discover,
	})
	view := m.View()
	for _, want := range []string{"Runtime Repair", "AGENTS.md", "Apply repair"} {
		if !strings.Contains(view, want) {
			t.Fatalf("repair missing %q:\n%s", want, view)
		}
	}
	m = mustModel(m.Update(key("tab")))
	m = mustModel(m.Update(key("right")))
	m = mustModel(m.Update(key("enter")))
	if !m.RepairApplied() {
		t.Fatalf("expected apply, message=%q plan=%#v", m.RepairMessage(), m.RepairPlan())
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
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
		"Context",
		"MCP",
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
	if len(selector) != 6 {
		t.Fatalf("selector sections = %d, want 6", len(selector))
	}
	if selector[4].Key != "context" || selector[5].Key != "mcp" {
		t.Fatalf("sections = %#v", selector)
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

	// Context Graph is a single checkbox; default enabled; Space/Enter toggles.
	m = mustModel(m.Update(key("left")))
	m = gotoConfigSection(t, m, "context")
	m = mustModel(m.Update(key("enter")))
	graph, _ := m.ConfigDraft().FieldByKey("context.graph.enabled")
	if graph.Value != "true" {
		t.Fatalf("default context graph = %q", graph.Value)
	}
	if !strings.Contains(m.View(), "[x] Enable Context Graph") {
		t.Fatalf("context checkbox missing:\n%s", m.View())
	}
	m = mustModel(m.Update(key("enter")))
	graph, _ = m.ConfigDraft().FieldByKey("context.graph.enabled")
	if graph.Value != "false" {
		t.Fatalf("context graph after toggle = %q", graph.Value)
	}
	m = mustModel(m.Update(key("enter"))) // re-enable for later review expectations
	graph, _ = m.ConfigDraft().FieldByKey("context.graph.enabled")
	if graph.Value != "true" {
		t.Fatalf("context graph re-enabled = %q", graph.Value)
	}

	// Locked project.name cannot change.
	draft := m.ConfigDraft()
	if draft.SelectOption("project.name", "hacked") {
		t.Fatal("project.name must not be editable")
	}

	// Move to footer Next and open Step 3 Review.
	m = gotoInitFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if m.InitWizardStep() != screens.InitWizardStepReview {
		t.Fatalf("step = %d, want review", m.InitWizardStep())
	}
	view = m.View()
	for _, want := range []string{
		"Review / Materialization Plan",
		"Apply writes .atlas/ config and compact runtime gateway files.",
		"[ Back ]",
		"[ Apply config ]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}

	m = mustModel(m.Update(key("enter"))) // Back is default focus
	if m.InitWizardStep() != screens.InitWizardStepConfig {
		t.Fatalf("back step = %d, want config", m.InitWizardStep())
	}
	if !strings.Contains(m.View(), "Step 2 — Initial Configuration") {
		t.Fatalf("expected step 2 after review back:\n%s", m.View())
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

func gotoInitStep2(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	if m.Focus() != tui.FocusContent {
		m = mustModel(m.Update(key("tab")))
	}
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
	return m
}

func gotoInitFooterAction(t *testing.T, m tui.Model, next bool) tui.Model {
	t.Helper()
	if m.Focus() != tui.FocusContent {
		m = mustModel(m.Update(key("tab")))
	}
	if m.ConfigPanel() == screens.ConfigPanelSections {
		m = mustModel(m.Update(key("enter")))
	}
	for m.ConfigPanel() != screens.ConfigPanelFooter {
		prevPanel := m.ConfigPanel()
		prevField := m.ConfigFieldIndex()
		prevOpt := m.ConfigOptionIndex()
		prevFooter := m.ConfigFooterIndex()
		prevMCP := m.MCPListFocus()
		prevMode := m.MCPMode()
		m = mustModel(m.Update(key("down")))
		if m.ConfigPanel() == prevPanel &&
			m.ConfigFieldIndex() == prevField &&
			m.ConfigOptionIndex() == prevOpt &&
			m.ConfigFooterIndex() == prevFooter &&
			m.MCPListFocus() == prevMCP &&
			m.MCPMode() == prevMode {
			t.Fatal("could not reach footer")
		}
	}
	if next {
		if m.ConfigFooterIndex() != 1 {
			m = mustModel(m.Update(key("right")))
		}
	} else if m.ConfigFooterIndex() != 0 {
		m = mustModel(m.Update(key("left")))
	}
	return m
}

func gotoInitReview(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	if m.InitWizardStep() == screens.InitWizardStepProject {
		m = gotoInitStep2(t, m)
	}
	if m.InitWizardStep() == screens.InitWizardStepConfig {
		m = gotoInitFooterAction(t, m, true)
		m = mustModel(m.Update(key("enter")))
	}
	if m.InitWizardStep() != screens.InitWizardStepReview {
		t.Fatalf("step = %d, want review", m.InitWizardStep())
	}
	return m
}

func reviewVisibleText(t *testing.T, m tui.Model) (tui.Model, string) {
	t.Helper()
	top := m.View()
	m = mustModel(m.Update(key("end")))
	bottom := m.View()
	m = mustModel(m.Update(key("home")))
	return m, top + "\n" + bottom
}

func TestInitReviewPlanContentAndApply(t *testing.T) {
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
		t.Fatalf("start step = %d, want project", m.InitWizardStep())
	}
	m = gotoInitReview(t, m)
	m, view := reviewVisibleText(t, m)
	for _, want := range []string{
		"Review / Materialization Plan",
		"Name: " + m.DraftName(),
		"Mode: Existing project",
		"Workflow: SDD",
		"Spec engine: OpenSpec",
		"Adapters: none",
		"Source control: None",
		"Branch strategy: Manual",
		"Atlas governance files: Local only",
		"Memory strategy: SQLite + Context Capsule",
		"Context Graph: Enabled",
		"MCP integrations: 0 configured",
		".atlas/config.yaml",
		".atlas/local.yaml",
		".atlas/state.yaml",
		".atlas/assets.lock.yaml",
		".atlas/backups/",
		"AGENTS.md",
		"No existing runtime artifacts detected.",
		"No backups required.",
		"No existing runtime files need replacement.",
		"Existing project source files are preserved.",
		"README.md is preserved",
		"Git history is not modified.",
		"No commits are created.",
		"No branches are created.",
		"No remote operations are performed.",
		"Secrets and credentials are not stored.",
		"Apply writes .atlas/ config and compact runtime gateway files.",
		"create/update this slice",
		"[ Back ]",
		"[ Apply config ]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("review missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "planned for later") {
		t.Fatalf("review must not say planned for later:\n%s", view)
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.TrimSpace(line) == "Action" {
			t.Fatalf("step 3 must not render Action heading:\n%s", m.View())
		}
	}
	assertActionNearFooter(t, m.View())
	assertGlobalTopGap(t, m.View())
	assertShell(t, m.View(), "Review / Materialization Plan")

	m = mustModel(m.Update(key("right")))
	m = mustModel(m.Update(key("enter")))
	if !m.InitApplied() {
		t.Fatalf("expected apply success, message = %q", m.InitReviewMessage())
	}
	if !strings.Contains(m.View(), "Atlas configuration and runtime initialized.") {
		t.Fatalf("missing success title:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "Runtime files materialized.") {
		t.Fatalf("missing success body:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "[ Close ]") {
		t.Fatalf("missing Close after apply:\n%s", m.View())
	}
	if strings.Contains(m.View(), "[ Apply config ]") {
		t.Fatalf("Apply config should be gone after success:\n%s", m.View())
	}
	assertAtlasConfigPersisted(t, root)
	assertAgentsMaterialized(t, root)
	assertForbiddenRuntimeAbsent(t, root)
	assertMissingPath(t, root, ".cursor")
	assertMissingPath(t, root, ".opencode")

	updated, cmd := m.Update(key("enter"))
	m = applyCmd(t, updated.(tui.Model), cmd)
	if m.Route() != tui.RouteDashboard {
		t.Fatalf("close route = %s, want Dashboard", m.Route())
	}
	if !m.Initialized() {
		t.Fatal("dashboard should see initialized atlas config")
	}
}

func TestInitReviewArtifactsAndAdapters(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# agents"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	m = gotoInitStep2(t, m)
	m = gotoConfigSection(t, m, "adapters")
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(key("enter")))
	m = gotoInitReview(t, m)
	_, view := reviewVisibleText(t, m)
	for _, want := range []string{
		"Existing runtime artifacts detected:",
		"AGENTS.md",
		".atlas/backups/<timestamp>/",
		".atlas/backups/<timestamp>/AGENTS.md",
		"Atlas will replace runtime targets after backup.",
		".cursor/rules/atlas.mdc",
		"Adapters: Cursor",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("artifact review missing %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas must not be created, stat err = %v", err)
	}

	m = mustModel(m.Update(key("right")))
	m = mustModel(m.Update(key("enter")))
	if !m.InitApplied() {
		t.Fatal("expected apply")
	}
	assertAtlasConfigPersisted(t, root)
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "<!-- ATLAS:MANAGED:BEGIN -->") {
		t.Fatalf("AGENTS.md not materialized:\n%s", agents)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "rules", "atlas.mdc")); err != nil {
		t.Fatalf("cursor rule missing: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".atlas", "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one backup timestamp dir, entries=%v", entries)
	}
	backupAgents, err := os.ReadFile(filepath.Join(root, ".atlas", "backups", entries[0].Name(), "AGENTS.md"))
	if err != nil || string(backupAgents) != "# agents" {
		t.Fatalf("backup AGENTS.md = %q err=%v", backupAgents, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "backups", entries[0].Name(), "manifest.json")); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}

func TestInitCancelDoesNotOpenReview(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	m = mustModel(m.Update(key("tab")))
	for m.InitField() != screens.InitFieldDecisionCancel {
		prev := m.InitField()
		m = mustModel(m.Update(key("down")))
		if m.InitField() == prev {
			t.Fatal("could not reach cancel")
		}
	}
	m = mustModel(m.Update(key("enter")))
	m = gotoInitStep2(t, m)
	m = gotoInitFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if m.InitWizardStep() != screens.InitWizardStepConfig {
		t.Fatalf("canceled init opened step %d", m.InitWizardStep())
	}
	if strings.Contains(m.View(), "Step 3 — Review") {
		t.Fatalf("canceled init must not open review:\n%s", m.View())
	}
	assertNoMutation(t, root)
}

func TestInitReviewSummarizesSessionMCP(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	m = gotoInitStep2(t, m)
	m = gotoConfigSection(t, m, "mcp")
	m = mustModel(m.Update(key("enter")))
	m = mcpGotoAddButton(t, m)
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Jira Main")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if len(m.MCPDraft().CustomServers) != 1 {
		t.Fatalf("custom = %#v", m.MCPDraft().CustomServers)
	}
	m = gotoInitReview(t, m)
	_, view := reviewVisibleText(t, m)
	for _, want := range []string{
		"MCP integrations: 1 configured",
		"Jira Main",
		"kind: custom",
		"transport: stdio",
		"status: will persist in .atlas/config.yaml",
		"No MCP credentials, connections, or validation are implemented",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("mcp review missing %q:\n%s", want, view)
		}
	}
	assertNoMutation(t, root)
}

func TestInitReviewListsSelectedBuiltins(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	m = gotoInitStep2(t, m)
	m = gotoConfigSection(t, m, "mcp")
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(key("enter"))) // Jira
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("enter"))) // Context7
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("enter"))) // Chrome
	if m.MCPDraft().SelectedBuiltinCount() != 3 {
		t.Fatalf("selected builtins = %d", m.MCPDraft().SelectedBuiltinCount())
	}
	m = gotoInitReview(t, m)
	_, view := reviewVisibleText(t, m)
	for _, want := range []string{
		"MCP integrations: 3 configured",
		"- Jira",
		"- Context7",
		"- Chrome DevTools",
		"kind: built-in",
		"enabled: true",
		"status: will persist in .atlas/config.yaml",
		"No MCP credentials, connections, or validation are implemented",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("builtin review missing %q:\n%s", want, view)
		}
	}
	assertNoMutation(t, root)
}

func TestInitStep2MCPSectionAndAdd(t *testing.T) {
	root := t.TempDir()
	m := loadWorkspace(t, tui.Options{
		Route:    tui.RouteInitPlan,
		Getwd:    func() (string, error) { return root, nil },
		Discover: workspace.Discover,
	})
	m = gotoInitStep2(t, m)
	m = gotoConfigSection(t, m, "mcp")
	view := m.View()
	for _, want := range []string{
		"MCP",
		"Built-in MCPs",
		"[ ] Jira",
		"[ ] Context7",
		"[ ] Chrome DevTools",
		"[ Add MCP ]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("init mcp missing %q:\n%s", want, view)
		}
	}

	m = mustModel(m.Update(key("enter"))) // enter MCP fields; arrows must not toggle
	jira := m.MCPDraft().Builtins[0].Enabled
	ctx := m.MCPDraft().Builtins[1].Enabled
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("up")))
	if m.MCPDraft().Builtins[0].Enabled != jira || m.MCPDraft().Builtins[1].Enabled != ctx {
		t.Fatal("arrows mutated init mcp")
	}
	m = mustModel(m.Update(key("enter"))) // Jira
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("enter"))) // Context7
	m = mustModel(m.Update(key("down")))
	m = mustModel(m.Update(key("enter"))) // Chrome
	if !m.MCPDraft().Builtins[0].Enabled || !m.MCPDraft().Builtins[1].Enabled || !m.MCPDraft().Builtins[2].Enabled {
		t.Fatalf("expected all built-ins: %#v", m.MCPDraft().Builtins)
	}

	for i := 0; i < 8 && m.MCPListFocus() != screens.MCPFocusAddBtn; i++ {
		m = mustModel(m.Update(key("down")))
	}
	if m.MCPListFocus() != screens.MCPFocusAddBtn {
		t.Fatalf("focus = %q, want add", m.MCPListFocus())
	}
	m = mustModel(m.Update(key("enter")))
	if m.MCPMode() != screens.MCPModeAdd {
		t.Fatalf("mode = %q", m.MCPMode())
	}
	addView := m.View()
	for _, want := range []string{"Add MCP", "Name", "Transport", "Command or URL", "Arguments", "Environment references", "[ Cancel ]", "[ Add ]"} {
		if !strings.Contains(addView, want) {
			t.Fatalf("init add missing %q:\n%s", want, addView)
		}
	}
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if m.MCPAddError() == "" {
		t.Fatal("empty name should fail in init add")
	}
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Init Browser")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if len(m.MCPDraft().CustomServers) != 1 || m.MCPDraft().CustomServers[0].Name != "Init Browser" {
		t.Fatalf("custom = %#v", m.MCPDraft().CustomServers)
	}
	if m.InitWizardStep() != screens.InitWizardStepConfig {
		t.Fatalf("should stay on step 2, got %d", m.InitWizardStep())
	}

	for i := 0; i < 8 && m.MCPListFocus() != screens.MCPFocusAddBtn; i++ {
		m = mustModel(m.Update(key("down")))
	}
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("init browser")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))
	if m.MCPAddError() == "" {
		t.Fatal("duplicate name should fail")
	}
	m = mcpGotoFooterAction(t, m, false)
	m = mustModel(m.Update(key("enter"))) // Cancel
	if m.MCPMode() != screens.MCPModeList {
		t.Fatalf("cancel should return to list, mode=%q", m.MCPMode())
	}

	if m.MCPListFocus() != screens.MCPFocusCustom {
		for i := 0; i < 8 && m.MCPListFocus() != screens.MCPFocusCustom; i++ {
			m = mustModel(m.Update(key("down")))
		}
	}
	m = mustModel(m.Update(key("d")))
	if len(m.MCPDraft().CustomServers) != 0 {
		t.Fatalf("custom still present: %#v", m.MCPDraft().CustomServers)
	}
	if !strings.Contains(m.View(), "Removed Init Browser") {
		t.Fatalf("missing remove notice:\n%s", m.View())
	}
	if !m.MCPDraft().Builtins[0].Enabled {
		t.Fatal("removing custom must not disable built-ins")
	}

	m = mcpGotoAddButton(t, m)
	m = mustModel(m.Update(key("enter")))
	m = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Init Browser")}))
	m = mcpGotoFooterAction(t, m, true)
	m = mustModel(m.Update(key("enter")))

	m = gotoInitReview(t, m)
	_, review := reviewVisibleText(t, m)
	for _, want := range []string{
		"Init Browser",
		"kind: custom",
		"kind: built-in",
		"Jira",
		"Context7",
		"Chrome DevTools",
	} {
		if !strings.Contains(review, want) {
			t.Fatalf("review missing %q:\n%s", want, review)
		}
	}

	m = mustModel(m.Update(key("right")))
	m = mustModel(m.Update(key("enter")))
	if !m.InitApplied() {
		t.Fatalf("apply failed: %q", m.InitReviewMessage())
	}
	assertAtlasConfigPersisted(t, root)
	cfgRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(cfgRaw)
	for _, want := range []string{"Init Browser", "jira:", "context7:", "chrome_devtools:", "enabled: true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("persisted config missing %q:\n%s", want, text)
		}
	}
	assertAgentsMaterialized(t, root)
	assertForbiddenRuntimeAbsent(t, root)
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
	for _, want := range []string{"Atlas", "Dashboard", "Status", "Doctor", "Help", "Exit", contentTitle} {
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
	writePersistedAtlasConfig(t, root, nil)
}

func writePersistedAtlasConfig(t *testing.T, root string, mutate func(*config.ProjectDocument)) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "demo",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	})
	doc := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	if mutate != nil {
		mutate(&doc)
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".atlas", "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertAtlasConfigPersisted(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		".atlas/config.yaml",
		".atlas/local.yaml",
		".atlas/state.yaml",
		".atlas/assets.lock.yaml",
		".atlas/backups",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
	stateRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stateRaw), "runtime_materialized: true") {
		t.Fatalf("state missing runtime_materialized true:\n%s", stateRaw)
	}
	if !strings.Contains(string(stateRaw), "runtime_materialized_at:") {
		t.Fatalf("state missing runtime_materialized_at:\n%s", stateRaw)
	}
}

func assertAgentsMaterialized(t *testing.T, root string) {
	t.Helper()
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md missing: %v", err)
	}
	for _, want := range []string{
		"<!-- ATLAS:MANAGED:BEGIN -->",
		"<!-- ATLAS:MANAGED:END -->",
		"<!-- ATLAS:USER:BEGIN -->",
		"<!-- ATLAS:USER:END -->",
	} {
		if !strings.Contains(string(agents), want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, agents)
		}
	}
}

func assertForbiddenRuntimeAbsent(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		".agents",
		".claude",
		"CLAUDE.md",
		"GEMINI.md",
		filepath.Join(".atlas", "memory", "atlas.sqlite"),
		filepath.Join(".atlas", "context", "memory-capsule.md"),
	} {
		assertMissingPath(t, root, rel)
	}
}

func assertMissingPath(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist, err=%v", rel, err)
	}
}

// assertRuntimeNotMaterialized asserts Configure/non-init paths did not create runtime files.
func assertRuntimeNotMaterialized(t *testing.T, root string) {
	t.Helper()
	assertMissingPath(t, root, "AGENTS.md")
	assertMissingPath(t, root, ".cursor")
	assertMissingPath(t, root, ".opencode")
	assertForbiddenRuntimeAbsent(t, root)
}

func assertNoMutation(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf(".atlas should not be created, stat err = %v", err)
	}
}

func snapshotDir(t *testing.T, root string) map[string]string {
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
			out[filepath.ToSlash(rel)+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertSnapshotUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotDir(t, root)
	if len(before) != len(after) {
		t.Fatalf("tree size changed: before=%d after=%d", len(before), len(after))
	}
	for path, content := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("path removed: %s", path)
		}
		if got != content {
			t.Fatalf("path mutated: %s", path)
		}
	}
}
