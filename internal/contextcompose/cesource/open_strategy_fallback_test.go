//go:build !darwin && !linux

package cesource_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/cesource"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestOpenStrategy_OtherPlatforms(t *testing.T) {
	if cesource.OpenStrategyForTest != "lstat_open_fallback" {
		t.Fatalf("OpenStrategy=%q want lstat_open_fallback off darwin/linux", cesource.OpenStrategyForTest)
	}
}

func TestReadPackBounded_SymlinkParentRejected(t *testing.T) {
	_, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(filepath.Dir(parent), 0o700); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Join(homePath, "outside-packs")
	if err := os.MkdirAll(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, filepath.Base(abs))
	if err := os.WriteFile(outsideFile, []byte("schema_version: 1\n# leaked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, parent); err != nil {
		t.Fatal(err)
	}

	_, err := cesource.ReadPackBoundedForTest(homePath, rel)
	if err == nil {
		t.Fatal("expected parent symlink refusal")
	}
	if !errors.Is(err, fsafety.ErrSymlink) {
		t.Fatalf("want ErrSymlink, got %v", err)
	}
}

func TestReadPackBounded_NonRegularRejected(t *testing.T) {
	_, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(abs, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := cesource.ReadPackBoundedForTest(homePath, rel)
	if err == nil {
		t.Fatal("expected non-regular refusal")
	}
	if !errors.Is(err, fsafety.ErrNotRegularFile) {
		t.Fatalf("want ErrNotRegularFile, got %v", err)
	}
}
