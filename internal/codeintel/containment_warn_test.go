package codeintel_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
)

func TestContainment_WalkFailureRecordsWarning(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".codegraph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(dir, "developer-owned.bin")
	if err := os.WriteFile(unknown, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	codeintel.SetFilepathWalkForTest(func(walkRoot string, fn filepath.WalkFunc) error {
		return errors.New("injected walk failure")
	})
	t.Cleanup(func() { codeintel.SetFilepathWalkForTest(nil) })

	snap := codeintel.SnapshotCodegraphSideEffectsForTest(root)
	report := codeintel.ContainCodegraphSideEffectsForTest(root, snap)
	joined := strings.Join(report.Warnings, "\n")
	if !strings.Contains(joined, "walk") {
		t.Fatalf("expected walk warning, got %#v", report.Warnings)
	}
	got, err := os.ReadFile(unknown)
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("unknown content must stay untouched: %q err=%v", got, err)
	}
}
