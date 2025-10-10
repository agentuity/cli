//go:build !windows

package util

import (
	"os/exec"
	"syscall"
)

func ProcessKill(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func ProcessSetup(cmd *exec.Cmd) {
	// set this process as the parent of the process group since it will likely span child processes
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
