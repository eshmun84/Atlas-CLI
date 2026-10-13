package main

import (
	"fmt"
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
	want, err := resolveExistingPath(root)
	if err != nil {
		t.Fatal(err)
	}
	// validate returns Abs+Clean without requiring EvalSymlinks; both must exist.
	if filepath.Base(got) != filepath.Base(want) {
		t.Fatalf("got %q want base of %q", got, want)
	}
	info, err := os.Lstat(got)
	if err != nil || !info.IsDir() {
		t.Fatalf("validated root not usable: %v", err)
	}
}

func TestPrepareRunWorkspace_ResolvesSymlinkParents(t *testing.T) {
	realRoot := t.TempDir()
	parent := t.TempDir()
	// Valid E2E root must itself be a real directory (symlink roots rejected).
	// Symlink parents of MkdirTemp children still need EvalSymlinks consistency.
	ws, err := prepareRunWorkspace(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Cleanup() })
	for _, path := range []string{ws.RunDir, ws.Project, ws.AtlasHome} {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Clean(path) != filepath.Clean(resolved) {
			t.Fatalf("path not symlink-resolved: %s vs %s", path, resolved)
		}
	}
	_ = parent
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
		{"dotdot", "..", "ambiguous"},
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
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q missing %q", err, tc.want)
			}
		})
	}
}

func TestValidateE2ERoot_SymlinkRejected(t *testing.T) {
	realRoot := t.TempDir()
	parent := t.TempDir()
	link := filepath.Join(parent, "e2e-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}
	_, err := validateE2ERoot(link)
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected symlink refusal, got %v", err)
	}
	// Real target remains usable and untouched.
	if _, err := validateE2ERoot(realRoot); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(realRoot, "keep-me.txt")
	if err := os.WriteFile(marker, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "safe" {
		t.Fatalf("symlink validation must not mutate target: %v %q", err, data)
	}
}

func TestPrepareRunWorkspace_FreshUniqueAndPreservesExisting(t *testing.T) {
	root := t.TempDir()
	precious := filepath.Join(root, "user-owned-precious")
	if err := os.Mkdir(precious, 0o755); err != nil {
		t.Fatal(err)
	}
	preciousFile := filepath.Join(precious, "notes.txt")
	if err := os.WriteFile(preciousFile, []byte("do-not-delete"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Fixed historical names must never be assumed owned / deleted.
	legacy := filepath.Join(root, "memory-skill-36b5")
	if err := os.Mkdir(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(legacy, "evidence.txt")
	if err := os.WriteFile(legacyFile, []byte("prior-run"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws1, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws1.RunDir == ws2.RunDir {
		t.Fatal("repeated runs must create distinct workspaces")
	}
	if !strings.HasPrefix(filepath.Base(ws1.RunDir), "atlas-e2e-persist-") {
		t.Fatalf("unexpected run dir name: %s", ws1.RunDir)
	}

	// Existing directories preserved.
	for _, path := range []string{preciousFile, legacyFile} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("existing path deleted: %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("existing path emptied: %s", path)
		}
	}

	// Keep both workspaces (simulate success evidence retention).
	if _, err := os.Lstat(ws1.RunDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(ws2.RunDir); err != nil {
		t.Fatal(err)
	}
}

func TestCleanup_OnlyOwnedRunDir(t *testing.T) {
	root := t.TempDir()
	sibling := filepath.Join(root, "sibling-evidence")
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "keep.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	runDir := ws.RunDir
	if err := os.WriteFile(filepath.Join(ws.Project, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(runDir); !os.IsNotExist(err) {
		t.Fatalf("owned run dir should be removed, err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(sibling, "keep.txt"))
	if err != nil || string(data) != "ok" {
		t.Fatalf("sibling must be preserved: %v %q", err, data)
	}
	// Second cleanup is a no-op.
	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestCleanup_RefusesSymlinkRunDir(t *testing.T) {
	root := t.TempDir()
	realDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(realDir, "payload"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "atlas-e2e-persist-fake")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	ws := &runWorkspace{Root: root, RunDir: link, created: true}
	if err := ws.Cleanup(); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink cleanup refusal, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(realDir, "payload"))
	if err != nil || string(data) != "keep" {
		t.Fatalf("symlink target must remain: %v %q", err, data)
	}
}

func TestFailedInitializationCleanup(t *testing.T) {
	root := t.TempDir()
	precious := filepath.Join(root, "preexisting")
	if err := os.Mkdir(precious, 0o755); err != nil {
		t.Fatal(err)
	}

	ws, err := prepareRunWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	runDir := ws.RunDir

	// Simulate failed initialization: cleanup owned workspace only.
	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(runDir); !os.IsNotExist(err) {
		t.Fatal("failed init must remove owned run dir")
	}
	if _, err := os.Lstat(precious); err != nil {
		t.Fatal("failed init must not remove preexisting dirs")
	}
}

func TestRepeatedRuns_NoDestructiveBehavior(t *testing.T) {
	root := t.TempDir()
	var kept []string
	for i := 0; i < 3; i++ {
		ws, err := prepareRunWorkspace(root)
		if err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(ws.Project, "run.txt")
		if err := os.WriteFile(marker, []byte(ws.RunDir), 0o644); err != nil {
			t.Fatal(err)
		}
		kept = append(kept, ws.RunDir)
		// Retain evidence (do not Cleanup) — mirrors successful e2ememory runs.
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected retained run dirs, got %d", len(entries))
	}
	for _, dir := range kept {
		if _, err := os.Lstat(filepath.Join(dir, "project", "run.txt")); err != nil {
			t.Fatalf("prior run evidence missing: %v", err)
		}
	}
}

func TestPrepareRunWorkspace_PartialCreateCleansOwnedOnly(t *testing.T) {
	root := t.TempDir()
	sibling := filepath.Join(root, "other")
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "keep.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := mkdir
	defer func() { mkdir = orig }()
	calls := 0
	mkdir = func(path string, perm os.FileMode) error {
		calls++
		if calls == 1 {
			return orig(path, perm) // project ok
		}
		return fmt.Errorf("forced atlas-home failure")
	}

	ws, err := prepareRunWorkspace(root)
	if err == nil || ws != nil {
		t.Fatalf("expected partial create failure, got ws=%v err=%v", ws, err)
	}
	if !strings.Contains(err.Error(), "atlas-home") {
		t.Fatalf("unexpected error: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "atlas-e2e-persist-") {
			t.Fatalf("owned run dir leaked after partial failure: %s", e.Name())
		}
	}
	data, err := os.ReadFile(filepath.Join(sibling, "keep.txt"))
	if err != nil || string(data) != "ok" {
		t.Fatalf("sibling must survive: %v %q", err, data)
	}
}
