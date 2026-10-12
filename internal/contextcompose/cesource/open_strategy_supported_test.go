//go:build darwin || linux

package cesource_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/cesource"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestOpenStrategy_DarwinLinux(t *testing.T) {
	if cesource.OpenStrategyForTest != "openat_nofollow" {
		t.Fatalf("OpenStrategy=%q want openat_nofollow on darwin/linux", cesource.OpenStrategyForTest)
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
		// Fail-closed is required; prefer ErrNotRegularFile after Fstat.
		t.Fatalf("want ErrNotRegularFile, got %v", err)
	}
}

func TestReadPackBounded_ConcurrentParentSymlinkSwap(t *testing.T) {
	_, homePath, projectID, _ := seedCEOnly(t)
	task := "orient atlas architecture"
	rel := atlascontext.ContextPackRel(projectID, packID(task))
	abs := filepath.Join(homePath, filepath.FromSlash(rel))
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	body := packYAMLBody(t, projectID, task, []atlascontext.PackCandidate{
		{Path: "AGENTS.md", Kind: "runtime_atlas", Reason: "authority"},
	})
	if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	realParent := parent + ".real"
	outsideDir := filepath.Join(homePath, "outside-packs-race")
	if err := os.MkdirAll(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	leaked := []byte("schema_version: 1\n# OUTSIDE-HOME-CONTENT\n")
	if err := os.WriteFile(filepath.Join(outsideDir, filepath.Base(abs)), leaked, 0o600); err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_ = os.Rename(parent, realParent)
			_ = os.Symlink(outsideDir, parent)
			time.Sleep(time.Millisecond)
			_ = os.Remove(parent)
			_ = os.Rename(realParent, parent)
			time.Sleep(time.Millisecond)
		}
		// Restore a real parent + valid pack for cleanup.
		_ = os.Remove(parent)
		if _, err := os.Stat(realParent); err == nil {
			_ = os.Rename(realParent, parent)
		} else {
			_ = os.MkdirAll(parent, 0o700)
			_ = os.WriteFile(abs, []byte(body), 0o600)
		}
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		data, err := cesource.ReadPackBoundedForTest(homePath, rel)
		if err == nil {
			if string(data) == string(leaked) || containsOutsideMarker(data) {
				stop.Store(true)
				wg.Wait()
				t.Fatal("read outside content via parent symlink swap")
			}
			continue
		}
		// Fail-closed errors are expected during the race.
	}
	stop.Store(true)
	wg.Wait()
}

func containsOutsideMarker(data []byte) bool {
	return strings.Contains(string(data), "OUTSIDE-HOME-CONTENT")
}
