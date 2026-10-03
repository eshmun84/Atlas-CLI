package version_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/version"
)

func TestVersionSet(t *testing.T) {
	t.Parallel()

	if version.Version == "" {
		t.Fatal("version.Version must not be empty")
	}
}
