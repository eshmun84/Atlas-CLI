//go:build unix

package mutatelock_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
)

func TestAcquire_BusySameProcessSecondAcquire(t *testing.T) {
	cache := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	ws := t.TempDir()
	a, err := mutatelock.Acquire(mutatelock.Options{HomePath: home, Workspace: ws, CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Release() }()

	_, err = mutatelock.Acquire(mutatelock.Options{HomePath: home, Workspace: ws, CacheDir: cache})
	if !errors.Is(err, mutatelock.ErrBusy) {
		t.Fatalf("want ErrBusy, got %v", err)
	}
}

func TestAcquire_DoesNotCreateGovernedPaths(t *testing.T) {
	cache := t.TempDir()
	home := filepath.Join(t.TempDir(), "missing-home")
	ws := t.TempDir()
	set, err := mutatelock.Acquire(mutatelock.Options{HomePath: home, Workspace: ws, CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = set.Release() }()
	if _, err := os.Lstat(home); !os.IsNotExist(err) {
		t.Fatalf("must not create ATLAS_HOME: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(ws, ".atlas")); !os.IsNotExist(err) {
		t.Fatalf("must not create workspace/.atlas: %v", err)
	}
	locks := filepath.Join(cache, "atlas", "locks")
	info, err := os.Lstat(locks)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		t.Fatal("locks must be a real directory")
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("lock root perm=%04o want 0700", info.Mode().Perm())
	}
	entries, err := os.ReadDir(locks)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected lock files under cache, got %d", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".lock") {
			t.Fatalf("unexpected entry %s", e.Name())
		}
		info, err := os.Lstat(filepath.Join(locks, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("lock must be regular file: %s", e.Name())
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("lock perm=%04o want 0600", info.Mode().Perm())
		}
	}
}

func TestAcquire_PreexistingLockRoot0777_TightensTo0700(t *testing.T) {
	cache := t.TempDir()
	locks := filepath.Join(cache, "atlas", "locks")
	if err := os.MkdirAll(locks, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locks, 0o777); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	set, err := mutatelock.Acquire(mutatelock.Options{HomePath: home, CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = set.Release() }()
	info, err := os.Lstat(locks)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("lock root perm=%04o want 0700 after tighten", info.Mode().Perm())
	}
}

func TestAcquire_CacheBaseGroupOtherWritable_FailClosed(t *testing.T) {
	cache := t.TempDir()
	if err := os.Chmod(cache, 0o777); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(cache)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mutatelock.Acquire(mutatelock.Options{HomePath: t.TempDir(), CacheDir: cache})
	if err == nil || !strings.Contains(err.Error(), "group/other writable") {
		t.Fatalf("expected cache base fail closed, got %v", err)
	}
	after, err := os.Lstat(cache)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("cache mode must be unchanged: before=%04o after=%04o", before.Mode().Perm(), after.Mode().Perm())
	}
}

func TestAcquire_CacheBaseSelfOwnedNotWorldWritable_Allowed(t *testing.T) {
	cache := t.TempDir()
	// 0755: self-owned, group/other readable but not writable — allowed for UserCacheDir.
	if err := os.Chmod(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	set, err := mutatelock.Acquire(mutatelock.Options{HomePath: t.TempDir(), CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = set.Release() }()
	info, err := os.Lstat(cache)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("cache mode must remain unchanged: got %04o", info.Mode().Perm())
	}
}

func TestAcquire_AtlasDir0777_TightensTo0700(t *testing.T) {
	cache := t.TempDir()
	atlas := filepath.Join(cache, "atlas")
	if err := os.Mkdir(atlas, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(atlas, 0o777); err != nil {
		t.Fatal(err)
	}
	set, err := mutatelock.Acquire(mutatelock.Options{HomePath: t.TempDir(), CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = set.Release() }()
	info, err := os.Lstat(atlas)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("atlas perm=%04o want 0700 after tighten", info.Mode().Perm())
	}
}

func TestAcquire_AtlasComponentSymlink_FailClosed(t *testing.T) {
	cache := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(cache, "atlas")); err != nil {
		t.Fatal(err)
	}
	_, err := mutatelock.Acquire(mutatelock.Options{HomePath: t.TempDir(), CacheDir: cache})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected atlas symlink fail closed, got %v", err)
	}
}

