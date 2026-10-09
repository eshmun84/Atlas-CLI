package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestCaptureInitMutationSnapshot_SymlinkParentNestedCandidateFails(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	// Symlink parent for nested candidate .cursor/agents.
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".cursor", "agents")); err != nil {
		t.Fatal(err)
	}
	_, err := captureInitMutationSnapshot(root, "", "", nil, nil)
	if err == nil {
		t.Fatal("expected symlink parent failure")
	}
	if !strings.Contains(err.Error(), "symlink") && !strings.Contains(err.Error(), "workspace dir") {
		t.Fatalf("want symlink/workspace dir error, got %v", err)
	}
}

func TestRestoreInitFiles_ProjectTree_NoRemoveAllOnForeignContent(t *testing.T) {
	homePath := t.TempDir()
	pid := "foreign-home"
	prefix := filepath.ToSlash(filepath.Join("projects", pid))
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, filepath.ToSlash(filepath.Join(prefix, "state")), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	homeFP := fsafety.TransactionFootprint{}
	homeFP.MergeDirs(dirs)
	w, wdirs, err := fsafety.AtomicWriteContainedTracked(homePath, filepath.ToSlash(filepath.Join(prefix, "state", "local.yaml")), []byte("atlas\n"), 0o600, 0o700, ".atlas-write-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	homeFP.AddFile(w)
	homeFP.MergeDirs(wdirs)
	foreign := filepath.Join(homePath, "projects", pid, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := initMutationSnapshot{
		homePath:        homePath,
		projectID:       pid,
		homeDataExisted: false,
		dirsExisted:     map[string]bool{},
		homeFootprint:   homeFP,
	}
	err = restoreInitFiles(t.TempDir(), snap)
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		// teardown reports conflict for non-empty created project root/dirs
		if err == nil || (!strings.Contains(err.Error(), "conflict") && !strings.Contains(err.Error(), "not empty")) {
			t.Fatalf("expected footprint conflict, got %v", err)
		}
	}
	got, readErr := os.ReadFile(foreign)
	if readErr != nil || string(got) != "keep\n" {
		t.Fatalf("foreign must survive: %q %v", got, readErr)
	}
}

func TestCommitProjectReset_OnlyStagingActive(t *testing.T) {
	// Authority: CommitProjectReset is invoked only after AcceptHomeReset Apply success.
	// Core does not re-check AcceptHomeReset here; inactive staging is a no-op.
	if err := restoreInitFiles(t.TempDir(), initMutationSnapshot{}); err != nil {
		t.Fatal(err)
	}
}

func TestCopyDirContainedTracked_PartialMkdir_FootprintIncludesCreatedDirs(t *testing.T) {
	homePath := t.TempDir()
	src := t.TempDir()
	// Lexical walk: "a" then "b". Pre-seed dest/b as a file so mkdir for "b" fails
	// after "a" was successfully created and recorded.
	if err := os.MkdirAll(filepath.Join(src, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	destRel := "stamp"
	if err := os.MkdirAll(filepath.Join(homePath, destRel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homePath, destRel, "b"), []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}

	var fp fsafety.TransactionFootprint
	_, err := copyDirContainedTracked(homePath, src, destRel, &fp)
	if err == nil {
		t.Fatal("expected mkdir failure during nested copy")
	}
	foundA := false
	want := filepath.ToSlash(filepath.Join(destRel, "a"))
	for _, d := range fp.Dirs {
		if d.Rel == want {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Fatalf("expected partial CreatedDir for %s, got %#v", want, fp.Dirs)
	}
}
