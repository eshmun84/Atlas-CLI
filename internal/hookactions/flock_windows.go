//go:build windows

package hookactions

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func flockExclusiveNonBlocking(f *os.File) error {
	// LockFileEx with LOCKFILE_FAIL_IMMEDIATELY — one byte range from offset 0.
	var ol windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return errMaintainBusy
	}
	return fmt.Errorf("hookactions: LockFileEx: %w", err)
}

func flockUnlock(f *os.File) error {
	var ol windows.Overlapped
	if err := windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol); err != nil {
		return fmt.Errorf("hookactions: UnlockFileEx: %w", err)
	}
	return nil
}
