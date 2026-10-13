package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunBounded_TimeoutKillsChild(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	start := time.Now()
	rr, err := runBounded(context.Background(), 200*time.Millisecond, 1024, 1024, "sleep", []string{"10"}, "", os.Environ())
	elapsed := time.Since(start)
	if !errors.Is(err, errProcessTimeout) && !rr.TimedOut {
		t.Fatalf("want timeout, got err=%v timedOut=%v", err, rr.TimedOut)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("child not terminated promptly: %s", elapsed)
	}
}

func TestRunBounded_OutputOverflowKillsChild(t *testing.T) {
	if _, err := exec.LookPath("yes"); err != nil {
		t.Skip("yes not available")
	}
	rr, err := runBounded(context.Background(), 5*time.Second, 256, 256, "yes", []string{"OVERFLOW"}, "", os.Environ())
	if !rr.Overflowed && !errors.Is(err, errOutputOverflow) {
		t.Fatalf("want overflow, got err=%v overflowed=%v len=%d", err, rr.Overflowed, len(rr.Stdout))
	}
	if len(rr.Stdout) > 256 {
		t.Fatalf("stdout not bounded: %d", len(rr.Stdout))
	}
}

func TestRunBounded_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fail")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rr, err := runBounded(context.Background(), 5*time.Second, 4096, 4096, script, nil, dir, os.Environ())
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}
	if rr.ExitCode != 3 {
		t.Fatalf("exit=%d", rr.ExitCode)
	}
	if rr.TimedOut || rr.Overflowed {
		t.Fatalf("unexpected flags %#v", rr)
	}
}

func TestSanitizeErrorSnippet_RedactsAndTruncates(t *testing.T) {
	stdout := []byte(`{"path":"/Users/secret/home/proj"}`)
	pad := make([]byte, 400)
	for i := range pad {
		pad[i] = 'x'
	}
	stdout = append(stdout, pad...)
	got := sanitizeErrorSnippet(stdout, []byte("/tmp/leak"))
	if strings.Contains(got, "/Users/secret") {
		t.Fatalf("leaked path: %q", got)
	}
	if strings.Contains(got, "stderr=/tmp/") {
		t.Fatalf("leaked raw tmp path: %q", got)
	}
	if !strings.Contains(got, "/…/tmp/") {
		t.Fatalf("expected redacted tmp marker: %q", got)
	}
	if !strings.Contains(got, "…(truncated)") {
		t.Fatalf("expected truncation marker: %q", got)
	}
}
