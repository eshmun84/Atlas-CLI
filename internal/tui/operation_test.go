package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
)

func TestConfigureApply_ShowsLoaderAndBlocksDuplicate(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writePersistedAtlasConfig(t, root, func(doc *config.ProjectDocument) {
		doc.Adapters.Selected = []string{"cursor"}
	})

	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: inspect.Inspect,
	})
	m = gotoInitFooterAction(t, m, true)

	updated, cmd := apply(m, key("enter"))
	m = updated
	if !m.OperationActive() {
		t.Fatal("expected operation active immediately after Apply")
	}
	view := m.View()
	if !strings.Contains(view, "Applying configuration") {
		t.Fatalf("loader missing:\n%s", view)
	}
	if !strings.Contains(view, "Current operation:") {
		t.Fatalf("current operation detail missing:\n%s", view)
	}

	// Duplicate Enter must be ignored while active.
	m2, cmd2 := apply(m, key("enter"))
	if cmd2 != nil {
		t.Fatal("duplicate Apply must not start a second command")
	}
	if !m2.OperationActive() {
		t.Fatal("operation should remain active")
	}

	m = applyCmd(t, m, cmd)
	if m.OperationActive() {
		t.Fatal("operation should clear after completion")
	}
	if !strings.Contains(m.ConfigureNotice(), "Configuration applied successfully") {
		t.Fatalf("success notice missing:\n%s", m.ConfigureNotice())
	}
	if !strings.Contains(m.ConfigureNotice(), "Completed in") {
		t.Fatalf("duration missing:\n%s", m.ConfigureNotice())
	}
}

func TestConfigureApply_DuplicateShortcutIgnored(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writePersistedAtlasConfig(t, root, func(doc *config.ProjectDocument) {
		doc.Adapters.Selected = []string{"cursor"}
	})
	m := loadWorkspace(t, tui.Options{
		Route: tui.RouteConfigure, Getwd: func() (string, error) { return root, nil }, Discover: inspect.Inspect,
	})
	m = gotoInitFooterAction(t, m, true)
	updated, cmd := apply(m, key("enter"))
	m = updated
	if !m.OperationActive() {
		t.Fatal("expected active op")
	}
	for _, k := range []string{"enter", " ", "space"} {
		m2, c := apply(m, key(k))
		if c != nil {
			t.Fatalf("shortcut %q started work while busy", k)
		}
		if !m2.OperationActive() {
			t.Fatalf("shortcut %q cleared operation", k)
		}
	}
	m = applyCmd(t, m, cmd)
	if m.OperationActive() {
		t.Fatal("expected clear")
	}
}
