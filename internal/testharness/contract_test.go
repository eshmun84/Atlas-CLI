package testharness_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	"github.com/eshmun84/Atlas-CLI/internal/testharness"
)

func TestMain(m *testing.M) {
	installed := testharness.Install(testharness.Options{})
	code := m.Run()
	installed.Cleanup()
	os.Exit(code)
}

func TestContractOpenSpec_IsDoubleNotLiveCLI(t *testing.T) {
	look, err := exec.LookPath("openspec")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(look, "atlas-contract-bin-") {
		t.Fatalf("LookPath = %s, want the contract double", look)
	}
	out, err := exec.Command(look, "version", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), testharness.ContractOpenSpecVersion) {
		t.Fatalf("version = %s", out)
	}
}

func TestContractOpenSpec_InitHelpDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	look, err := exec.LookPath("openspec")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(look, "init", "--help")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--tools") {
		t.Fatalf("help = %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("help created openspec/: %v", err)
	}
}

func TestContractOpenSpec_InitCursorReady(t *testing.T) {
	root := t.TempDir()
	look, err := exec.LookPath("openspec")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(look, "init", "--tools", "cursor", "--no-animation")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if st, reason := openspec.ClassifyProject(root); st != openspec.ProjectReady {
		t.Fatalf("classify = %s (%s)", st, reason)
	}
	if missing := openspec.MissingToolProjections(root, []string{"cursor"}); len(missing) != 0 {
		t.Fatalf("markers missing: %v", missing)
	}
	if missing := openspec.MissingToolProjections(root, []string{"opencode"}); len(missing) != 1 {
		t.Fatalf("opencode markers = %v, want missing", missing)
	}
}

func TestContractOpenSpec_RefusesSymlinkRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "openspec")); err != nil {
		t.Fatal(err)
	}
	look, err := exec.LookPath("openspec")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(look, "init", "--tools", "none", "--no-animation")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("symlink init succeeded: %s", out)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("wrote through symlink: %v", entries)
	}
}

func TestContractOpenSpec_ClearingPATHHidesDouble(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))
	if _, err := exec.LookPath("openspec"); err == nil {
		t.Fatal("empty PATH must not find the contract double")
	}
}
