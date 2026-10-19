//go:build unix

package hookactions

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func flockExclusiveNonBlocking(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errMaintainBusy
	}
	return fmt.Errorf("hookactions: flock: %w", err)
}

func flockUnlock(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("hookactions: funlock: %w", err)
	}
	return nil
}
