package home_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestEnsureAndMirror_HomeDirPermissions0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "atlas-home")
	t.Setenv(home.EnvAtlasHome, dir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ref := filepath.Join(t.TempDir(), "ref700")
	if err := os.Mkdir(ref, 0o700); err != nil {
		t.Fatal(err)
	}
	want, _ := os.Stat(ref)
	got, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("home root perm=%#o want %#o", got.Mode().Perm(), want.Mode().Perm())
	}
	for _, name := range []string{"projects", "assets", "state"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want.Mode().Perm() {
			t.Fatalf("%s perm=%#o want %#o", name, info.Mode().Perm(), want.Mode().Perm())
		}
	}
	// Bundled assets remain readable files under the 0700 root.
	asset := filepath.Join(dir, "assets", "agents", "base.md")
	info, err := os.Stat(asset)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o400 == 0 {
		t.Fatal("asset must remain readable")
	}
}

func TestEnsureAndMirror_HardensExisting0755Home(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "atlas-home")
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(home.EnvAtlasHome, dir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ref := filepath.Join(t.TempDir(), "ref700")
	if err := os.Mkdir(ref, 0o700); err != nil {
		t.Fatal(err)
	}
	want, _ := os.Stat(ref)
	for _, path := range []string{dir, filepath.Join(dir, "projects")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want.Mode().Perm() {
			t.Fatalf("%s perm=%#o want %#o after harden", path, info.Mode().Perm(), want.Mode().Perm())
		}
	}
}

func TestEnsureLayout_BlocksSymlinkHomeRoot(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureLayout(link); err == nil {
		t.Fatal("expected symlink home root block")
	}
}
