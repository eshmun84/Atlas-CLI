package atlashooks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBunPluginRuntime runs Bun subprocess tests for the OpenCode plugin asset.
// Skips when bun is not installed (local optional tooling).
func TestBunPluginRuntime(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun not on PATH; install bun to run OpenCode plugin runtime tests")
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Package lives next to this file.
	cmd := exec.Command(bun, "test", "./index.test.ts")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bun test failed: %v\n%s\n(dir=%s)", err, out, filepath.Clean(dir))
	}
	t.Logf("bun test ok:\n%s", out)
}
