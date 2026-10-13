package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateE2ERoot_FreshDirectory(t *testing.T) {
	root := t.TempDir()
	got, err := validateE2ERoot(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(got)
	if err != nil || !info.IsDir() {
		t.Fatalf("validated root not usable: %v", err)
	}
}

func TestValidateE2ERoot_Invalid(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", "empty"},
		{"dot", ".", "ambiguous"},
		{"missing", missing, "does not exist"},
		{"filesystem_root", string(filepath.Separator), "filesystem root"},
		{"user_home", home, "user home"},
		{"file", filePath, "not a directory"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateE2ERoot(tc.raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got err=%v want contains %q", err, tc.want)
			}
		})
	}
}

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
	if !strings.Contains(filepath.Base(ws1.RunDir), "36b7-integration-") {
		t.Fatalf("unexpected run dir name: %s", ws1.RunDir)
	}
	for _, p := range []string{ws1.ProjectA, ws1.ProjectB, ws1.AtlasHome, ws1.Evidence, ws1.BinDir} {
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
		t.Fatalf("run dir still present: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("preexisting sibling was deleted: %v", err)
	}
}

func TestWriteEvidenceFile_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ev.txt")
	if err := writeEvidenceFile(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := writeEvidenceFile(path, []byte("two")); err == nil {
		t.Fatal("expected exclusive create failure")
	}
}
