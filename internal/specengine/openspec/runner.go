package openspec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const (
	probeTimeout     = 5 * time.Second
	mutateTimeout    = 60 * time.Second
	defaultMaxOutput = 1 << 20 // 1 MiB
)

// RunResult is the bounded outcome of one OpenSpec CLI invocation.
type RunResult struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	Truncated bool
}

// Runner executes OpenSpec CLI invocations. Implementations must never use a shell.
type Runner interface {
	Run(ctx context.Context, executable string, args []string, dir string) (RunResult, error)
}

// LookPathFunc locates an executable on PATH.
type LookPathFunc func(file string) (string, error)

// ExecRunner runs OpenSpec with explicit argv, timeout, output bounds, and cwd.
type ExecRunner struct {
	Timeout   time.Duration
	MaxOutput int64
}

// Run executes executable with args in dir. Never uses sh -c or shell concatenation.
func (r ExecRunner) Run(ctx context.Context, executable string, args []string, dir string) (RunResult, error) {
	if executable == "" {
		return RunResult{}, fmt.Errorf("openspec: executable path is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = probeTimeout
	}
	maxOut := r.MaxOutput
	if maxOut <= 0 {
		maxOut = defaultMaxOutput
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, executable, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutW := &limitedWriter{buf: &stdoutBuf, limit: maxOut}
	stderrW := &limitedWriter{buf: &stderrBuf, limit: maxOut}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	err := cmd.Run()
	result := RunResult{
		Stdout:    append([]byte(nil), stdoutBuf.Bytes()...),
		Stderr:    append([]byte(nil), stderrBuf.Bytes()...),
		Truncated: stdoutW.truncated || stderrW.truncated,
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if runCtx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("openspec: command timed out after %s: %w", timeout, ErrTimeout)
	}
	if runCtx.Err() == context.Canceled {
		return result, fmt.Errorf("openspec: command canceled: %w", context.Canceled)
	}
	if result.Truncated {
		return result, fmt.Errorf("openspec: output exceeded %d bytes: %w", maxOut, ErrOutputExceeded)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, fmt.Errorf("openspec: exit %d: %w", exitErr.ExitCode(), ErrProcessFailed)
		}
		return result, fmt.Errorf("openspec: run: %w", err)
	}
	return result, nil
}

type limitedWriter struct {
	buf       *bytes.Buffer
	limit     int64
	n         int64
	truncated bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.limit > 0 && w.n >= w.limit {
		w.truncated = true
		return len(p), nil
	}
	remain := w.limit - w.n
	if w.limit > 0 && int64(len(p)) > remain {
		_, _ = w.buf.Write(p[:remain])
		w.n = w.limit
		w.truncated = true
		return len(p), nil
	}
	n, err := w.buf.Write(p)
	w.n += int64(n)
	return n, err
}

var _ io.Writer = (*limitedWriter)(nil)

// Sentinel errors for provider execution.
var (
	ErrTimeout        = errors.New("provider timeout")
	ErrProcessFailed  = errors.New("provider process failed")
	ErrInvalidJSON    = errors.New("invalid provider JSON")
	ErrOutputExceeded = errors.New("provider output exceeded bound")
	ErrMissing        = errors.New("openspec executable not found")
)
