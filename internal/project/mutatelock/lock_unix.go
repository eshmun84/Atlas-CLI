//go:build unix

package mutatelock

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ErrBusy is returned when another cooperating Atlas process holds the lock.
var ErrBusy = errors.New("another Atlas mutation is in progress")

// Set holds acquired exclusive locks for the lifetime of one mutation transaction.
type Set struct {
	files []*os.File
}

// Options controls Acquire. CacheDir overrides UserCacheDir for tests only.
type Options struct {
	HomePath  string // canonicalized; empty skips HOME lock
	Workspace string // canonicalized; empty skips WORKSPACE lock
	CacheDir  string // optional override of os.UserCacheDir()
}

// Acquire takes HOME then WORKSPACE exclusive locks (non-blocking).
// Creates only UserCacheDir()/atlas/locks coordination state — never governed product paths.
func Acquire(opts Options) (Set, error) {
	var set Set
	root, err := lockRoot(opts.CacheDir)
	if err != nil {
		return Set{}, err
	}
	if strings.TrimSpace(opts.HomePath) != "" {
		canon, err := CanonicalTarget(opts.HomePath)
		if err != nil {
			return Set{}, err
		}
		f, err := acquireOne(root, "home", canon)
		if err != nil {
			_ = set.Release()
			return Set{}, err
		}
		set.files = append(set.files, f)
	}
	if strings.TrimSpace(opts.Workspace) != "" {
		canon, err := CanonicalTarget(opts.Workspace)
		if err != nil {
			_ = set.Release()
			return Set{}, err
		}
		f, err := acquireOne(root, "workspace", canon)
		if err != nil {
			_ = set.Release()
			return Set{}, err
		}
		set.files = append(set.files, f)
	}
	if len(set.files) == 0 {
		return Set{}, fmt.Errorf("mutatelock: at least one of HomePath or Workspace is required")
	}
	return set, nil
}

// Release unlocks and closes held lock files. Idempotent. Does not delete lock files.
func (s *Set) Release() error {
	if s == nil {
		return nil
	}
	var first error
	for i := len(s.files) - 1; i >= 0; i-- {
		f := s.files[i]
		if f == nil {
			continue
		}
		if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil && first == nil {
			first = err
		}
		if err := f.Close(); err != nil && first == nil {
			first = err
		}
		s.files[i] = nil
	}
	s.files = nil
	return first
}

func lockRoot(cacheOverride string) (string, error) {
	cache := strings.TrimSpace(cacheOverride)
	if cache == "" {
		var err error
		cache, err = os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("mutatelock: user cache dir: %w", err)
		}
	}
	cache, err := resolveCacheBase(cache)
	if err != nil {
		return "", err
	}

	atlas := filepath.Join(cache, "atlas")
	if err := ensureTrustedAtlasDir(atlas); err != nil {
		return "", err
	}

	locks := filepath.Join(atlas, "locks")
	if err := ensureTrustedLockRoot(locks); err != nil {
		return "", err
	}
	return locks, nil
}

