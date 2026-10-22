//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInstall_RemovesAbandonedStageAndKeepsLockedStage(t *testing.T) {
	prefix := t.TempDir()
	dest := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	dead := filepath.Join(dest, stagePrefix+"dead")
	if err := os.WriteFile(dead, []byte("dead"), 0o644); err != nil {
		t.Fatal(err)
	}
	heldPath := filepath.Join(dest, stagePrefix+"held")
	held, err := os.OpenFile(heldPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := unix.Flock(int(held.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(int(held.Fd()), unix.LOCK_UN)
	link := filepath.Join(dest, stagePrefix+"link")
	if err := os.Symlink("atlas", link); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(src, []byte("published"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(src, prefix, Hooks{Materialize: func() error { return nil }}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dead); !os.IsNotExist(err) {
		t.Fatalf("abandoned stage still present: %v", err)
	}
	if _, err := os.Lstat(heldPath); err != nil {
		t.Fatalf("locked stage removed: %v", err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("stage symlink removed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "atlas"))
	if err != nil || string(got) != "published" {
		t.Fatalf("binary=%q err=%v", got, err)
	}
}

func TestInstall_RefusesUnwritablePrefix(t *testing.T) {
	if unix.Geteuid() == 0 {
		t.Skip("root ignores directory mode")
	}
	prefix := t.TempDir()
	dest := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dest, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dest, 0o755) })
	probe := filepath.Join(dest, ".probe")
	if err := os.WriteFile(probe, []byte("x"), 0o644); err == nil {
		t.Skip("directory mode does not block this user")
	}
	src := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(src, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	called := false
	err := Install(src, prefix, Hooks{Materialize: func() error {
		called = true
		return nil
	}})
	if err == nil {
		t.Fatal("expected permission failure")
	}
	if called {
		t.Fatal("materialize ran without a writable prefix")
	}
}

func TestCheckPublishTarget_RefusesForeignOwner(t *testing.T) {
	err := checkPublishTarget("/bin/ls")
	if err == nil || !strings.Contains(err.Error(), "owned by uid") {
		t.Fatalf("got %v", err)
	}
}

func TestInstall_PublishedBinaryOwnedByCurrentUser(t *testing.T) {
	homeDir, prefix := isolatedInstallEnv(t)
	if err := installAt(writeBin(t, "v1"), prefix); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(prefix, "bin", "atlas")
	var st unix.Stat_t
	if err := unix.Stat(final, &st); err != nil {
		t.Fatal(err)
	}
	if st.Uid != uint32(unix.Geteuid()) {
		t.Fatalf("binary uid=%d euid=%d", st.Uid, unix.Geteuid())
	}
	var homeSt unix.Stat_t
	if err := unix.Stat(homeDir, &homeSt); err != nil {
		t.Fatal(err)
	}
	if homeSt.Uid != uint32(unix.Geteuid()) {
		t.Fatalf("home uid=%d", homeSt.Uid)
	}
}

func TestStageDeletable_ForeignBinary(t *testing.T) {
	var st unix.Stat_t
	if err := unix.Lstat("/bin/ls", &st); err != nil {
		t.Fatal(err)
	}
	if st.Uid == uint32(unix.Geteuid()) {
		t.Skip("/bin/ls is owned by the effective user")
	}
	if stageDeletable(st) {
		t.Fatal("root-owned binary would be deleted")
	}
}

func TestRemoveAbandonedStage_PreservesForeignDescriptorOwner(t *testing.T) {
	saveSeams(t)
	dest := t.TempDir()
	name := stagePrefix + "owned"
	path := filepath.Join(dest, name)
	if err := os.WriteFile(path, []byte("keep-foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := fstatFd
	fstatFd = func(fd int, st *unix.Stat_t) error {
		if err := unix.Fstat(fd, st); err != nil {
			return err
		}
		if st.Uid == uint32(unix.Geteuid()) {
			st.Uid++
		}
		return nil
	}
	t.Cleanup(func() { fstatFd = prev })
	prevUnlink := unlinkAt
	unlinkAt = func(int, string, int) error {
		t.Fatal("unlinked a stage whose open descriptor was not ours")
		return nil
	}
	t.Cleanup(func() { unlinkAt = prevUnlink })
	dirfd := mustInstallDir(t, dest)
	defer unix.Close(dirfd)
	if err := removeAbandonedStage(dirfd, name); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "keep-foreign" {
		t.Fatalf("file=%q err=%v", got, err)
	}
}

func TestRemoveAbandonedStage_ReplaceBeforeOpenPreserved(t *testing.T) {
	saveSeams(t)
	dest := t.TempDir()
	name := stagePrefix + "before-open"
	path := filepath.Join(dest, name)
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	stageAfterInspect = func() {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(path, []byte("replacement"), 0o644); err != nil {
			t.Error(err)
		}
	}
	dirfd := mustInstallDir(t, dest)
	defer unix.Close(dirfd)
	if err := removeAbandonedStage(dirfd, name); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("file=%q err=%v", got, err)
	}
}

func TestRemoveAbandonedStage_ReplaceBeforeUnlinkPreserved(t *testing.T) {
	saveSeams(t)
	dest := t.TempDir()
	name := stagePrefix + "before-unlink"
	path := filepath.Join(dest, name)
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	stageBeforeUnlink = func() {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(path, []byte("intruder"), 0o644); err != nil {
			t.Error(err)
		}
	}
	dirfd := mustInstallDir(t, dest)
	defer unix.Close(dirfd)
	if err := removeAbandonedStage(dirfd, name); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "intruder" {
		t.Fatalf("file=%q err=%v", got, err)
	}
}

func TestRemoveAbandonedStage_RejectsPathOutsideDirectory(t *testing.T) {
	parent := t.TempDir()
	secret := filepath.Join(parent, "secret")
	if err := os.WriteFile(secret, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(parent, "bin")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	dirfd := mustInstallDir(t, dest)
	defer unix.Close(dirfd)
	for _, name := range []string{"../secret", secret, "/etc/passwd", "..", stagePrefix + "x/../secret"} {
		err := removeAbandonedStage(dirfd, name)
		if err == nil || !strings.Contains(err.Error(), "outside the install directory") {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	got, err := os.ReadFile(secret)
	if err != nil || string(got) != "keep" {
		t.Fatalf("secret=%q err=%v", got, err)
	}
}

func TestSweep_DoesNotFollowInstallDirSymlink(t *testing.T) {
	real := t.TempDir()
	dead := filepath.Join(real, stagePrefix+"dead")
	if err := os.WriteFile(dead, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "bin")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	err := sweepAbandonedStages(link)
	if err == nil {
		t.Fatal("expected symlink install directory to be refused")
	}
	got, err := os.ReadFile(dead)
	if err != nil || string(got) != "keep" {
		t.Fatalf("stage behind symlink=%q err=%v", got, err)
	}
}

func TestSweep_SymlinkStageDoesNotFollowTarget(t *testing.T) {
	dest := t.TempDir()
	target := filepath.Join(t.TempDir(), "precious")
	if err := os.WriteFile(target, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dest, stagePrefix+"link")); err != nil {
		t.Fatal(err)
	}
	if err := sweepAbandonedStages(dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "precious" {
		t.Fatalf("target=%q err=%v", got, err)
	}
	info, err := os.Lstat(filepath.Join(dest, stagePrefix+"link"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink removed: %v", err)
	}
}

func mustInstallDir(t *testing.T, path string) int {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	return fd
}
