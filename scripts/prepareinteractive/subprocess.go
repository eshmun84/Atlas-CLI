package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
)

const (
	composeTimeout   = 30 * time.Second
	maxComposeStdout = 512 * 1024
	maxComposeStderr = 64 * 1024
)

var (
	errComposeOutputOverflow = errors.New("compose subprocess output exceeded capture bound")
	errComposeTimeout        = errors.New("compose subprocess timed out")
)

// limitedWriter bounds capture during Write and kills the child on overflow.
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
		return len(p), errComposeOutputOverflow
	}
	remain := w.max - w.buf.Len()
	if remain <= 0 {
		w.overflowed = true
		if w.onOverflow != nil {
			w.onOverflow()
		}
		return len(p), errComposeOutputOverflow
	}
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		w.overflowed = true
		if w.onOverflow != nil {
			w.onOverflow()
		}
		return remain, errComposeOutputOverflow
	}
	return w.buf.Write(p)
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

func compose(atlasBin, atlasHome, project, task string) (composeapp.Result, []byte, error) {
	return composeBounded(atlasBin, atlasHome, project, task, composeTimeout, maxComposeStdout, maxComposeStderr)
}

func composeBounded(atlasBin, atlasHome, project, task string, timeout time.Duration, maxOut, maxErr int) (composeapp.Result, []byte, error) {
	if timeout <= 0 {
		timeout = composeTimeout
	}
	if maxOut <= 0 {
		maxOut = maxComposeStdout
	}
	if maxErr <= 0 {
		maxErr = maxComposeStderr
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, atlasBin, "context", "compose", task, "--json")
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "ATLAS_HOME="+atlasHome)

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
	out := stdoutW.Bytes()
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	overflowed := stdoutW.Overflowed() || stderrW.Overflowed()

	var res composeapp.Result
	jsonOK := false
	if len(out) > 0 {
		if err := json.Unmarshal(out, &res); err == nil {
			jsonOK = true
		}
	}

	switch {
	case timedOut:
		return res, out, fmt.Errorf("%w after %s", errComposeTimeout, timeout)
	case overflowed:
		return res, out, fmt.Errorf("%w (stdout=%d stderr=%d)", errComposeOutputOverflow, len(out), len(stderrW.Bytes()))
	case jsonOK:
		// Preserve non-zero exit with structured JSON for callers.
		return res, out, runErr
	case runErr != nil:
		return res, out, fmt.Errorf("%w: stdout not valid JSON stderr=%q", runErr, truncate(string(stderrW.Bytes()), 200))
	default:
		return res, out, fmt.Errorf("compose stdout was not valid JSON")
	}
}