// resolveCacheBase Abs/Cleans and resolves an existing cache directory,
// then fail-closes unless it is self-owned and not group/other-writable.
// Does not chmod the user's general cache directory or parents above it.
func resolveCacheBase(cache string) (string, error) {
	abs, err := filepath.Abs(cache)
	if err != nil {
		return "", fmt.Errorf("mutatelock: cache abs: %w", err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(abs, 0o700); err != nil {
			return "", fmt.Errorf("mutatelock: create cache base: %w", err)
		}
		info, err = os.Lstat(abs)
		if err != nil {
			return "", fmt.Errorf("mutatelock: lstat cache base: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("mutatelock: lstat cache base: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("mutatelock: resolve cache base: %w", err)
		}
		abs = filepath.Clean(resolved)
		info, err = os.Lstat(abs)
		if err != nil {
			return "", fmt.Errorf("mutatelock: lstat resolved cache: %w", err)
		}
	}
	if !info.IsDir() {
		return "", fmt.Errorf("mutatelock: cache base is not a directory")
	}
	if err := verifyTrustedCacheBase(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// verifyTrustedCacheBase requires a real self-owned directory with no
// group/other write bits. Never chmods the cache base.
func verifyTrustedCacheBase(path string) error {
	euid := unix.Geteuid()
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return fmt.Errorf("mutatelock: lstat cache base: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("mutatelock: cache base is not a directory")
	}
	if !AcceptSelfOwned(st.Uid, euid) {
		return fmt.Errorf("mutatelock: cache base not owned by current user")
	}
	if !AcceptCacheBaseMode(uint32(st.Mode)) {
		return fmt.Errorf("mutatelock: cache base is group/other writable")
	}
	return nil
}

// ensureTrustedAtlasDir validates/creates <cache>/atlas:
// real dir, not symlink, owned by EUID, mode ends as 0700.
func ensureTrustedAtlasDir(path string) error {
	euid := unix.Geteuid()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := unix.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("mutatelock: mkdir atlas: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("mutatelock: lstat atlas: %w", err)
	} else {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("mutatelock: atlas dir is a symlink")
		}
		if !info.IsDir() {
			return fmt.Errorf("mutatelock: atlas dir is not a directory")
		}
	}

	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return fmt.Errorf("mutatelock: lstat atlas: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("mutatelock: atlas dir is not a directory")
	}
	if !AcceptSelfOwned(st.Uid, euid) {
		// Fail closed: never chmod a foreign-owned atlas coordination dir.
		return fmt.Errorf("mutatelock: atlas dir not owned by current user")
	}
	if st.Mode&0o777 != 0o700 {
		if err := unix.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("mutatelock: chmod atlas: %w", err)
		}
		if err := unix.Lstat(path, &st); err != nil {
			return fmt.Errorf("mutatelock: relstat atlas: %w", err)
		}
		if st.Mode&0o777 != 0o700 {
			return fmt.Errorf("mutatelock: atlas mode %#o after chmod", st.Mode&0o777)
		}
	}
	return nil
}

// ensureTrustedLockRoot validates/creates the locks directory:
// real dir, owned by EUID, mode ends as 0700.
func ensureTrustedLockRoot(path string) error {
	euid := unix.Geteuid()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := unix.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("mutatelock: mkdir locks: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("mutatelock: lstat locks: %w", err)
	} else {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("mutatelock: lock root is a symlink")
		}
		if !info.IsDir() {
			return fmt.Errorf("mutatelock: lock root is not a directory")
		}
	}

	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return fmt.Errorf("mutatelock: lstat locks: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("mutatelock: lock root is not a directory")
	}
	if !AcceptSelfOwned(st.Uid, euid) {
		return fmt.Errorf("mutatelock: lock root not owned by current user")
	}
	if st.Mode&0o777 != 0o700 {
		if err := unix.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("mutatelock: chmod lock root: %w", err)
		}
		if err := unix.Lstat(path, &st); err != nil {
			return fmt.Errorf("mutatelock: relstat lock root: %w", err)
		}
		if st.Mode&0o777 != 0o700 {
			return fmt.Errorf("mutatelock: lock root mode %#o after chmod", st.Mode&0o777)
		}
	}
	return nil
}

func lockFileName(kind, canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return kind + "-" + hex.EncodeToString(sum[:]) + ".lock"
}

func acquireOne(root, kind, canonical string) (*os.File, error) {
	name := lockFileName(kind, canonical)
	path := filepath.Join(root, name)
	euid := unix.Geteuid()

	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("mutatelock: open %s lock: %w", kind, err)
	}
	f := os.NewFile(uintptr(fd), path)

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("mutatelock: fstat %s lock: %w", kind, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = f.Close()
		return nil, fmt.Errorf("mutatelock: %s lock is not a regular file", kind)
	}
	if !AcceptSelfOwned(st.Uid, euid) {
		_ = f.Close()
		// Fail closed: never chmod or flock a foreign-owned lock file.
		return nil, fmt.Errorf("mutatelock: %s lock not owned by current user", kind)
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("mutatelock: fchmod %s lock: %w", kind, err)
	}

	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("mutatelock: flock %s: %w", kind, err)
	}
	return f, nil
}
