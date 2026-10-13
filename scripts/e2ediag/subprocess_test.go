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

func TestRunBounded_StdoutOverflowKillsChild(t *testing.T) {
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

func TestRunBounded_StderrOverflowKillsChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "spam-err")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nwhile true; do echo OVERFLOW >&2; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rr, err := runBounded(context.Background(), 5*time.Second, 256, 128, script, nil, dir, os.Environ())
	if !rr.Overflowed && !errors.Is(err, errOutputOverflow) {
		t.Fatalf("want stderr overflow, got err=%v overflowed=%v stderr=%d", err, rr.Overflowed, len(rr.Stderr))
	}
	if len(rr.Stderr) > 128 {
		t.Fatalf("stderr not bounded: %d", len(rr.Stderr))
	}
}

func TestRunBounded_ValidJSONNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-atlas")
	body := "#!/bin/sh\necho '{\"schema_version\":1,\"command\":\"propose\",\"status\":\"invalid_input\",\"error\":{\"code\":\"invalid_input\",\"message\":\"malformed candidate JSON\"}}'\nexit 2\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	rr, err := runBounded(context.Background(), 5*time.Second, 4096, 4096, script, nil, dir, os.Environ())
	if !rr.JSONOK {
		t.Fatalf("expected JSON parse, err=%v stdout=%q", err, rr.Stdout)
	}
	if err == nil {
		t.Fatal("expected non-zero exit error preserved with JSON")
	}
	if rr.TimedOut || rr.Overflowed {
		t.Fatalf("unexpected flags %#v", rr)
	}
	if !strings.Contains(string(rr.Stdout), `"invalid_input"`) {
		t.Fatalf("stdout=%s", rr.Stdout)
	}
}

func TestRunBounded_MalformedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-atlas")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho not-json\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rr, err := runBounded(context.Background(), 5*time.Second, 4096, 4096, script, nil, dir, os.Environ())
	if err == nil {
		t.Fatal("expected error")
	}
	if rr.JSONOK {
		t.Fatal("must not report JSONOK")
	}
	msg := err.Error()
	if !strings.Contains(msg, "json=malformed_or_truncated") && !strings.Contains(msg, "stdout=") {
		t.Fatalf("error should describe malformed output safely: %q", msg)
	}
}

func TestRunBounded_StdinOverflowRejected(t *testing.T) {
	_, err := runBoundedWithStdin(context.Background(), time.Second, 1024, 1024, 16, "true", nil, "", os.Environ(), bytesRepeat(32))
	if !errors.Is(err, errStdinOverflow) {
		t.Fatalf("err=%v", err)
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

func bytesRepeat(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return b
}
