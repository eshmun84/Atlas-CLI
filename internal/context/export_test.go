package context

import (
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// SetAfterWriteHookForTest installs a test-only seam after Home context writes.
func SetAfterWriteHookForTest(fn func(rel string) error) {
	contextAfterWriteHook = fn
}

// SetCaptureFileSnapshotForTest installs a test-only CaptureFileSnapshot seam.
func SetCaptureFileSnapshotForTest(fn func(root, rel string) (fsafety.FileSnapshot, error)) {
	if fn == nil {
		captureFileSnapshotFn = fsafety.CaptureFileSnapshot
		return
	}
	captureFileSnapshotFn = fn
}

// SetIndexWalkForTest installs a test-only Walk seam for BuildIndex.
func SetIndexWalkForTest(fn func(root string, walkFn filepath.WalkFunc) error) {
	if fn == nil {
		indexWalkFn = filepath.Walk
		return
	}
	indexWalkFn = fn
}

// SetIndexRelForTest installs a test-only Rel seam for BuildIndex.
func SetIndexRelForTest(fn func(basepath, targpath string) (string, error)) {
	if fn == nil {
		indexRelFn = filepath.Rel
		return
	}
	indexRelFn = fn
}
