//go:build !unix

package mutatelock

import (
	"errors"
	"fmt"
)

// ErrBusy is returned when another cooperating Atlas process holds the lock.
var ErrBusy = errors.New("another Atlas mutation is in progress")

// Set is a placeholder on non-Unix platforms.
type Set struct{}

// Options controls Acquire.
type Options struct {
	HomePath  string
	Workspace string
	CacheDir  string
}

// Acquire is unsupported outside Unix (macOS/Linux) for this phase.
func Acquire(opts Options) (Set, error) {
	return Set{}, fmt.Errorf("mutatelock: unsupported platform")
}

// Release is a no-op on unsupported platforms.
func (s *Set) Release() error { return nil }
