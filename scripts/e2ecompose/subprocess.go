package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
)

const (
	defaultComposeTimeout = 30 * time.Second
	maxStdoutCapture      = 512 * 1024
	maxStderrCapture      = 64 * 1024
	errorSnippetMax       = 240
)

var (
	errOutputOverflow = errors.New("subprocess output exceeded capture bound")
	errProcessTimeout = errors.New("subprocess timed out")
)

// limitedWriter captures up to max bytes then signals overflow and invokes onOverflow.
type limitedWriter struct {
	mu         sync.Mutex
	buf        bytes.Buffer
	max        int
	overflowed bool
	onOverflow func()
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.overflowed {
		return len(p), errOutputOverflow
	}
	remain := w.max - w.buf.Len()
	if remain <= 0 {
		w.overflowed = true
		if w.onOverflow != nil {
			w.onOverflow()
		}
		return len(p), errOutputOverflow
	}
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		w.overflowed = true
		if w.onOverflow != nil {
			w.onOverflow()
		}
		return remain, errOutputOverflow
	}
	n, err := w.buf.Write(p)
	return n, err
}

func (w *limitedWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]byte, w.buf.Len())
	copy(out, w.buf.Bytes())
	return out
}

func (w *limitedWriter) Overflowed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.overflowed
}

// runResult is a bounded atlas subprocess outcome.
type runResult struct {
	Stdout     []byte
	Stderr     []byte
	ExitCode   int
	TimedOut   bool
	Overflowed bool
	Result     composeapp.Result
	JSONOK     bool
}

// runBounded executes bin with a deadline and capture bounds. On timeout or
// output overflow the child process is killed. stdout is parsed as JSON when
// possible even if the exit code is non-zero.
func runBounded(parent context.Context, timeout time.Duration, maxOut, maxErr int, bin string, args []string, dir string, env []string) (runResult, error) {
	if timeout <= 0 {
		timeout = defaultComposeTimeout
	}
	if maxOut <= 0 {
		maxOut = maxStdoutCapture
	}
	if maxErr <= 0 {
		maxErr = maxStderrCapture
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = env

	var killed sync.Once
	kill := func() {
		killed.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
	}

	stdoutW := &limitedWriter{max: maxOut, onOverflow: kill}
	stderrW := &limitedWriter{max: maxErr, onOverflow: kill}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	runErr := cmd.Run()
	out := runResult{
		Stdout:     stdoutW.Bytes(),
		Stderr:     stderrW.Bytes(),
		Overflowed: stdoutW.Overflowed() || stderrW.Overflowed(),
		TimedOut:   errors.Is(ctx.Err(), context.DeadlineExceeded),
		ExitCode:   -1,
	}

	if cmd.ProcessState != nil {
		out.ExitCode = cmd.ProcessState.ExitCode()
	}

	if len(out.Stdout) > 0 {
		if err := json.Unmarshal(out.Stdout, &out.Result); err == nil {
			out.JSONOK = true
		}
	}

	switch {
	case out.TimedOut:
		return out, fmt.Errorf("%w after %s", errProcessTimeout, timeout)
	case out.Overflowed:
		return out, fmt.Errorf("%w (stdout=%d stderr=%d)", errOutputOverflow, len(out.Stdout), len(out.Stderr))
	case runErr == nil:
		return out, nil
	case out.JSONOK:
		// Non-zero exit with structured JSON is a valid transport outcome.
		return out, runErr
	default:
		return out, fmt.Errorf("%w: %s", runErr, sanitizeErrorSnippet(out.Stdout, out.Stderr))
	}
}

func sanitizeErrorSnippet(stdout, stderr []byte) string {
	parts := make([]string, 0, 2)
	if n := len(stdout); n > 0 {
		s := string(stdout)
		if n > errorSnippetMax {
			s = s[:errorSnippetMax] + "…(truncated)"
		}
		parts = append(parts, "stdout="+redactPaths(s))
	} else {
		parts = append(parts, "stdout=<empty>")
	}
	if n := len(stderr); n > 0 {
		s := string(stderr)
		if n > errorSnippetMax {
			s = s[:errorSnippetMax] + "…(truncated)"
		}
		parts = append(parts, "stderr="+redactPaths(s))
	}
	if !json.Valid(stdout) && len(stdout) > 0 {
		parts = append(parts, "json=malformed_or_truncated")
	}
	return strings.Join(parts, " ")
}

func redactPaths(s string) string {
	// Avoid echoing absolute home/temp paths in harness errors.
	s = strings.ReplaceAll(s, "/Users/", "/…/")
	s = strings.ReplaceAll(s, "/home/", "/…/")
	s = strings.ReplaceAll(s, "/var/folders/", "/…/folders/")
	s = strings.ReplaceAll(s, "/private/var/", "/…/var/")
	s = strings.ReplaceAll(s, "/tmp/", "/…/tmp/")
	return s
}
