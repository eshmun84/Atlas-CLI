//go:build opencode_bun

// Opt-in Bun runtime for the OpenCode plugin asset.
// Excluded from go test ./... A missing bun binary fails this test.
//
//	go test -tags opencode_bun ./internal/assets/adapter-files/opencode/hooks/atlas-hooks -count=1
package atlashooks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBunPluginRuntime(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Fatalf("opencode_bun requires bun on PATH: %v", err)
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
