//go:build windows

package hookactions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
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
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hookactions: spawn maintain worker: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}
