//go:build unix

package hookactions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func spawnDetachedWorker(ref ProjectRef) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("hookactions: resolve executable: %w", err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		EnvInternalMaintain+"=1",
		envMaintainRoot+"="+ref.Root,
		envMaintainHome+"="+ref.Home,
	)
	if ref.Home != "" {
		cmd.Env = append(cmd.Env, "ATLAS_HOME="+ref.Home)
	}
	if sid := strings.TrimSpace(ref.SessionID); sid != "" {
		cmd.Env = append(cmd.Env, envMaintainSession+"="+sid)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hookactions: spawn maintain worker: %w", err)
	}
	// Intentionally do not Wait — worker outlives the hook subprocess.
	_ = cmd.Process.Release()
	return nil
}
