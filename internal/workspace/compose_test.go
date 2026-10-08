package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/delivery"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestDiscover_ComposesProjectSnapshotAndRuntimeHealth(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	snap := result.ProjectSnapshot()
	if snap.RootPath != result.RootPath {
		t.Fatalf("snapshot root mismatch")
	}
	if snap.Files.HasReadme != result.Files.HasReadme {
		t.Fatal("embedded project fields must match snapshot")
	}
	// Runtime health is attached, not part of project.Inspect.
	_ = result.Runtime
	if delivery.DefaultMode(snap.Git.IsRepo) != delivery.ModeNone {
		t.Fatalf("no-git fixture should default delivery to none")
	}

	// project.Inspect alone must not require runtime.
	only, err := project.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if only.RootPath == "" || !only.Files.HasReadme {
		t.Fatalf("inspect incomplete: %#v", only)
	}
}
