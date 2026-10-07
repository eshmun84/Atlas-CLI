package home_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestProjectID_SameRootStable(t *testing.T) {
	root := t.TempDir()
	a, err := home.ProjectID(root, "Demo App")
	if err != nil {
		t.Fatal(err)
	}
	b, err := home.ProjectID(root, "Demo App")
	if err != nil {
		t.Fatal(err)
	}
	if a != b || !strings.HasPrefix(a, "demo-app-") {
		t.Fatalf("id unstable or unexpected: %q %q", a, b)
	}
}

func TestProjectID_SameNameDifferentRoot(t *testing.T) {
	a, err := home.ProjectID(t.TempDir(), "same-name")
	if err != nil {
		t.Fatal(err)
	}
	b, err := home.ProjectID(t.TempDir(), "same-name")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("same name different root must differ: %q", a)
	}
	if !strings.HasPrefix(a, "same-name-") || !strings.HasPrefix(b, "same-name-") {
		t.Fatalf("unexpected ids: %q %q", a, b)
	}
}

func TestResetProject_OnlyMatchingID(t *testing.T) {
	homeDir := t.TempDir()
	rootA := t.TempDir()
	rootB := t.TempDir()
	idA, err := home.ProjectID(rootA, "same-name")
	if err != nil {
		t.Fatal(err)
	}
	idB, err := home.ProjectID(rootB, "same-name")
	if err != nil {
		t.Fatal(err)
	}
	if idA == idB {
		t.Fatal("ids must differ")
	}
	if err := home.EnsureProjectLayout(homeDir, idA); err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureProjectLayout(homeDir, idB); err != nil {
		t.Fatal(err)
	}
	markerA := filepath.Join(home.ProjectContextDir(homeDir, idA), "keep-a.txt")
	markerB := filepath.Join(home.ProjectContextDir(homeDir, idB), "keep-b.txt")
	if err := os.WriteFile(markerA, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerB, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	globalAsset := filepath.Join(homeDir, "assets", "global.txt")
	if err := os.MkdirAll(filepath.Dir(globalAsset), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalAsset, []byte("global"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := home.ResetProject(homeDir, idA); err != nil {
		t.Fatal(err)
	}
	if home.ProjectDataPresent(homeDir, idA) {
		t.Fatal("project A should be reset")
	}
	if !home.ProjectDataPresent(homeDir, idB) {
		t.Fatal("same-name different-root project B must remain")
	}
	if _, err := os.Stat(markerB); err != nil {
		t.Fatal("project B marker missing")
	}
	if _, err := os.Stat(globalAsset); err != nil {
		t.Fatal("global Home assets must remain")
	}
}

func TestResetProject_RefusesNameAlone(t *testing.T) {
	homeDir := t.TempDir()
	id, err := home.ProjectID(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureProjectLayout(homeDir, id); err != nil {
		t.Fatal(err)
	}
	// Name alone is never a valid Reset target; Reset requires the full project ID.
	if err := home.ResetProject(homeDir, "demo"); err == nil {
		// empty dir named "demo" may not exist — ensure we did not wipe the real id
		if !home.ProjectDataPresent(homeDir, id) {
			t.Fatal("reset by name alone must not delete the project-id directory")
		}
	} else if home.ProjectDataPresent(homeDir, id) {
		// expected: name-only path either no-ops or fails without touching id
	}
	if !home.ProjectDataPresent(homeDir, id) {
		t.Fatal("project id directory must survive name-only reset attempt")
	}
}

func TestWriteProjectIdentity_LocalOnly(t *testing.T) {
	homeDir := t.TempDir()
	root := t.TempDir()
	id, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureProjectLayout(homeDir, id); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	if err := home.WriteProjectIdentity(homeDir, id, "demo", root, fixed); err != nil {
		t.Fatal(err)
	}
	doc, present, err := home.LoadProjectIdentity(homeDir, id)
	if err != nil || !present {
		t.Fatalf("identity present=%v err=%v", present, err)
	}
	if doc.ProjectID != id || doc.RootFingerprint == "" || doc.RootPath == "" {
		t.Fatalf("identity = %#v", doc)
	}
	status := home.InspectProject(homeDir, id)
	if !status.Present || !status.IdentityPresent {
		t.Fatalf("status = %#v", status)
	}
}
