package openspec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestCaptureProviderFileSnapshots_SymlinkRootFailClosed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "openspec")); err != nil {
		t.Fatal(err)
	}
	_, err := openspec.CaptureProviderFileSnapshots(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink refusal, got %v", err)
	}
}

func TestRecordProviderMutationFootprint_NewAndModified(t *testing.T) {
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
	mustWrite("openspec/config.yaml", "before\n")
	mustWrite(".cursor/commands/opsx-propose.md", "cursor-before\n")

	before, err := openspec.InventoryProviderPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := openspec.CaptureProviderFileSnapshots(root)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite("openspec/config.yaml", "after\n")
	mustWrite(".opencode/commands/opsx-propose.md", "opencode-new\n")

	var fp fsafety.TransactionFootprint
	if err := openspec.RecordProviderMutationFootprint(&fp, root, before, baseline); err != nil {
		t.Fatal(err)
	}
	if w := fp.FileByRel("openspec/config.yaml"); w == nil || w.Created || string(w.Expected) != "after\n" {
		t.Fatalf("expected modified footprint for openspec/config.yaml: %#v", w)
	}
	if w := fp.FileByRel(".opencode/commands/opsx-propose.md"); w == nil || !w.Created {
		t.Fatalf("expected created footprint for opencode opsx: %#v", w)
	}
}
