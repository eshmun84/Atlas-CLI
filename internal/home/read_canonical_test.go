package home_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestReadCanonical_AssetLeafSymlink_ReturnsEmbedded(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	embedPath := "agents/base.md"
	embedded, err := assets.Content.ReadFile(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	extPath := filepath.Join(outside, "base.md")
	// External file byte-matches embed — must still not be trusted via symlink.
	if err := os.WriteFile(extPath, embedded, 0o644); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(homeDir, "assets", filepath.FromSlash(embedPath))
	if err := os.Remove(leaf); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extPath, leaf); err != nil {
		t.Fatal(err)
	}

	got, err := home.ReadCanonical(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, embedded) {
		t.Fatal("must return embedded canonical, not follow symlink")
	}
	// Mutate external after ReadCanonical setup to prove we didn't latch onto it.
	if err := os.WriteFile(extPath, []byte("EXTERNAL MUTATED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got2, err := home.ReadCanonical(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got2, embedded) {
		t.Fatal("external symlink target must not be read/trusted")
	}
	if string(got2) == "EXTERNAL MUTATED\n" {
		t.Fatal("returned external target bytes")
	}
}

func TestReadCanonical_AssetsDirSymlink_ReturnsEmbedded(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	embedPath := "agents/base.md"
	embedded, err := assets.Content.ReadFile(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	// Move real assets tree to outside and symlink assets -> outside.
	realAssets := filepath.Join(homeDir, "assets")
	moved := filepath.Join(outside, "assets")
	if err := os.Rename(realAssets, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, realAssets); err != nil {
		t.Fatal(err)
	}

	got, err := home.ReadCanonical(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, embedded) {
		t.Fatal("assets dir symlink must fall back to embedded")
	}
}

func TestReadCanonical_HealthyMatchingHomeCopy(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	embedPath := "agents/base.md"
	embedded, err := assets.Content.ReadFile(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := home.ReadCanonical(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, embedded) {
		t.Fatal("matching Home copy must equal embedded")
	}
}

func TestReadCanonical_DriftedHomeCopy_ReturnsEmbedded(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	embedPath := "agents/base.md"
	embedded, err := assets.Content.ReadFile(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	homeCopy := filepath.Join(homeDir, "assets", filepath.FromSlash(embedPath))
	if err := os.WriteFile(homeCopy, []byte("DRIFT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := home.ReadCanonical(embedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, embedded) {
		t.Fatal("drifted Home must not be trusted")
	}
	if string(got) == "DRIFT\n" {
		t.Fatal("returned drifted bytes")
	}
}
