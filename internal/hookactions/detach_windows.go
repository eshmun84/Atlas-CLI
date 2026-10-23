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
		envMaintainMode+"="+maintainModeSession,
		envMaintainRoot+"="+ref.Root,
		envMaintainHome+"="+ref.Home,
		envMaintainProjectID+"="+ref.ProjectID,
		envMaintainAdmissionID+"="+ref.AdmissionID,
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

func spawnDetachedScheduler(ref ProjectRef, token string, fence int64) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("hookactions: resolve executable: %w", err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		EnvInternalMaintain+"=1",
		envMaintainMode+"="+maintainModePostCode,
		envMaintainRoot+"="+ref.Root,
		envMaintainHome+"="+ref.Home,
		envMaintainProjectID+"="+ref.ProjectID,
		envMaintainToken+"="+strings.TrimSpace(token),
		envMaintainFence+"="+fmt.Sprintf("%d", fence),
	)
	if ref.Home != "" {
		cmd.Env = append(cmd.Env, "ATLAS_HOME="+ref.Home)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hookactions: spawn post-code scheduler: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}
