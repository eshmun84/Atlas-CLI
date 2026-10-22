package main

import (
	"errors"
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
)

// ensureHome is the Home mutation used by install. Tests may wrap it.
var ensureHome = home.EnsureAndMirror

// Install calls the Rollback hook only before rename commits the binary.
// A directory sync error after that rename must not call rollbackMirror:
// the new binary and the materialized Home are the recoverable state, and a
// later install reconciles them. See errPublishDurability.

// lockHome serializes this install with other Atlas Home mutators.
// The lock lives in the user cache (or installLockCacheDir), not inside Home,
// so acquiring it does not create Atlas Home.
func lockHome(homePath string) (func(), error) {
	set, err := mutatelock.Acquire(mutatelock.Options{
		HomePath: homePath,
		CacheDir: installLockCacheDir,
	})
	if err != nil {
		if errors.Is(err, mutatelock.ErrBusy) {
			return nil, fmt.Errorf("atlas home: another installation is in progress: %w", err)
		}
		return nil, err
	}
	return func() { _ = set.Release() }, nil
}
