package home_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestLoadState_SymlinkLeaf_EnsureAndMirrorFailsClosed(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	outside := t.TempDir()

	// Existing Home layout + marker asset so we can detect mutation.
	if err := os.MkdirAll(filepath.Join(homeDir, "state"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "assets", "agents"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(homeDir, "assets", "agents", "base.md")
	markerBytes := []byte("PREEXISTING MARKER\n")
	if err := os.WriteFile(marker, markerBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	extState := []byte("schema_version: 1\nhome_path: /forged\nsource: forged\nassets: []\n")
	extPath := filepath.Join(outside, "home.yaml")
	if err := os.WriteFile(extPath, extState, 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	if err := os.Symlink(extPath, statePath); err != nil {
		t.Fatal(err)
	}

	beforeEntries, err := os.ReadDir(homeDir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = home.EnsureAndMirror(time.Now().UTC())
	if err == nil {
		t.Fatal("EnsureAndMirror must fail closed on symlink Home state")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "state") && !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("unexpected error: %v", err)
	}

	afterEntries, err := os.ReadDir(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEntries) != len(beforeEntries) {
		t.Fatalf("Home mutated: before=%d after=%d", len(beforeEntries), len(afterEntries))
	}
	gotMarker, err := os.ReadFile(marker)
	if err != nil || string(gotMarker) != string(markerBytes) {
		t.Fatalf("Home asset mutated: %q err=%v", gotMarker, err)
	}
	gotExt, err := os.ReadFile(extPath)
	if err != nil || string(gotExt) != string(extState) {
		t.Fatalf("external state mutated: %q err=%v", gotExt, err)
	}
	info, err := os.Lstat(statePath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("state leaf must remain symlink")
	}
}

func TestLoadState_Missing_FalseNil(t *testing.T) {
	homeDir := t.TempDir()
	doc, present, err := home.LoadState(homeDir)
	if err != nil || present {
		t.Fatalf("present=%v err=%v doc=%#v", present, err, doc)
	}
}

func TestLoadProjectIdentity_SymlinkLeaf_Error(t *testing.T) {
	homeDir := t.TempDir()
	outside := t.TempDir()
	pid := "id-sym"
	localDir := filepath.Join(homeDir, "projects", pid, "local")
	if err := os.MkdirAll(localDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ext := []byte("schema_version: 1\nproject_id: forged\nproject_name: EXTERNAL\n")
	extPath := filepath.Join(outside, "identity.yaml")
	if err := os.WriteFile(extPath, ext, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, filepath.Join(localDir, "identity.yaml")); err != nil {
		t.Fatal(err)
	}
	doc, present, err := home.LoadProjectIdentity(homeDir, pid)
	if err == nil || present {
		t.Fatalf("must refuse identity symlink: present=%v err=%v doc=%#v", present, err, doc)
	}
	if doc.ProjectName == "EXTERNAL" {
		t.Fatal("external identity bytes must never be parsed")
	}
	got, _ := os.ReadFile(extPath)
	if string(got) != string(ext) {
		t.Fatal("external identity mutated")
	}
}

func TestLoadProjectLocalState_SymlinkLeaf_Error(t *testing.T) {
	homeDir := t.TempDir()
	outside := t.TempDir()
	pid := "local-sym"
	stateDir := filepath.Join(homeDir, "projects", pid, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	ext := []byte("schema_version: 1\nproject_id: forged\n")
	extPath := filepath.Join(outside, "local.yaml")
	if err := os.WriteFile(extPath, ext, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, filepath.Join(stateDir, "local.yaml")); err != nil {
		t.Fatal(err)
	}
	doc, present, err := home.LoadProjectLocalState(homeDir, pid)
	if err == nil || present {
		t.Fatalf("must refuse local-state symlink: present=%v err=%v doc=%#v", present, err, doc)
	}
	got, _ := os.ReadFile(extPath)
	if string(got) != string(ext) {
		t.Fatal("external local state mutated")
	}
}

func TestLoadProjectIdentity_Regular_OK(t *testing.T) {
	homeDir := t.TempDir()
	pid := "id-ok"
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	if err := home.WriteProjectIdentity(homeDir, pid, "demo", t.TempDir(), fixed); err != nil {
		t.Fatal(err)
	}
	doc, present, err := home.LoadProjectIdentity(homeDir, pid)
	if err != nil || !present || doc.ProjectName != "demo" {
		t.Fatalf("present=%v err=%v doc=%#v", present, err, doc)
	}
}

func TestInspectPath_HomeRootSymlink_Unhealthy(t *testing.T) {
	real := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	st := home.InspectPath(link)
	if st.LayoutComplete || st.StateError == "" {
		t.Fatalf("symlink root must be unhealthy: %#v", st)
	}
	if len(st.AssetErrors) == 0 {
		t.Fatal("expected AssetErrors for symlink root")
	}
}

func TestInspectPath_AssetsDirSymlink_LayoutIncomplete(t *testing.T) {
	homeDir := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, d := range home.LayoutDirectories {
		if d == "assets" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(homeDir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(homeDir, "assets")); err != nil {
		t.Fatal(err)
	}
	st := home.InspectPath(homeDir)
	if st.LayoutComplete {
		t.Fatalf("assets symlink must make LayoutComplete=false: %#v", st)
	}
	found := false
	for _, e := range st.AssetErrors {
		if strings.Contains(e, "assets") && strings.Contains(e, "symlink") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected assets symlink error, got %#v", st.AssetErrors)
	}
}

func TestInspectPath_AssetLeafSymlink_IntegrityError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assets := home.BundledAssets()
	if len(assets) == 0 {
		t.Fatal("no bundled assets")
	}
	asset := assets[0]
	leaf := home.AssetHomePath(homeDir, asset)
	outside := t.TempDir()
	extPath := filepath.Join(outside, "evil.md")
	if err := os.WriteFile(extPath, []byte("EXTERNAL TRUSTED?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(leaf); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, leaf); err != nil {
		t.Fatal(err)
	}
	st := home.InspectPath(homeDir)
	if len(st.AssetErrors) == 0 {
		t.Fatalf("expected asset leaf symlink error: %#v", st)
	}
	// Must not treat external content as matching / present drift.
	for _, d := range st.DriftedAssets {
		if d == asset.ID {
			t.Fatal("symlink leaf must not be scored as content drift against external bytes")
		}
	}
}

func TestLoadState_Directory_Error(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homeDir, "state", "home.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, present, err := home.LoadState(homeDir)
	if err == nil || present {
		t.Fatalf("directory state must error: present=%v err=%v", present, err)
	}
}
