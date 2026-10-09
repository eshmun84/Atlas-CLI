package home_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestResolve_UsesATLAS_HOME(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, dir)
	got, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(dir)
	if got != want {
		t.Fatalf("Resolve() = %q want %q", got, want)
	}
}

func TestInspect_DoesNotCreateHome(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing-home")
	t.Setenv(home.EnvAtlasHome, dir)
	before, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	status := home.Inspect()
	if status.Exists {
		t.Fatal("home should not exist")
	}
	if status.Path == "" {
		t.Fatal("path should resolve")
	}
	after, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("Inspect mutated parent dir: before=%d after=%d", len(before), len(after))
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("Inspect created Atlas Home")
	}
}

func TestReadCanonical_PrefersEmbeddedOverDriftedHome(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "atlas-home")
	t.Setenv(home.EnvAtlasHome, dir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	embedPath := "agents/base.md"
	trusted, err := home.ReadCanonical(embedPath)
	if err != nil || len(trusted) == 0 {
		t.Fatalf("canonical: %v", err)
	}
	homeCopy := filepath.Join(dir, "assets", filepath.FromSlash(embedPath))
	if err := os.WriteFile(homeCopy, []byte("TAMPERED HOME COPY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := home.ReadCanonical(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(trusted) {
		t.Fatalf("drifted Home must not be trusted:\ngot %q\nwant embedded", got)
	}
	if string(got) == "TAMPERED HOME COPY\n" {
		t.Fatal("returned tampered Home bytes")
	}
	status := home.Inspect()
	found := false
	for _, id := range status.DriftedAssets {
		if id == "agents/base.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Inspect must report drift: %#v", status.DriftedAssets)
	}
}

func TestEnsureAndMirror_CreatesLayoutAndAssets(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "atlas-home")
	t.Setenv(home.EnvAtlasHome, dir)
	fixed := time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC)
	result, err := home.EnsureAndMirror(fixed)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.HomePath == "" {
		t.Fatalf("result = %#v", result)
	}
	for _, name := range home.LayoutDirectories {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() {
			t.Fatalf("layout missing %s: %v", name, err)
		}
	}
	for _, asset := range home.BundledAssets() {
		path := home.AssetHomePath(dir, asset)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing asset %s: %v", asset.ID, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "atlas-orchestrator.md")); err != nil {
		t.Fatal("convenience agent missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "adapters", "cursor", "atlas.mdc")); err != nil {
		t.Fatal("convenience adapter missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "contracts", "sdd-openspec.md")); err != nil {
		t.Fatal("mirrored SDD contract missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "contracts", "sdd-openspec.md")); err != nil {
		t.Fatal("convenience SDD contract missing")
	}
	status := home.Inspect()
	if !status.Exists || !status.LayoutComplete || len(status.MissingAssets) != 0 || len(status.DriftedAssets) != 0 || len(status.AssetErrors) != 0 {
		t.Fatalf("status = %#v", status)
	}
	doc, present, err := home.LoadState(dir)
	if err != nil || !present || doc.HomePath == "" || len(doc.Assets) == 0 {
		t.Fatalf("state = %#v present=%v err=%v", doc, present, err)
	}
}

func TestInspect_EmbeddedAssetReadFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, dir)
	if _, err := home.EnsureAndMirror(time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	var failID string
	home.SetReadEmbeddedAssetForTest(func(embedPath string) ([]byte, error) {
		if failID == "" {
			failID = embedPath
			return nil, errors.New("injected embed read failure")
		}
		return assets.Content.ReadFile(embedPath)
	})
	t.Cleanup(func() { home.SetReadEmbeddedAssetForTest(nil) })

	status := home.InspectPath(dir)
	if len(status.AssetErrors) == 0 {
		t.Fatal("expected AssetErrors for embedded read failure")
	}
	if !strings.Contains(status.AssetErrors[0], "injected embed read failure") {
		t.Fatalf("AssetErrors = %#v", status.AssetErrors)
	}
	// Unrelated diagnostics still available.
	if !status.Exists || !status.LayoutComplete {
		t.Fatalf("partial inspect must still report layout: %#v", status)
	}
}
