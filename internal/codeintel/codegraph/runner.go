package codegraph

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
	defaultTimeout   = 5 * time.Second
	defaultMaxOutput = 1 << 20 // 1 MiB
)

// RunResult is the bounded outcome of one provider process invocation.
type RunResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner executes CodeGraph CLI invocations. Implementations must never use a shell.
type Runner interface {
	Run(ctx context.Context, executable string, args []string) (RunResult, error)
}

// LookPathFunc locates an executable on PATH.
type LookPathFunc func(file string) (string, error)

// ExecRunner runs CodeGraph with explicit argv, timeout, and output bounds.
type ExecRunner struct {
	Timeout   time.Duration
	MaxOutput int64
}

// Run executes executable with args. Never uses sh -c or shell concatenation.
func (r ExecRunner) Run(ctx context.Context, executable string, args []string) (RunResult, error) {
	if executable == "" {
		return RunResult{}, fmt.Errorf("codegraph: executable path is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxOut := r.MaxOutput
	if maxOut <= 0 {
		maxOut = defaultMaxOutput
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, executable, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdoutBuf, limit: maxOut}
	cmd.Stderr = &limitedWriter{buf: &stderrBuf, limit: maxOut}

	err := cmd.Run()
	result := RunResult{
		Stdout: append([]byte(nil), stdoutBuf.Bytes()...),
		Stderr: append([]byte(nil), stderrBuf.Bytes()...),
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if runCtx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("codegraph: command timed out after %s: %w", timeout, ErrTimeout)
	}
	if runCtx.Err() == context.Canceled {
		return result, fmt.Errorf("codegraph: command canceled: %w", context.Canceled)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, fmt.Errorf("codegraph: exit %d: %w", exitErr.ExitCode(), ErrProcessFailed)
		}
		return result, fmt.Errorf("codegraph: run: %w", err)
	}
	return result, nil
}

type limitedWriter struct {
	buf   *bytes.Buffer
	limit int64
	n     int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.limit > 0 && w.n >= w.limit {
		return len(p), nil
	}
	remain := w.limit - w.n
	if w.limit > 0 && int64(len(p)) > remain {
		_, _ = w.buf.Write(p[:remain])
		w.n = w.limit
		return len(p), nil
	}
	n, err := w.buf.Write(p)
	w.n += int64(n)
	return n, err
}

// Ensure limitedWriter implements io.Writer.
var _ io.Writer = (*limitedWriter)(nil)

// Sentinel errors normalized by the adapter.
var (
	ErrTimeout       = errors.New("provider timeout")
	ErrProcessFailed = errors.New("provider process failed")
	ErrInvalidJSON   = errors.New("invalid provider JSON")
)
