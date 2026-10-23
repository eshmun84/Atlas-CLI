package doctor_test

import (
	"os"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/testharness"
)

func TestMain(m *testing.M) {
	installed := testharness.Install(testharness.Options{})
	code := m.Run()
	installed.Cleanup()
	os.Exit(code)
}
