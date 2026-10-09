package codeintel

import (
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// SetRootFingerprintForTest installs a test-only RootFingerprint seam.
func SetRootFingerprintForTest(fn func(string) (string, error)) {
	if fn == nil {
		rootFingerprintFn = home.RootFingerprint
		return
	}
	rootFingerprintFn = fn
}

// SetFilepathWalkForTest installs a test-only filepath.Walk seam for containment.
func SetFilepathWalkForTest(fn func(root string, fn filepath.WalkFunc) error) {
	if fn == nil {
		filepathWalkFn = filepath.Walk
		return
	}
	filepathWalkFn = fn
}

// SetFingerprintWalkForTest installs a test-only Walk seam for ComputeSourceFingerprint.
func SetFingerprintWalkForTest(fn func(root string, walkFn filepath.WalkFunc) error) {
	if fn == nil {
		fingerprintWalkFn = filepath.Walk
		return
	}
	fingerprintWalkFn = fn
}

// SetFingerprintRelForTest installs a test-only Rel seam for ComputeSourceFingerprint.
func SetFingerprintRelForTest(fn func(basepath, targpath string) (string, error)) {
	if fn == nil {
		fingerprintRelFn = filepath.Rel
		return
	}
	fingerprintRelFn = fn
}

// SnapshotCodegraphSideEffectsForTest exposes snapshotCodegraphSideEffects.
func SnapshotCodegraphSideEffectsForTest(root string) containmentSnapshot {
	return snapshotCodegraphSideEffects(root)
}

// ContainCodegraphSideEffectsForTest exposes containCodegraphSideEffects.
func ContainCodegraphSideEffectsForTest(root string, before containmentSnapshot) ContainmentReport {
	return containCodegraphSideEffects(root, before)
}
