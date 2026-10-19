package openspec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestRemoveStaleToolProjections_PreservesUserAndSelected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(".cursor/commands/opsx-propose.md", "os\n")
	mustWrite(".cursor/skills/openspec-propose/SKILL.md", "os\n")
	mustWrite(".cursor/commands/my-cmd.md", "user\n")
	mustWrite(".cursor/rules/mine.mdc", "user\n")
	mustWrite(".opencode/commands/opsx-propose.md", "os\n")
	mustWrite(".opencode/skills/openspec-propose/SKILL.md", "os\n")

	var fp fsafety.TransactionFootprint
	if err := openspec.RemoveStaleToolProjections(&fp, root, []string{"opencode"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "commands", "opsx-propose.md")); !os.IsNotExist(err) {
		t.Fatal("cursor provider command must be removed")
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "skills", "openspec-propose", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("cursor provider skill must be removed")
	}
	got, err := os.ReadFile(filepath.Join(root, ".cursor", "commands", "my-cmd.md"))
	if err != nil || string(got) != "user\n" {
		t.Fatalf("user command lost: %v %q", err, got)
	}
	got, err = os.ReadFile(filepath.Join(root, ".cursor", "rules", "mine.mdc"))
	if err != nil || string(got) != "user\n" {
		t.Fatalf("user rule lost: %v %q", err, got)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); err != nil {
		t.Fatal("selected opencode provider must remain")
	}
	if !openspec.HasStaleToolProjections(root, []string{"opencode"}) {
		// cursor user files are not provider-owned; stale should be false
	} else {
		t.Fatal("no provider-owned stale files expected for cursor after cleanup")
	}
}

func TestListStaleToolProjectionFiles_OnlyProviderOwned(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "commands"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "commands", "opsx-propose.md"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "commands", "other.md"), []byte("x"), 0o644)
	files, err := openspec.ListStaleToolProjectionFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != ".cursor/commands/opsx-propose.md" {
		t.Fatalf("got %v", files)
	}
}
