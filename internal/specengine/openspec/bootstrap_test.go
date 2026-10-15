package openspec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestEnsureBootstrap_RemovedFailClosed(t *testing.T) {
	root := t.TempDir()
	_, err := openspec.EnsureBootstrap(root)
	if err == nil || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("expected removed error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec")); !os.IsNotExist(err) {
		t.Fatal("must not create openspec/ via EnsureBootstrap")
	}
}

func TestAssessLayout_Absent(t *testing.T) {
	assess, err := openspec.AssessLayout(t.TempDir())
	if err != nil || !assess.Absent {
		t.Fatalf("%#v %v", assess, err)
	}
}
