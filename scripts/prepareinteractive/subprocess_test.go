package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLimitedWriter_BoundsDuringWrite(t *testing.T) {
	killed := false
	w := &limitedWriter{max: 8, onOverflow: func() { killed = true }}
	if _, err := w.Write([]byte("12345678")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("X")); !errors.Is(err, errComposeOutputOverflow) {
		t.Fatalf("err=%v", err)
	}
	if !killed || !w.Overflowed() || len(w.Bytes()) != 8 {
		t.Fatalf("killed=%v overflow=%v len=%d", killed, w.Overflowed(), len(w.Bytes()))
	}
}

func TestCompose_BoundedJSONNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-atlas")
	body := "#!/bin/sh\necho '{\"schema_version\":1,\"status\":\"invalid_request\",\"count\":0,\"command\":\"compose\"}'\nexit 2\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	res, raw, err := compose(script, dir, dir, "task")
	if !jsonLooksOK(raw) {
		t.Fatalf("raw=%s", raw)
	}
	if res.SchemaVersion != 1 || res.Status != "invalid_request" {
		t.Fatalf("%#v", res)
	}
	if err == nil {
		t.Fatal("expected non-zero exit preserved")
	}
}

func TestCompose_Timeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep unavailable")
	}
	// Temporarily shrink timeout via a tiny wrapper that sleeps.
	// compose uses fixed 30s — use a fake atlas that hangs; kill via overflow is
	// tested separately. Here use CommandContext path with sleep by pointing
	// atlasBin at sleep (args ignored after binary). compose always passes
	// context compose args, so use a script.
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "hang")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Override timeout by testing limitedWriter kill path is enough for overflow;
	// for timeout, call compose with hanging binary — 30s is long for unit tests.
	// Use a local helper with short timeout instead.
	start := time.Now()
	_, _, err := composeBounded(script, dir, dir, "task", 200*time.Millisecond, maxComposeStdout, maxComposeStderr)
	if !errors.Is(err, errComposeTimeout) {
		t.Fatalf("err=%v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("slow kill: %s", time.Since(start))
	}
}

func TestCompose_OutputOverflowDuringCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	if _, err := exec.LookPath("yes"); err != nil {
		t.Skip("yes unavailable")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "spam")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec yes OVERFLOW\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, out, err := composeBounded(script, dir, dir, "task", 5*time.Second, 256, 256)
	if !errors.Is(err, errComposeOutputOverflow) {
		t.Fatalf("err=%v", err)
	}
	if len(out) > 256 {
		t.Fatalf("stdout not bounded during capture: %d", len(out))
	}
}

func jsonLooksOK(raw []byte) bool {
	return len(raw) > 0 && raw[0] == '{'
}
