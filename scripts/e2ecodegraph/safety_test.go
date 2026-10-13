package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareRunWorkspace_UniqueOwnership(t *testing.T) {
	root := t.TempDir()
	ws1, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws1.RunDir == ws2.RunDir {
		t.Fatal("expected distinct run directories")
	}
	if !strings.Contains(filepath.Base(ws1.RunDir), "atlas-e2e-codegraph-") {
		t.Fatalf("unexpected run dir name: %s", ws1.RunDir)
	}
	for _, p := range []string{ws1.Project, ws1.AtlasHome, ws1.Evidence} {
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			t.Fatalf("missing workspace dir %s: %v", p, err)
		}
	}
	_ = ws1.Cleanup()
	_ = ws2.Cleanup()
}

func TestCleanup_OnlyOwnedRunDir(t *testing.T) {
	root := t.TempDir()
	preexisting := filepath.Join(root, "keep-me")
	if err := os.Mkdir(preexisting, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(preexisting, "marker.txt")
	if err := os.WriteFile(marker, []byte("stay"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	runDir := ws.RunDir
	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("owned run dir still present: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("preexisting path was removed: %v", err)
	}
	// Second cleanup is a no-op.
	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestCleanup_RefusesOutsideRoot(t *testing.T) {
	root := t.TempDir()
	ws, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	ws.RunDir = outside
	if err := ws.Cleanup(); err == nil {
		t.Fatal("expected refusal to cleanup outside root")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside dir should remain: %v", err)
	}
}

func TestWriteEvidenceFile_ExclusiveAndFailClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "matrix.json")
	if err := writeEvidenceFile(path, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeEvidenceFile(path, []byte(`{"overwrite":true}`)); err == nil {
		t.Fatal("expected exclusive create failure on existing file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("original evidence mutated: %s", data)
	}
}

func TestWriteEvidenceFile_SymlinkLeafRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"env":"ATTACK"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "evidence.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	err := writeEvidenceFile(link, []byte(`{"ok":true}`))
	if err == nil {
		t.Fatal("expected symlink leaf to be rejected")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"env":"ATTACK"}` {
		t.Fatalf("symlink write-through occurred: %s", got)
	}
}

func TestValidateE2ERoot_RejectsSymlink(t *testing.T) {
	realRoot := t.TempDir()
	parent := t.TempDir()
	link := filepath.Join(parent, "root-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateE2ERoot(link); err == nil {
		t.Fatal("expected symlink root rejection")
	}
}
