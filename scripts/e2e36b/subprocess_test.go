package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunBounded_CapturesStdout(t *testing.T) {
	bin := "echo"
	if runtime.GOOS == "windows" {
		t.Skip("unix echo")
	}
	rr, err := runBounded(context.Background(), 5*time.Second, 1024, 1024, bin, []string{"hello-e2e36b"}, "", os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if string(rr.Stdout) == "" {
		t.Fatal("empty stdout")
	}
}

func TestRunBounded_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sleep")
	}
	_, err := runBounded(context.Background(), 200*time.Millisecond, 1024, 1024, "sleep", []string{"2"}, "", os.Environ())
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestResolveAtlasBin_RequiresExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-exec")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveAtlasBin(path); err == nil {
		t.Fatal("expected rejection of non-executable")
	}
}
