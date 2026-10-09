package fsafety

import "os"

// SetRemoveForTest installs a test-only os.Remove seam for RemoveRegularFileTracked.
func SetRemoveForTest(fn func(string) error) {
	if fn == nil {
		removeFn = os.Remove
		return
	}
	removeFn = fn
}

// SetMkdirForTest installs a test-only os.Mkdir seam for contained directory creation.
func SetMkdirForTest(fn func(string, os.FileMode) error) {
	if fn == nil {
		mkdirFn = os.Mkdir
		return
	}
	mkdirFn = fn
}
