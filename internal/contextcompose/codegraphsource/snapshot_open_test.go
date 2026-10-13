package codegraphsource_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/codegraphsource"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestSnapshot_GraphDBSymlinkRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.db")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "graph.db")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(link)
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatal("expected symlink graph.db rejection")
	}
	if !errors.Is(err, fsafety.ErrSymlink) && !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("want symlink error, got %v", err)
	}
}

func TestSnapshot_WALSymlinkRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "graph.db")
	if err := os.WriteFile(db, []byte("main-db"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "evil-wal")
	if err := os.WriteFile(target, []byte("ATTACK"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, db+"-wal"); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(db)
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatal("expected WAL symlink rejection")
	}
	if !errors.Is(err, fsafety.ErrSymlink) && !strings.Contains(err.Error(), "graph.db-wal") {
		t.Fatalf("want wal symlink error, got %v", err)
	}
}

func TestSnapshot_SHMSymlinkRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "graph.db")
	if err := os.WriteFile(db, []byte("main-db"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "evil-shm")
	if err := os.WriteFile(target, []byte("ATTACK"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, db+"-shm"); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(db)
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatal("expected SHM symlink rejection")
	}
	if !errors.Is(err, fsafety.ErrSymlink) && !strings.Contains(err.Error(), "graph.db-shm") {
		t.Fatalf("want shm symlink error, got %v", err)
	}
}

func TestSnapshot_RegularDBAccepted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "graph.db")
	payload := []byte("sqlite-stub-content")
	if err := os.WriteFile(db, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db+"-wal", []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db+"-shm", make([]byte, 16), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(db)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got, err := os.ReadFile(snap.DB)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("snapshot content mismatch: %q", got)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(snap.DB + suffix); err != nil {
			t.Fatalf("expected sidecar copy %s: %v", suffix, err)
		}
	}
}

func TestSnapshot_MissingSidecarsOK(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "graph.db")
	if err := os.WriteFile(db, []byte("only-main"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(db)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(snap.DB + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("wal should be absent: %v", err)
	}
	if _, err := os.Stat(snap.DB + "-shm"); !os.IsNotExist(err) {
		t.Fatalf("shm should be absent: %v", err)
	}
}

func TestSnapshot_OpenedFileMustBeRegular(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fifo := filepath.Join(dir, "graph.db")
	if err := makeFIFO(fifo); err != nil {
		t.Skip(err)
	}
	defer os.Remove(fifo)
	_, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(fifo)
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatal("expected non-regular rejection")
	}
}

func TestSnapshot_BoundedCopyPreserved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "big.db")
	dst := filepath.Join(dir, "out.db")
	// Write more than a tiny bound; use explicit small bound via helper.
	if err := os.WriteFile(src, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := codegraphsource.CopyRegularFileBoundedForTest(src, dst, 8)
	if err == nil {
		t.Fatal("expected bound rejection")
	}
	if !strings.Contains(err.Error(), "exceeds snapshot bound") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("partial destination must be removed")
	}
}

func TestSnapshot_CleanupAfterFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "graph.db")
	if err := os.WriteFile(db, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), db+"-wal"); err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(db)
	if err == nil {
		cleanup()
		t.Fatal("expected failure on wal symlink")
	}
	// On failure the implementation cleans the temp dir before returning a
	// no-op cleanup; calling it must be safe and leave no snapshot DB.
	cleanup()
	if snap.Dir != "" {
		if _, err := os.Lstat(snap.Dir); !os.IsNotExist(err) {
			t.Fatalf("failed snapshot dir leaked: %v", err)
		}
	}
	if snap.DB != "" {
		if _, err := os.Lstat(snap.DB); !os.IsNotExist(err) {
			t.Fatalf("failed snapshot db leaked: %v", err)
		}
	}
}

func TestSnapshot_LeafReplacementWithSymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("O_NOFOLLOW leaf race coverage is for darwin/linux; this build uses %s", codegraphsource.SnapshotOpenStrategyForTest)
	}
	if codegraphsource.SnapshotOpenStrategyForTest != "open_nofollow" {
		t.Fatalf("SnapshotOpenStrategy=%q want open_nofollow", codegraphsource.SnapshotOpenStrategyForTest)
	}

	dir := t.TempDir()
	db := filepath.Join(dir, "graph.db")
	target := filepath.Join(dir, "secret")
	if err := os.WriteFile(target, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("regular"), 0o600); err != nil {
		t.Fatal(err)
	}

	var (
		stop atomic.Bool
		wg   sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_ = os.Remove(db)
			_ = os.Symlink(target, db)
			_ = os.Remove(db)
			_ = os.WriteFile(db, []byte("regular"), 0o600)
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	sawReject := false
	sawOK := false
	for time.Now().Before(deadline) {
		snap, cleanup, err := codegraphsource.PrepareQuerySnapshotForTest(db)
		if err != nil {
			if errors.Is(err, fsafety.ErrSymlink) || strings.Contains(err.Error(), "symlink") || os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
				sawReject = true
				continue
			}
			// Other races (ENOENT mid-replace) are acceptable; never silent success through symlink.
			continue
		}
		data, rerr := os.ReadFile(snap.DB)
		cleanup()
		if rerr != nil {
			t.Fatal(rerr)
		}
		if string(data) == "SECRET" {
			stop.Store(true)
			wg.Wait()
			t.Fatal("copied through symlink leaf during race")
		}
		if string(data) == "regular" {
			sawOK = true
		}
	}
	stop.Store(true)
	wg.Wait()
	if !sawReject && !sawOK {
		t.Fatal("race test made no observable progress")
	}
}

func TestSnapshotOpenStrategy_Documented(t *testing.T) {
	t.Parallel()
	switch runtime.GOOS {
	case "darwin", "linux":
		if codegraphsource.SnapshotOpenStrategyForTest != "open_nofollow" {
			t.Fatalf("got %q", codegraphsource.SnapshotOpenStrategyForTest)
		}
	default:
		if codegraphsource.SnapshotOpenStrategyForTest != "lstat_open_fallback" {
			t.Fatalf("got %q", codegraphsource.SnapshotOpenStrategyForTest)
		}
	}
}

func makeFIFO(path string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("fifo not available on windows")
	}
	// Directory is a non-regular leaf that open+Fstat must reject.
	return os.Mkdir(path, 0o755)
}
