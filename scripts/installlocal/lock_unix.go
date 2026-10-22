//go:build unix

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func acquirePrefixLock(destDir string) (*os.File, error) {
	path := filepath.Join(destDir, ".atlas.install.lock")
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("install lock: %w", err)
	}
	f := os.NewFile(uintptr(fd), path)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("install lock: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = f.Close()
		return nil, fmt.Errorf("install lock is not a regular file: %s", path)
	}
	if st.Uid != uint32(unix.Geteuid()) {
		_ = f.Close()
		return nil, fmt.Errorf("refusing to use install lock owned by uid %d", st.Uid)
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("install lock: %w", err)
	}
	if err := flockExclusive(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errInstallBusy) {
			return nil, fmt.Errorf("publish directory: %w", err)
		}
		return nil, fmt.Errorf("install lock: %w", err)
	}
	return f, nil
}

func unlockPrefix(f *os.File) error {
	if f == nil {
		return nil
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock install directory: %w", err)
	}
	return nil
}

func flockExclusive(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errInstallBusy
	}
	return fmt.Errorf("flock: %w", err)
}

var (
	fstatAt  = unix.Fstatat
	fstatFd  = unix.Fstat
	unlinkAt = unix.Unlinkat
)

func sweepAbandonedStages(destDir string) error {
	// O_NOFOLLOW applies to the final component. A symlink install directory
	// is refused instead of followed. Parent symlinks such as /var -> /private/var
	// are resolved by the kernel for earlier components; O_NOFOLLOW_ANY would
	// reject those and cannot be used here.
	dirfd, err := unix.Open(destDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open install directory: %w", err)
	}
	defer unix.Close(dirfd)

	entries, err := os.ReadDir(destDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", destDir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, stagePrefix) || len(name) == len(stagePrefix) {
			continue
		}
		if err := removeAbandonedStage(dirfd, name); err != nil {
			return err
		}
	}
	return nil
}

// removeAbandonedStage deletes one leftover stage inside the already-opened
// install directory.
//
// macOS 15.4 (Darwin 24.4) has openat, fstatat(AT_SYMLINK_NOFOLLOW) and
// unlinkat. It has no funlinkat and no unlink that succeeds only when the
// directory entry still refers to a given inode. unlinkat always removes
// whatever name is present at the syscall.
//
// Policy, fail closed:
//   - The name must be a single relative stage component. Absolute paths and
//     ".." never reach openat, because an absolute openat ignores dirfd.
//   - The directory entry is inspected with fstatat(AT_SYMLINK_NOFOLLOW).
//     Symlinks, directories and other owners are left in place.
//   - The same name is opened with openat(O_NOFOLLOW). The descriptor is
//     fstat'd. If its device, inode or generation differ from the inspection,
//     the entry was replaced and is kept.
//   - An exclusive non-blocking flock is required. A busy lock means a live
//     installer still holds the stage; the file is kept.
//   - The name is fstatat'd again immediately before unlinkat. Any mismatch
//     keeps the current entry.
//
// A replacement that lands after that last fstatat and before unlinkat can
// still be unlinked, because those two calls are not one syscall. This
// process already holds the install-directory lock, so another installlocal
// cannot create that replacement. A non-cooperating writer can. When that
// race cannot be excluded, this function keeps the file rather than deleting
// an entry it has not re-verified.
func removeAbandonedStage(dirfd int, name string) error {
	if err := validateStageLeaf(name); err != nil {
		return err
	}
	var inspected unix.Stat_t
	if err := fstatAt(dirfd, name, &inspected, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return fmt.Errorf("stat staged binary: %w", err)
	}
	if !stageDeletable(inspected) {
		return nil
	}
	if stageAfterInspect != nil {
		stageAfterInspect()
	}

	fd, err := unix.Openat(dirfd, name, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ELOOP) {
			return nil
		}
		return fmt.Errorf("open staged binary %s: %w", name, err)
	}
	defer unix.Close(fd)

	var opened unix.Stat_t
	if err := fstatFd(fd, &opened); err != nil {
		return fmt.Errorf("stat staged binary: %w", err)
	}
	if !stageDeletable(opened) || !sameStageIdentity(inspected, opened) {
		return nil
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil
		}
		return fmt.Errorf("lock staged binary: %w", err)
	}
	if err := unlinkVerifiedStage(dirfd, name, opened); err != nil {
		return fmt.Errorf("remove abandoned staged binary %s: %w", name, err)
	}
	return nil
}

func unlinkVerifiedStage(dirfd int, name string, opened unix.Stat_t) error {
	if stageBeforeUnlink != nil {
		stageBeforeUnlink()
	}
	var current unix.Stat_t
	if err := fstatAt(dirfd, name, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	if !stageDeletable(current) || !sameStageIdentity(opened, current) {
		return nil
	}
	if err := unlinkAt(dirfd, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

// stageDeletable is true only for a regular file owned by the effective uid.
// Symlinks fail this check on the no-follow stat, so their targets are never opened.
func stageDeletable(st unix.Stat_t) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFREG && st.Uid == uint32(unix.Geteuid())
}

func sameStageIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Gen == b.Gen
}

// validateStageLeaf rejects any name that could escape the install directory.
// openat uses an absolute path regardless of dirfd, so this check is mandatory.
func validateStageLeaf(name string) error {
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) || filepath.Base(name) != name || strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("refusing to remove %q outside the install directory", name)
	}
	if !strings.HasPrefix(name, stagePrefix) || len(name) == len(stagePrefix) {
		return fmt.Errorf("refusing to remove %q outside the install directory", name)
	}
	return nil
}

func sameDevice(pathA, pathB string) error {
	var a, b unix.Stat_t
	if err := unix.Stat(pathA, &a); err != nil {
		return fmt.Errorf("stat %s: %w", pathA, err)
	}
	if err := unix.Stat(pathB, &b); err != nil {
		return fmt.Errorf("stat %s: %w", pathB, err)
	}
	if a.Dev != b.Dev {
		return fmt.Errorf("staged binary and destination are on different filesystems")
	}
	return nil
}

func checkPublishTarget(path string) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return fmt.Errorf("stat publish target: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("publish collision: %s is not a regular file", path)
	}
	if st.Uid != uint32(unix.Geteuid()) {
		return fmt.Errorf("refusing to replace %s owned by uid %d", path, st.Uid)
	}
	return nil
}
