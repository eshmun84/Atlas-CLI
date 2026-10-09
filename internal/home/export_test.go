package home

import "github.com/eshmun84/Atlas-CLI/internal/assets"

// SetReadEmbeddedAssetForTest installs a test-only embedded asset reader for Inspect.
func SetReadEmbeddedAssetForTest(fn func(embedPath string) ([]byte, error)) {
	if fn == nil {
		readEmbeddedAssetFn = func(embedPath string) ([]byte, error) {
			return assets.Content.ReadFile(embedPath)
		}
		return
	}
	readEmbeddedAssetFn = fn
}
