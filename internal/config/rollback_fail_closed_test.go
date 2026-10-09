package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
)

func TestRestoreConfigure_OwnershipAbsentInspectionFailure(t *testing.T) {
	homePath := t.TempDir()
	root := t.TempDir()
	pid := "own-abs-fail"
	outside := t.TempDir()
	ext := filepath.Join(outside, "ownership.yaml")
	if err := os.WriteFile(ext, []byte("EXTERNAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mcpDir := filepath.Join(homePath, "projects", pid, "mcp")
	if err := os.MkdirAll(mcpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(mcpDir, mcp.FileOwnership)); err != nil {
		t.Fatal(err)
	}
	err := config.RestoreConfigureFilesForTest(root, homePath, pid, false, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("expected ownership remove/inspection failure, got %v", err)
	}
	got, readErr := os.ReadFile(ext)
	if readErr != nil || string(got) != "EXTERNAL\n" {
		t.Fatalf("external mutated: %q err=%v", got, readErr)
	}
}

func TestRestoreConfigure_OwnershipAbsentMissingSucceeds(t *testing.T) {
	homePath := t.TempDir()
	root := t.TempDir()
	pid := "own-abs-ok"
	if err := os.MkdirAll(filepath.Join(homePath, "projects", pid, "mcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := config.RestoreConfigureFilesForTest(root, homePath, pid, false, nil, 0); err != nil {
		t.Fatalf("ordinary absent must succeed: %v", err)
	}
}

func TestCaptureOwnershipBaseline_SymlinkFails(t *testing.T) {
	homePath := t.TempDir()
	pid := "cap-sym"
	outside := t.TempDir()
	ext := filepath.Join(outside, "ownership.yaml")
	if err := os.WriteFile(ext, []byte("X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mcpDir := filepath.Join(homePath, "projects", pid, "mcp")
	if err := os.MkdirAll(mcpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(mcpDir, mcp.FileOwnership)); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := config.CaptureOwnershipBaselineForTest(homePath, pid)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink refuse, got %v", err)
	}
}
