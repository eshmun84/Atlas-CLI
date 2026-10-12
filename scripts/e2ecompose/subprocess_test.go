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

func TestRunBounded_ValidJSONNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-atlas")
	body := "#!/bin/sh\necho '{\"schema_version\":1,\"status\":\"invalid_request\",\"count\":0,\"command\":\"compose\"}'\nexit 2\n"
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	rr, err := runBounded(context.Background(), 5*time.Second, 4096, 4096, script, nil, dir, os.Environ())
	if !rr.JSONOK {
		t.Fatalf("expected JSON parse, err=%v stdout=%q", err, rr.Stdout)
	}
	if rr.Result.Status != "invalid_request" {
		t.Fatalf("status=%q", rr.Result.Status)
	}
	if err == nil {
		t.Fatal("expected non-zero exit error preserved with JSON")
	}
	if rr.TimedOut || rr.Overflowed {
		t.Fatalf("unexpected flags %#v", rr)
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
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker in %q", got)
	}
	if strings.Contains(got, "stderr=/tmp/") {
		t.Fatalf("stderr /tmp/ prefix not redacted: %q", got)
	}
	if !strings.Contains(got, "stderr=/…/tmp/leak") {
		t.Fatalf("expected redacted stderr form: %q", got)
	}
}
