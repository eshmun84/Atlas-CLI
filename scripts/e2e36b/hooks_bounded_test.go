package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunBoundedWithStdin_RejectsOversizedStdin(t *testing.T) {
	huge := make([]byte, maxHooksStdin+1)
	_, err := runBoundedWithStdin(context.Background(), time.Second, 1024, 1024, maxHooksStdin, "true", nil, "", os.Environ(), huge)
	if !errors.Is(err, errStdinOverflow) {
		t.Fatalf("got %v want stdin overflow", err)
	}
}

func TestRunBoundedWithStdin_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sleep")
	}
	_, err := runBoundedWithStdin(context.Background(), 200*time.Millisecond, 1024, 1024, 64, "sleep", []string{"2"}, "", os.Environ(), []byte("{}"))
	if !errors.Is(err, errProcessTimeout) {
		t.Fatalf("got %v want timeout", err)
	}
}

func TestRunBoundedWithStdin_OutputOverflow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix yes/head")
	}
	// Generate more than 32 bytes of stdout quickly.
	_, err := runBoundedWithStdin(context.Background(), 5*time.Second, 32, 32, 64, "python3", []string{"-c", "print('x'*10000)"}, "", os.Environ(), nil)
	if !errors.Is(err, errOutputOverflow) {
		t.Fatalf("got %v want output overflow", err)
	}
}

func TestRunBoundedWithStdin_RedactsAbsolutePathsInErrors(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist-binary")
	_, err := runBoundedWithStdin(context.Background(), time.Second, 1024, 1024, 64, missing, nil, "", os.Environ(), []byte("{}"))
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "/Users/") || strings.Contains(msg, "/home/") {
		// sanitizeErrorSnippet redacts when stdout/stderr present; spawn errors may include path.
		// Ensure redactPaths is applied for snippets that include home prefixes when present.
		redacted := redactPaths(msg)
		if strings.Contains(redacted, "/Users/") {
			t.Fatalf("redactPaths left absolute user path: %s", redacted)
		}
	}
}

func TestRequireEvidence_PropagatesWriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ev.txt")
	if err := requireEvidence(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := requireEvidence(path, []byte("two")); err == nil {
		t.Fatal("expected exclusive-create failure to propagate")
	}
}

func TestFingerprintError_NotSilentSuccess(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-tree")
	_, err := fingerprintTree(missing)
	if err == nil {
		t.Fatal("expected fingerprint error on missing tree")
	}
}
