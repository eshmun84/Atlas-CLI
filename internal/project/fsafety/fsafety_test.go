package fsafety_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestContainedJoin_RejectsSymlinkComponents(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	cases := []struct {
		name string
		link string
		rel  string
	}{
		{name: "atlas", link: ".atlas", rel: ".atlas/config.yaml"},
		{name: "cursor", link: ".cursor", rel: ".cursor/mcp.json"},
		{name: "opencode", link: ".opencode", rel: ".opencode/atlas.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			linkPath := filepath.Join(root, tc.link)
			if err := os.Symlink(outside, linkPath); err != nil {
				t.Fatal(err)
			}
			if _, err := fsafety.ContainedJoin(root, tc.rel); err == nil {
				t.Fatal("expected symlink component refusal")
			}
			// Zero external write: outside must stay empty.
			entries, err := os.ReadDir(outside)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("external write detected: %v", entries)
			}
			_ = os.Remove(linkPath)
		})
	}
}

func TestContainedJoin_RejectsSymlinkRoot(t *testing.T) {
	realRoot := t.TempDir()
	parent := t.TempDir()
	linkRoot := filepath.Join(parent, "linked-root")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := fsafety.ContainedJoin(linkRoot, ".atlas/config.yaml"); err == nil {
		t.Fatal("expected symlink root refusal")
	}
}

func TestContainedJoin_RejectsSymlinkParentMissingLeaf(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".cursor")); err != nil {
		t.Fatal(err)
	}
	if _, err := fsafety.ContainedJoin(root, ".cursor/rules/atlas.mdc"); err == nil {
		t.Fatal("expected refusal when parent is symlink and leaf missing")
	}
}

func TestAtomicWriteContained_BlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".atlas")); err != nil {
		t.Fatal(err)
	}
	err := fsafety.AtomicWriteContained(root, ".atlas/config.yaml", []byte("x\n"), 0o644)
	if err == nil {
		t.Fatal("expected write block")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("external write: %v", entries)
	}
}

func TestAtomicWriteContained_LegitPath(t *testing.T) {
	root := t.TempDir()
	if err := fsafety.AtomicWriteContained(root, ".atlas/config.yaml", []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil || string(data) != "ok\n" {
		t.Fatalf("got %q %v", data, err)
	}
}

func TestSafeMkdirAll_HonorsPerm(t *testing.T) {
	root := t.TempDir()
	if err := fsafety.SafeMkdirAll(root, "project-dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsafety.SafeMkdirAll(root, "mcp-dir", 0o700); err != nil {
		t.Fatal(err)
	}
	// Reference dirs created with the same requested mode capture umask effects.
	ref755 := filepath.Join(root, "ref755")
	ref700 := filepath.Join(root, "ref700")
	if err := os.Mkdir(ref755, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ref700, 0o700); err != nil {
		t.Fatal(err)
	}
	got755, err := os.Stat(filepath.Join(root, "project-dir"))
	if err != nil {
		t.Fatal(err)
	}
	got700, err := os.Stat(filepath.Join(root, "mcp-dir"))
	if err != nil {
		t.Fatal(err)
	}
	want755, _ := os.Stat(ref755)
	want700, _ := os.Stat(ref700)
	if got755.Mode().Perm() != want755.Mode().Perm() {
		t.Fatalf("0755 dir perm=%#o want %#o", got755.Mode().Perm(), want755.Mode().Perm())
	}
	if got700.Mode().Perm() != want700.Mode().Perm() {
		t.Fatalf("0700 dir perm=%#o want %#o", got700.Mode().Perm(), want700.Mode().Perm())
	}
	if got700.Mode().Perm()&0o077 != want700.Mode().Perm()&0o077 {
		t.Fatalf("0700 should not grant group/other unexpectedly: %#o", got700.Mode().Perm())
	}
}

func TestCaptureRestoreFileSnapshot(t *testing.T) {
	root := t.TempDir()
	rel := ".atlas/config.yaml"
	snapMissing, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil || snapMissing.Exists {
		t.Fatalf("missing snap: %#v %v", snapMissing, err)
	}
	if err := fsafety.AtomicWriteContained(root, rel, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil || !snap.Exists || string(snap.Data) != "v1\n" {
		t.Fatalf("snap: %#v %v", snap, err)
	}
	wroteV2, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("v2\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := fsafety.RestoreFileSnapshot(root, snap, &wroteV2, nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if string(data) != "v1\n" {
		t.Fatalf("restored %q", data)
	}
	wroteV1, _, err := fsafety.AtomicWriteContainedTracked(root, rel, []byte("v1\n"), 0o644, 0o755, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := fsafety.RestoreFileSnapshot(root, snapMissing, &wroteV1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "config.yaml")); !os.IsNotExist(err) {
		t.Fatal("expected file removed on missing snapshot restore")
	}
}
