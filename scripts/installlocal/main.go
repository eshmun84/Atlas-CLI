// Command installlocal stages the Atlas binary under PREFIX, materializes
// Atlas Home, then renames the staged binary into place.
//
// The install commits at a successful rename onto prefix/bin/atlas. Failures
// before that rename leave any previous binary in place and roll back Home
// mirror mutations from this install. A directory fsync failure after the
// rename does not roll Home back and does not claim a durable install.
// See rollback.go.
//
// Usage:
//
//	go run ./scripts/installlocal -binary=bin/atlas -prefix="$PREFIX"
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

const stagePrefix = ".atlas.new."

// errInstallBusy means another installlocal process holds the prefix lock
// or an individual staged binary.
var errInstallBusy = errors.New("another installation is in progress")

// errPublishDurability means rename already published the new binary and Home
// was left as materialized. The following directory fsync failed, so the
// directory entry is not known to be durable. Retrying install reconciles
// both. Home must not be restored after this error.
var errPublishDurability = errors.New("install published but not durable: directory sync after publish failed")

func main() {
	binary := flag.String("binary", "bin/atlas", "path to built atlas binary")
	prefix := flag.String("prefix", "", "install prefix (default: $PREFIX or ~/.local)")
	flag.Parse()

	p := *prefix
	if p == "" {
		p = os.Getenv("PREFIX")
	}
	if p == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fatal(err)
		}
		p = filepath.Join(homeDir, ".local")
	}

	if err := installAt(*binary, p); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "installlocal: %v\n", err)
	os.Exit(1)
}

// Hooks are the Home steps around binary publish.
// Materialize must roll back its own Home mutations before returning an error.
// Rollback runs only when Materialize succeeded and publish then failed.
type Hooks struct {
	Materialize func() error
	Rollback    func() error
}

// Install copies binary into prefix/bin as a temporary name, runs Materialize,
// syncs the temporary file, then renames it to prefix/bin/atlas on the same
// filesystem. The previous regular file at that path is replaced atomically.
// Symlinks, directories, and files owned by another user are refused.
func Install(binaryPath, prefix string, hooks Hooks) error {
	if hooks.Materialize == nil {
		return fmt.Errorf("materialize callback is required")
	}
	binaryPath = filepath.Clean(binaryPath)
	prefix = filepath.Clean(prefix)
	if binaryPath == "" || binaryPath == "." {
		return fmt.Errorf("binary path is required")
	}
	if prefix == "" || prefix == "." {
		return fmt.Errorf("prefix is required")
	}

	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("stat binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("binary path is not a regular file: %s", binaryPath)
	}

	destDir := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}

	lock, err := acquirePrefixLock(destDir)
	if err != nil {
		return err
	}
	var stage *os.File
	var stagePath string
	removeStage := false
	defer func() {
		if stage != nil {
			_ = stage.Close()
		}
		if removeStage && stagePath != "" {
			_ = os.Remove(stagePath)
		}
		if lock != nil {
			_ = unlockPrefix(lock)
			_ = lock.Close()
		}
	}()

	// Only this process holds the prefix lock, so unlocked stage files are
	// leftovers. Files still flocked by a live process are left in place.
	if err := sweepAbandonedStages(destDir); err != nil {
		return err
	}

	finalPath := filepath.Join(destDir, "atlas")
	if err := checkPublishTarget(finalPath); err != nil {
		return err
	}

	stage, err = os.CreateTemp(destDir, stagePrefix+"*")
	if err != nil {
		return fmt.Errorf("create staged binary: %w", err)
	}
	stagePath = stage.Name()
	removeStage = true

	src, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("open binary: %w", err)
	}
	if _, err := io.Copy(stage, src); err != nil {
		_ = src.Close()
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := src.Close(); err != nil {
		return fmt.Errorf("close binary: %w", err)
	}
	if err := stage.Chmod(0o755); err != nil {
		return fmt.Errorf("chmod staged binary: %w", err)
	}
	if err := syncFile(stage); err != nil {
		return fmt.Errorf("sync staged binary: %w", err)
	}
	if err := syncDir(destDir); err != nil {
		return fmt.Errorf("sync install directory: %w", err)
	}
	if err := sameDevice(stagePath, destDir); err != nil {
		return err
	}
	if err := flockExclusive(stage); err != nil {
		return fmt.Errorf("lock staged binary: %w", err)
	}

	if err := hooks.Materialize(); err != nil {
		return fmt.Errorf("materialize Atlas Home: %w", err)
	}

	// Still before the commit. A failure here restores Home and keeps the
	// previous binary.
	if err := syncBeforePublish(destDir); err != nil {
		return abortBeforePublish(hooks, fmt.Errorf("sync install directory before publish: %w", err))
	}
	if err := renameFn(stagePath, finalPath); err != nil {
		return abortBeforePublish(hooks, fmt.Errorf("publish binary: %w", err))
	}

	// Commit point. finalPath is the new binary. Do not remove it, and do not
	// roll Home back, even if the durability sync below fails.
	removeStage = false
	if err := syncAfterPublish(destDir); err != nil {
		return fmt.Errorf("%w: %w", errPublishDurability, err)
	}
	fmt.Printf("installed %s\n", finalPath)
	return nil
}

// abortBeforePublish rolls Home back only when rename has not committed.
func abortBeforePublish(hooks Hooks, err error) error {
	if hooks.Rollback == nil {
		return err
	}
	if rbErr := hooks.Rollback(); rbErr != nil {
		return fmt.Errorf("%w; %v", err, rbErr)
	}
	return err
}

var (
	renameFn          = os.Rename
	syncFile          = func(f *os.File) error { return f.Sync() }
	syncBeforePublish = syncDir
	syncAfterPublish  = syncDir
	// Stage sweep hooks. Production leaves them nil. The Unix sweeper calls
	// them between identity checks so tests can substitute the directory entry.
	stageAfterInspect func()
	stageBeforeUnlink func()
)

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// installLockCacheDir overrides the mutatelock cache in tests.
// Production leaves it empty so install coordinates with other Atlas mutators.
var installLockCacheDir string

// installAt is the make install path: one Home lock, one mirror transaction,
// then publish. The previous binary stays in place until rename succeeds.
func installAt(binaryPath, prefix string) error {
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("stat binary: %w", err)
	}
	homePath, err := home.Resolve()
	if err != nil {
		return err
	}
	release, err := lockHome(homePath)
	if err != nil {
		return err
	}
	defer release()

	var snap home.MirrorSnapshot
	var result home.EnsureResult
	mirrored := false
	return Install(binaryPath, prefix, Hooks{
		Materialize: func() error {
			var capErr error
			snap, capErr = home.CaptureMirrorSnapshot(homePath)
			if capErr != nil {
				return fmt.Errorf("capture Atlas Home: %w", capErr)
			}
			var mirErr error
			result, mirErr = ensureHome(time.Now().UTC())
			if mirErr != nil {
				if rbErr := rollbackMirror(snap, result); rbErr != nil {
					return fmt.Errorf("%w; %v", mirErr, rbErr)
				}
				return mirErr
			}
			mirrored = true
			return nil
		},
		Rollback: func() error {
			if !mirrored {
				return nil
			}
			return rollbackMirror(snap, result)
		},
	})
}