func TestAcquire_LocksLeafSymlink_FailClosed(t *testing.T) {
	cache := t.TempDir()
	atlas := filepath.Join(cache, "atlas")
	if err := os.Mkdir(atlas, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(atlas, "locks")); err != nil {
		t.Fatal(err)
	}
	_, err := mutatelock.Acquire(mutatelock.Options{HomePath: t.TempDir(), CacheDir: cache})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected locks symlink fail closed, got %v", err)
	}
}

func TestAcquire_LockRootNonDirectory_FailClosed(t *testing.T) {
	cache := t.TempDir()
	atlas := filepath.Join(cache, "atlas")
	if err := os.Mkdir(atlas, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(atlas, "locks"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := mutatelock.Acquire(mutatelock.Options{HomePath: t.TempDir(), CacheDir: cache})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected non-directory fail closed, got %v", err)
	}
}

func TestAcquire_SymlinkLockLeaf_FailClosed(t *testing.T) {
	cache := t.TempDir()
	locks := filepath.Join(cache, "atlas", "locks")
	if err := os.MkdirAll(locks, 0o700); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	canon, err := mutatelock.CanonicalTarget(home)
	if err != nil {
		t.Fatal(err)
	}
	// Pre-create a symlink where the home lock file would be.
	// We discover the name by acquiring once, releasing, removing, and replacing with symlink.
	set, err := mutatelock.Acquire(mutatelock.Options{HomePath: home, CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	_ = set.Release()
	entries, err := os.ReadDir(locks)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	lockPath := filepath.Join(locks, entries[0].Name())
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, lockPath); err != nil {
		t.Fatal(err)
	}
	_, err = mutatelock.Acquire(mutatelock.Options{HomePath: home, CacheDir: cache})
	if err == nil {
		t.Fatal("expected fail closed on symlink lock leaf")
	}
	_ = canon
}

func TestAcquire_CrossProcessBusy_ZeroMutation(t *testing.T) {
	if os.Getenv("ATLAS_MUTATELOCK_CHILD") == "1" {
		cache := os.Getenv("ATLAS_MUTATELOCK_CACHE")
		home := os.Getenv("ATLAS_MUTATELOCK_HOME")
		ws := os.Getenv("ATLAS_MUTATELOCK_WS")
		ready := os.Getenv("ATLAS_MUTATELOCK_READY")
		set, err := mutatelock.Acquire(mutatelock.Options{HomePath: home, Workspace: ws, CacheDir: cache})
		if err != nil {
			os.Exit(2)
		}
		defer func() { _ = set.Release() }()
		if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
			os.Exit(3)
		}
		time.Sleep(3 * time.Second)
		os.Exit(0)
	}

	cache := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	ws := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	marker := filepath.Join(ws, "must-not-exist")

	cmd := exec.Command(os.Args[0], "-test.run=TestAcquire_CrossProcessBusy_ZeroMutation", "-test.v=false")
	cmd.Env = append(os.Environ(),
		"ATLAS_MUTATELOCK_CHILD=1",
		"ATLAS_MUTATELOCK_CACHE="+cache,
		"ATLAS_MUTATELOCK_HOME="+home,
		"ATLAS_MUTATELOCK_WS="+ws,
		"ATLAS_MUTATELOCK_READY="+ready,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Lstat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not acquire lock in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, err := mutatelock.Acquire(mutatelock.Options{HomePath: home, Workspace: ws, CacheDir: cache})
	if !errors.Is(err, mutatelock.ErrBusy) {
		t.Fatalf("want ErrBusy from second process, got %v", err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatal("busy path must perform zero product mutation")
	}
	if _, err := os.Lstat(home); !os.IsNotExist(err) {
		t.Fatal("busy path must not create home")
	}
}
