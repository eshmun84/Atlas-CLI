package hookactions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestLifecycleLock_LeafSymlinkDoesNotMutateTarget(t *testing.T) {
	for _, kind := range []string{"maintain", "pending"} {
		t.Run(kind, func(t *testing.T) {
			home, outside := t.TempDir(), t.TempDir()
			state := filepath.Join(home, "projects", "stab-lock", "state")
			if err := os.MkdirAll(state, 0o755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(outside, "secret")
			const payload = "SECRET-LEAF"
			if err := os.WriteFile(target, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			leaf := "maintain.lock"
			if kind == "pending" {
				leaf = "pending.lock"
			}
			if err := os.Symlink(target, filepath.Join(state, leaf)); err != nil {
				t.Fatal(err)
			}
			_, err := acquireLifecycleLock(kind, home, "stab-lock")
			if !errors.Is(err, fsafety.ErrSymlink) {
				t.Fatalf("acquire err=%v want symlink refusal", err)
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != payload {
				t.Fatalf("symlink target changed to %q", got)
			}
		})
	}
}

func TestLifecycleLock_ParentSymlinkDoesNotReceiveStamp(t *testing.T) {
	for _, kind := range []string{"maintain", "pending"} {
		t.Run(kind, func(t *testing.T) {
			home, outside := t.TempDir(), t.TempDir()
			secret := filepath.Join(outside, "secret")
			const payload = "SECRET-PARENT"
			if err := os.WriteFile(secret, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			parent := filepath.Join(home, "projects", "stab-lock")
			if err := os.MkdirAll(parent, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(parent, "state")); err != nil {
				t.Fatal(err)
			}
			_, err := acquireLifecycleLock(kind, home, "stab-lock")
			if err == nil {
				t.Fatal("acquire through symlink parent succeeded")
			}
			got, err := os.ReadFile(secret)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != payload {
				t.Fatalf("parent symlink target changed to %q", got)
			}
			leaf := "maintain.lock"
			if kind == "pending" {
				leaf = "pending.lock"
			}
			if _, err := os.Lstat(filepath.Join(outside, leaf)); !os.IsNotExist(err) {
				t.Fatalf("lock leaf created through parent symlink: %v", err)
			}
		})
	}
}

func TestLifecycleLock_RegularFileContends(t *testing.T) {
	for _, kind := range []string{"maintain", "pending"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			first, err := acquireLifecycleLock(kind, home, "stab-lock")
			if err != nil {
				t.Fatal(err)
			}
			info, err := first.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() {
				t.Fatalf("lock mode=%v", info.Mode())
			}
			if _, err := acquireLifecycleLock(kind, home, "stab-lock"); !errors.Is(err, errMaintainBusy) {
				t.Fatalf("second acquire=%v want busy", err)
			}
			if err := releaseLifecycleLock(kind, first); err != nil {
				t.Fatal(err)
			}
			second, err := acquireLifecycleLock(kind, home, "stab-lock")
			if err != nil {
				t.Fatal(err)
			}
			if err := releaseLifecycleLock(kind, second); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLifecycleLock_DirectoryLeafOpenFails(t *testing.T) {
	for _, kind := range []string{"maintain", "pending"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			state := filepath.Join(home, "projects", "stab-lock", "state")
			leaf := "maintain.lock"
			if kind == "pending" {
				leaf = "pending.lock"
			}
			if err := os.MkdirAll(filepath.Join(state, leaf), 0o755); err != nil {
				t.Fatal(err)
			}
			_, err := acquireLifecycleLock(kind, home, "stab-lock")
			if err == nil {
				t.Fatal("directory lock leaf was acquired")
			}
			info, err := os.Lstat(filepath.Join(state, leaf))
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() {
				t.Fatalf("lock leaf mode=%v want directory", info.Mode())
			}
		})
	}
}

func acquireLifecycleLock(kind, home, project string) (*os.File, error) {
	if kind == "pending" {
		return TryAcquirePendingLock(home, project)
	}
	return TryAcquireMaintainLock(home, project)
}

func releaseLifecycleLock(kind string, f *os.File) error {
	if kind == "pending" {
		return ReleasePendingLock(f)
	}
	return ReleaseMaintainLock(f)
}
