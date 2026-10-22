//go:build !unix

package main

import (
	"fmt"
	"os"
)

func acquirePrefixLock(destDir string) (*os.File, error) {
	return nil, fmt.Errorf("installlocal: unsupported platform")
}

func unlockPrefix(f *os.File) error { return nil }

func flockExclusive(f *os.File) error {
	return fmt.Errorf("installlocal: unsupported platform")
}

func sweepAbandonedStages(destDir string) error { return nil }

func sameDevice(pathA, pathB string) error { return nil }

func checkPublishTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat publish target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("publish collision: %s is not a regular file", path)
	}
	return nil
}
